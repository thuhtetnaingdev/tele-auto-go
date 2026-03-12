package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tele-auto-go/internal/ai"
	"tele-auto-go/internal/config"
	"tele-auto-go/internal/health"
	"tele-auto-go/internal/logging"
	"tele-auto-go/internal/soul"
	"tele-auto-go/internal/store"
	tgsvc "tele-auto-go/internal/telegram"
)

func main() {
	if err := run(); err != nil {
		slog.Error("fatal error", "error", err.Error())
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.LoadForApp()
	if err != nil {
		return err
	}
	logger := logging.New(cfg.LogLevel)

	if err := os.MkdirAll(filepath.Dir(cfg.Telegram.SessionFile), 0o755); err != nil {
		return err
	}

	db, err := store.Open(cfg.SQLitePath)
	if err != nil {
		return err
	}
	defer db.Close()

	soulPrompt := soul.Load(cfg.SoulPromptPath, logger)
	aiClient := ai.New(
		cfg.OpenAI.BaseURL,
		cfg.OpenAI.APIKey,
		cfg.OpenAI.Model,
		cfg.OpenAI.MaxTokens,
		logger,
	)
	telegramService := tgsvc.NewService(cfg, logger, db, aiClient, soulPrompt)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpServer := health.NewServer(cfg.Port)
	go func() {
		logger.Info("Health server started", "addr", httpServer.Addr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, context.Canceled) {
			logger.Error("health server stopped unexpectedly", "error", err.Error())
			stop()
		}
	}()

	errCh := make(chan error, 1)
	go func() {
		errCh <- telegramService.Run(ctx)
	}()

	select {
	case <-ctx.Done():
		logger.Info("Shutting down")
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			logger.Error("Telegram service exited with error", "error", err.Error())
		}
		stop()
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = health.Shutdown(shutdownCtx, httpServer)

	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			return err
		}
	default:
	}
	return nil
}
