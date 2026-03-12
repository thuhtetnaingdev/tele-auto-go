package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"tele-auto-go/internal/config"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "logout failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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
