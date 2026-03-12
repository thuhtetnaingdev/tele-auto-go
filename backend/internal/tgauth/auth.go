package tgauth

import (
	"context"
	"errors"
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

var (
	ErrCodeRequired      = errors.New("otp code is required")
	ErrPhoneCodeRequired = errors.New("phone_code_hash is required")
)

type OTPRequestResult struct {
	Phone          string
	PhoneCodeHash  string
	CodeType       string
	NextType       string
	TimeoutSeconds int
	AlreadyAuth    bool
}

func RequestOTP(ctx context.Context, cfg config.Config, phone string) (OTPRequestResult, error) {
	phone = normalizePhone(phone, cfg)
	if phone == "" {
		return OTPRequestResult{}, errors.New("phone is required")
	}

	if err := os.MkdirAll(filepath.Dir(cfg.Telegram.SessionFile), 0o755); err != nil {
		return OTPRequestResult{}, err
	}

	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.Telegram.SessionFile},
	})

	result := OTPRequestResult{Phone: phone}
	err := client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized {
			result.AlreadyAuth = true
			return nil
		}

		sentCode, err := client.Auth().SendCode(ctx, phone, auth.SendCodeOptions{})
		if err != nil {
			return err
		}

		sent, ok := sentCode.(*tg.AuthSentCode)
		if !ok {
			// If telegram returns successful authorization here, treat it as already logged in.
			if _, ok := sentCode.(*tg.AuthSentCodeSuccess); ok {
				result.AlreadyAuth = true
				return nil
			}
			return fmt.Errorf("unexpected sent code type: %T", sentCode)
		}

		result.PhoneCodeHash = strings.TrimSpace(sent.PhoneCodeHash)
		result.CodeType = sentCodeTypeName(sent.Type)
		if next, ok := sent.GetNextType(); ok {
			result.NextType = authCodeTypeName(next)
		}
		if timeout, ok := sent.GetTimeout(); ok {
			result.TimeoutSeconds = timeout
		}
		return nil
	})
	if err != nil {
		return OTPRequestResult{}, err
	}

	return result, nil
}

func VerifyOTP(ctx context.Context, cfg config.Config, phone, phoneCodeHash, code, password string) error {
	phone = normalizePhone(phone, cfg)
	if phone == "" {
		return errors.New("phone is required")
	}
	if strings.TrimSpace(phoneCodeHash) == "" {
		return ErrPhoneCodeRequired
	}
	if strings.TrimSpace(code) == "" {
		return ErrCodeRequired
	}

	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.Telegram.SessionFile},
	})

	return client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if status.Authorized {
			return nil
		}

		_, err = client.Auth().SignIn(ctx, phone, strings.TrimSpace(code), strings.TrimSpace(phoneCodeHash))
		if errors.Is(err, auth.ErrPasswordAuthNeeded) {
			pass := strings.TrimSpace(password)
			if pass == "" {
				return errors.New("2FA password is required")
			}
			_, err = client.Auth().Password(ctx, pass)
		}
		if err != nil {
			return err
		}

		status, err = client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		if !status.Authorized {
			return errors.New("authorization did not complete")
		}
		return nil
	})
}

func Login(ctx context.Context, cfg config.Config, phone, code, password string) error {
	if strings.TrimSpace(code) == "" {
		_, err := RequestOTP(ctx, cfg, phone)
		if err != nil {
			return err
		}
		return ErrCodeRequired
	}
	return errors.New("legacy login without phone_code_hash is not supported")
}

func Logout(ctx context.Context, cfg config.Config) error {
	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.Telegram.SessionFile},
	})

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

	_ = os.Remove(filepath.Clean(cfg.Telegram.SessionFile))
	return nil
}

func Status(ctx context.Context, cfg config.Config) (bool, error) {
	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.Telegram.SessionFile},
	})

	authorized := false
	err := client.Run(ctx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil {
			return err
		}
		authorized = status.Authorized
		return nil
	})
	if err != nil {
		if errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "no such file") {
			return false, nil
		}
		return false, fmt.Errorf("auth status failed: %w", err)
	}
	return authorized, nil
}

func normalizePhone(phone string, cfg config.Config) string {
	phone = strings.TrimSpace(phone)
	if phone == "" {
		phone = strings.TrimSpace(cfg.Telegram.Phone)
	}
	return phone
}

func sentCodeTypeName(codeType tg.AuthSentCodeTypeClass) string {
	switch codeType.(type) {
	case *tg.AuthSentCodeTypeApp:
		return "app"
	case *tg.AuthSentCodeTypeSMS:
		return "sms"
	case *tg.AuthSentCodeTypeCall:
		return "call"
	case *tg.AuthSentCodeTypeFlashCall:
		return "flash_call"
	case *tg.AuthSentCodeTypeMissedCall:
		return "missed_call"
	case *tg.AuthSentCodeTypeEmailCode:
		return "email"
	case *tg.AuthSentCodeTypeFirebaseSMS:
		return "firebase_sms"
	case *tg.AuthSentCodeTypeSetUpEmailRequired:
		return "setup_email_required"
	default:
		return fmt.Sprintf("unknown(%T)", codeType)
	}
}

func authCodeTypeName(codeType tg.AuthCodeTypeClass) string {
	switch codeType.(type) {
	case *tg.AuthCodeTypeSMS:
		return "sms"
	case *tg.AuthCodeTypeCall:
		return "call"
	case *tg.AuthCodeTypeFlashCall:
		return "flash_call"
	case *tg.AuthCodeTypeMissedCall:
		return "missed_call"
	case *tg.AuthCodeTypeFragmentSMS:
		return "fragment_sms"
	default:
		return fmt.Sprintf("unknown(%T)", codeType)
	}
}
