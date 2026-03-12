package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"

	"tele-auto-go/internal/ai"
	"tele-auto-go/internal/config"
	"tele-auto-go/internal/health"
	"tele-auto-go/internal/logging"
	"tele-auto-go/internal/soul"
	"tele-auto-go/internal/store"
	tgsvc "tele-auto-go/internal/telegram"
)

func main() {
	loginFlag := flag.Bool("login", false, "run Telegram login flow and store session")
	logoutFlag := flag.Bool("logout", false, "logout Telegram and remove session file")
	flag.Parse()

	if *loginFlag && *logoutFlag {
		fmt.Fprintln(os.Stderr, "cannot use --login and --logout together")
		os.Exit(2)
	}

	var err error
	switch {
	case *loginFlag:
		err = runLogin()
	case *logoutFlag:
		err = runLogout()
	default:
		err = runApp()
	}
	if err != nil {
		slog.Error("fatal error", "error", err.Error())
		os.Exit(1)
	}
}

func runApp() error {
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

func runLogin() error {
	cfg, err := config.LoadForLogin()
	if err != nil {
		return err
	}

	reader := bufio.NewReader(os.Stdin)
	phone := strings.TrimSpace(cfg.Telegram.Phone)
	if phone == "" {
		phone, err = ask(reader, "TG_PHONE (e.g. +15551234567): ")
		if err != nil {
			return err
		}
	}
	password, err := ask(reader, "2FA password (press Enter if none): ")
	if err != nil {
		return err
	}

	sessionFile := cfg.Telegram.SessionFile
	if err := os.MkdirAll(filepath.Dir(sessionFile), 0o755); err != nil {
		return err
	}

	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionFile},
	})

	ctx := context.Background()
	return client.Run(ctx, func(ctx context.Context) error {
		flow := auth.NewFlow(
			auth.Constant(phone, password, auth.CodeAuthenticatorFunc(func(ctx context.Context, _ *tg.AuthSentCode) (string, error) {
				return ask(reader, "OTP code: ")
			})),
			auth.SendCodeOptions{},
		)
		if err := client.Auth().IfNecessary(ctx, flow); err != nil {
			return err
		}

		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			return fmt.Errorf("authorization did not complete")
		}

		fmt.Println("")
		fmt.Println("Telegram login successful.")
		fmt.Printf("Session stored at: %s\n", sessionFile)
		fmt.Println("Use this value in .env:")
		fmt.Printf("TG_SESSION_FILE=%s\n", sessionFile)
		return nil
	})
}

func runLogout() error {
	cfg, err := config.LoadForLogin()
	if err != nil {
		return err
	}
	sessionFile := cfg.Telegram.SessionFile

	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: sessionFile},
	})

	ctx := context.Background()
	if err := client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			return nil
		}
		_, err = tg.NewClient(client).AuthLogOut(ctx)
		return err
	}); err != nil {
		return err
	}

	_ = os.Remove(filepath.Clean(sessionFile))
	fmt.Println("Logged out and session file removed.")
	return nil
}

func ask(r *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	text, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}

