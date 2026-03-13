package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	Port           int
	LogLevel       string
	SQLitePath     string
	SoulPromptPath string
	AgentsDir      string
	ContextLimit   int

	Telegram struct {
		APIID       int
		APIHash     string
		Phone       string
		SessionFile string
	}

	OpenAI struct {
		BaseURL   string
		APIKey    string
		Model     string
		MaxTokens int
	}

	AutoReply struct {
		Enabled         bool
		PrivateOnly     bool
		IgnoreBots      bool
		IgnoreGroups    bool
		IgnoreMediaOnly bool
		LogContext      bool
		DelayMinMS      int
		DelayMaxMS      int
	}
}

const (
	fixedContextLimit = 20
	fixedAIMaxTokens  = 320
	fixedDelayMinMS   = 3000
	fixedDelayMaxMS   = 10000
	fixedPrivateOnly  = true
	fixedIgnoreBots   = true
	fixedIgnoreGroups = true
	fixedIgnoreMedia  = true
	fixedLogContext   = false
)

func LoadForApp() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{
		Port:           getInt("PORT", 3000),
		LogLevel:       getString("LOG_LEVEL", "info"),
		SQLitePath:     getString("SQLITE_PATH", "./data/app.db"),
		SoulPromptPath: getString("SOUL_PROMPT_PATH", "./SOUL.md"),
		AgentsDir:      getString("AGENTS_DIR", "./agents"),
		ContextLimit:   getPositiveInt("AI_CONTEXT_MESSAGE_LIMIT", fixedContextLimit),
	}

	var err error
	cfg.Telegram.APIID, err = getIntRequired("TG_API_ID")
	if err != nil {
		return Config{}, err
	}
	cfg.Telegram.APIHash, err = getRequired("TG_API_HASH")
	if err != nil {
		return Config{}, err
	}
	cfg.Telegram.SessionFile = getString("TG_SESSION_FILE", "./data/session.json")

	cfg.OpenAI.BaseURL, err = getRequired("OPENAI_BASE_URL")
	if err != nil {
		return Config{}, err
	}
	cfg.OpenAI.APIKey, err = getRequired("OPENAI_API_KEY")
	if err != nil {
		return Config{}, err
	}
	cfg.OpenAI.Model, err = getRequired("OPENAI_MODEL")
	if err != nil {
		return Config{}, err
	}
	cfg.OpenAI.MaxTokens = fixedAIMaxTokens

	cfg.AutoReply.Enabled = getBool("AUTO_REPLY_ENABLED", true)
	cfg.AutoReply.PrivateOnly = fixedPrivateOnly
	cfg.AutoReply.IgnoreBots = fixedIgnoreBots
	cfg.AutoReply.IgnoreGroups = fixedIgnoreGroups
	cfg.AutoReply.IgnoreMediaOnly = fixedIgnoreMedia
	cfg.AutoReply.LogContext = fixedLogContext
	cfg.AutoReply.DelayMinMS = fixedDelayMinMS
	cfg.AutoReply.DelayMaxMS = fixedDelayMaxMS

	return cfg, nil
}

func LoadForLogin() (Config, error) {
	_ = godotenv.Load()

	cfg := Config{}
	var err error
	cfg.Telegram.APIID, err = getIntRequired("TG_API_ID")
	if err != nil {
		return Config{}, err
	}
	cfg.Telegram.APIHash, err = getRequired("TG_API_HASH")
	if err != nil {
		return Config{}, err
	}
	cfg.Telegram.Phone = strings.TrimSpace(os.Getenv("TG_PHONE"))
	cfg.Telegram.SessionFile = getString("TG_SESSION_FILE", "./data/session.json")
	return cfg, nil
}

func getRequired(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("missing required env var: %s", name)
	}
	return value, nil
}

func getIntRequired(name string) (int, error) {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return 0, fmt.Errorf("missing required env var: %s", name)
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("invalid integer env var %s=%q", name, raw)
	}
	return v, nil
}

func getString(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func getInt(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func getPositiveInt(name string, fallback int) int {
	v := getInt(name, fallback)
	if v <= 0 {
		return fallback
	}
	return v
}

func getBool(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	return raw == "true"
}
