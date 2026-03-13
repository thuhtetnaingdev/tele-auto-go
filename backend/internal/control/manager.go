package control

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"time"

	"tele-auto-go/internal/ai"
	"tele-auto-go/internal/config"
	"tele-auto-go/internal/soul"
	"tele-auto-go/internal/store"
	tgsvc "tele-auto-go/internal/telegram"
)

type ServiceStatus struct {
	Running   bool   `json:"running"`
	StartedAt string `json:"startedAt,omitempty"`
	UptimeSec int64  `json:"uptimeSec,omitempty"`
	LastError string `json:"lastError,omitempty"`
}

type Manager struct {
	mu sync.Mutex

	logger *slog.Logger
	onEvent func(tgsvc.Event)

	runID int64

	running   bool
	cancel    context.CancelFunc
	done      chan struct{}
	startedAt time.Time
	lastError string
	service   *tgsvc.Service
}

func NewManager(logger *slog.Logger, onEvent func(tgsvc.Event)) *Manager {
	return &Manager{logger: logger, onEvent: onEvent}
}

func (m *Manager) Start() error {
	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		return nil
	}
	m.mu.Unlock()

	cfg, err := config.LoadForApp()
	if err != nil {
		m.setLastError(err)
		return err
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Telegram.SessionFile), 0o755); err != nil {
		m.setLastError(err)
		return err
	}

	db, err := store.Open(cfg.SQLitePath)
	if err != nil {
		m.setLastError(err)
		return err
	}

	soulPrompt := soul.Load(cfg.SoulPromptPath, m.logger)
	aiClient := ai.New(
		cfg.OpenAI.BaseURL,
		cfg.OpenAI.APIKey,
		cfg.OpenAI.Model,
		cfg.OpenAI.MaxTokens,
		m.logger,
	)
	telegramService := tgsvc.NewService(cfg, m.logger, db, aiClient, soulPrompt, m.onEvent)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	var runID int64

	m.mu.Lock()
	if m.running {
		m.mu.Unlock()
		cancel()
		_ = db.Close()
		return nil
	}
	m.running = true
	m.cancel = cancel
	m.done = done
	m.service = telegramService
	m.startedAt = time.Now().UTC()
	m.lastError = ""
	m.runID++
	runID = m.runID
	m.mu.Unlock()

	m.logger.Info("telegram worker starting")
	go func() {
		defer close(done)
		defer db.Close()

		err := telegramService.Run(ctx)
		m.mu.Lock()
		if m.runID == runID {
			m.running = false
			m.cancel = nil
			m.done = nil
			m.service = nil
			m.startedAt = time.Time{}
		}
		if err != nil && !errors.Is(err, context.Canceled) {
			m.lastError = err.Error()
		}
		m.mu.Unlock()

		if err != nil && !errors.Is(err, context.Canceled) {
			m.logger.Error("telegram worker stopped with error", "error", err.Error())
			return
		}
		m.logger.Info("telegram worker stopped")
	}()

	return nil
}

func (m *Manager) Stop(ctx context.Context) error {
	m.mu.Lock()
	if !m.running || m.cancel == nil {
		m.running = false
		m.cancel = nil
		m.done = nil
		m.startedAt = time.Time{}
		m.mu.Unlock()
		return nil
	}

	cancel := m.cancel
	done := m.done
	m.runID++
	m.running = false
	m.cancel = nil
	m.done = nil
	m.service = nil
	m.startedAt = time.Time{}
	m.mu.Unlock()

	cancel()
	if done == nil {
		return nil
	}
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return fmt.Errorf("stop timeout: %w", ctx.Err())
	}
}

func (m *Manager) Restart(ctx context.Context) error {
	if err := m.Stop(ctx); err != nil {
		return err
	}
	return m.Start()
}

func (m *Manager) Status() ServiceStatus {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := ServiceStatus{
		Running:   m.running,
		LastError: m.lastError,
	}
	if !m.startedAt.IsZero() {
		status.StartedAt = m.startedAt.Format(time.RFC3339)
		status.UptimeSec = int64(time.Since(m.startedAt).Seconds())
	}
	return status
}

func (m *Manager) setLastError(err error) {
	m.mu.Lock()
	m.lastError = err.Error()
	m.mu.Unlock()
}

func (m *Manager) SendConversationMessage(ctx context.Context, chatID, text string) (store.MessageRecord, error) {
	m.mu.Lock()
	svc := m.service
	running := m.running
	m.mu.Unlock()

	if !running || svc == nil {
		return store.MessageRecord{}, fmt.Errorf("telegram worker is not running")
	}
	return svc.SendText(ctx, chatID, text)
}

func (m *Manager) ResolveConversationName(ctx context.Context, chatID string) (string, error) {
	m.mu.Lock()
	svc := m.service
	running := m.running
	m.mu.Unlock()

	if !running || svc == nil {
		return "", fmt.Errorf("telegram worker is not running")
	}
	return svc.ResolveChatDisplayName(ctx, chatID)
}
