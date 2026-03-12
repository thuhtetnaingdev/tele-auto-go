package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"tele-auto-go/internal/config"

	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "login failed: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
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

func ask(r *bufio.Reader, prompt string) (string, error) {
	fmt.Print(prompt)
	text, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(text), nil
}
