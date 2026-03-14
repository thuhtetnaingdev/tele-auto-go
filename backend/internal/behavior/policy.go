package behavior

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
)

type QuietHoursWindow struct {
	Start string `json:"start" yaml:"start"`
	End   string `json:"end" yaml:"end"`
}

type ToneRules struct {
	Allow []string `json:"allow" yaml:"allow"`
	Deny  []string `json:"deny" yaml:"deny"`
}

type EscalationPolicy struct {
	FailureThreshold int      `json:"failureThreshold" yaml:"failureThreshold"`
	TriggerKeywords  []string `json:"triggerKeywords" yaml:"triggerKeywords"`
}

type Policy struct {
	DebounceSeconds         int                `json:"debounceSeconds" yaml:"debounceSeconds"`
	CooldownSeconds         int                `json:"cooldownSeconds" yaml:"cooldownSeconds"`
	QuietHours              []QuietHoursWindow `json:"quietHours" yaml:"quietHours"`
	MaxReplyWords           int                `json:"maxReplyWords" yaml:"maxReplyWords"`
	ShortReplyProbability   float64            `json:"shortReplyProbability" yaml:"shortReplyProbability"`
	OneWordReplyProbability float64            `json:"oneWordReplyProbability" yaml:"oneWordReplyProbability"`
	FollowUpProbability     float64            `json:"followUpProbability" yaml:"followUpProbability"`
	SplitMessageProbability float64            `json:"splitMessageProbability" yaml:"splitMessageProbability"`
	ToneRules               ToneRules          `json:"toneRules" yaml:"toneRules"`
	Escalation              EscalationPolicy   `json:"escalation" yaml:"escalation"`
}

type LoadedPolicy struct {
	Policy   Policy
	Path     string
	LoadedAt time.Time
}

const (
	defaultDebounceSeconds  = 10
	defaultCooldownSeconds  = 0
	defaultMaxReplyWords    = 60
	defaultFailureThreshold = 3
)

func DefaultPolicy() Policy {
	return NormalizePolicy(Policy{
		DebounceSeconds:         defaultDebounceSeconds,
		CooldownSeconds:         defaultCooldownSeconds,
		QuietHours:              []QuietHoursWindow{},
		MaxReplyWords:           defaultMaxReplyWords,
		ShortReplyProbability:   0,
		OneWordReplyProbability: 0,
		FollowUpProbability:     0,
		SplitMessageProbability: 0,
		ToneRules: ToneRules{
			Allow: []string{"natural", "brief", "human"},
			Deny:  []string{"assistant-like", "promotional", "flirty"},
		},
		Escalation: EscalationPolicy{
			FailureThreshold: defaultFailureThreshold,
			TriggerKeywords:  []string{},
		},
	})
}

func NormalizePolicy(policy Policy) Policy {
	policy.DebounceSeconds = clampNonNegative(policy.DebounceSeconds, defaultDebounceSeconds)
	policy.CooldownSeconds = clampNonNegative(policy.CooldownSeconds, defaultCooldownSeconds)
	if policy.MaxReplyWords <= 0 {
		policy.MaxReplyWords = defaultMaxReplyWords
	}
	policy.ShortReplyProbability = clampProbability(policy.ShortReplyProbability)
	policy.OneWordReplyProbability = clampProbability(policy.OneWordReplyProbability)
	policy.FollowUpProbability = clampProbability(policy.FollowUpProbability)
	policy.SplitMessageProbability = clampProbability(policy.SplitMessageProbability)
	policy.ToneRules.Allow = normalizeList(policy.ToneRules.Allow)
	policy.ToneRules.Deny = normalizeList(policy.ToneRules.Deny)
	policy.Escalation.TriggerKeywords = normalizeList(policy.Escalation.TriggerKeywords)
	if policy.Escalation.FailureThreshold <= 0 {
		policy.Escalation.FailureThreshold = defaultFailureThreshold
	}
	policy.QuietHours = normalizeQuietHours(policy.QuietHours)
	return policy
}

func Load(path string, logger *slog.Logger) LoadedPolicy {
	resolved, err := filepath.Abs(path)
	if err != nil {
		resolved = path
	}

	loaded := LoadedPolicy{
		Policy:   DefaultPolicy(),
		Path:     resolved,
		LoadedAt: time.Now().UTC(),
	}

	body, err := os.ReadFile(resolved)
	if err != nil {
		logger.Warn("behavior policy not loaded, using defaults", "path", resolved, "error", err.Error())
		return loaded
	}

	var policy Policy
	if err := yaml.Unmarshal(body, &policy); err != nil {
		logger.Warn("behavior policy parse failed, using defaults", "path", resolved, "error", err.Error())
		return loaded
	}

	loaded.Policy = NormalizePolicy(policy)
	if info, err := os.Stat(resolved); err == nil {
		loaded.LoadedAt = info.ModTime().UTC()
	}
	logger.Info("loaded behavior policy", "path", resolved)
	return loaded
}

func Write(path string, policy Policy) (LoadedPolicy, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		resolved = path
	}
	policy = NormalizePolicy(policy)

	body, err := yaml.Marshal(policy)
	if err != nil {
		return LoadedPolicy{}, fmt.Errorf("marshal behavior policy: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0o755); err != nil {
		return LoadedPolicy{}, fmt.Errorf("mkdir behavior dir: %w", err)
	}
	if err := os.WriteFile(resolved, body, 0o644); err != nil {
		return LoadedPolicy{}, fmt.Errorf("write behavior policy: %w", err)
	}
	return LoadedPolicy{
		Policy:   policy,
		Path:     resolved,
		LoadedAt: time.Now().UTC(),
	}, nil
}

func clampNonNegative(value, fallback int) int {
	if value < 0 {
		return fallback
	}
	return value
}

func clampProbability(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeList(items []string) []string {
	if len(items) == 0 {
		return []string{}
	}
	seen := make(map[string]struct{}, len(items))
	out := make([]string, 0, len(items))
	for _, item := range items {
		trimmed := strings.TrimSpace(item)
		if trimmed == "" {
			continue
		}
		key := strings.ToLower(trimmed)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, trimmed)
	}
	return out
}

func normalizeQuietHours(windows []QuietHoursWindow) []QuietHoursWindow {
	if len(windows) == 0 {
		return []QuietHoursWindow{}
	}
	out := make([]QuietHoursWindow, 0, len(windows))
	for _, window := range windows {
		window.Start = strings.TrimSpace(window.Start)
		window.End = strings.TrimSpace(window.End)
		if !isValidClock(window.Start) || !isValidClock(window.End) {
			continue
		}
		out = append(out, window)
	}
	return out
}

func isValidClock(raw string) bool {
	if len(raw) != 5 {
		return false
	}
	parsed, err := time.Parse("15:04", raw)
	return err == nil && parsed.Format("15:04") == raw
}
