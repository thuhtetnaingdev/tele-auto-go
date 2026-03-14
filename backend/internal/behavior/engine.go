package behavior

import (
	"fmt"
	"math/rand"
	"regexp"
	"strings"
	"time"
)

type Constraints struct {
	MaxReplyWords          int      `json:"maxReplyWords"`
	AllowedTones           []string `json:"allowedTones"`
	DeniedTones            []string `json:"deniedTones"`
	PreferShortReply       bool     `json:"preferShortReply"`
	PreferOneWordReply     bool     `json:"preferOneWordReply"`
	PreferFollowUpQuestion bool     `json:"preferFollowUpQuestion"`
	PreferSplitMessages    bool     `json:"preferSplitMessages"`
}

type RuntimeState struct {
	ChatID                  string    `json:"chatId"`
	LastIncomingAt          time.Time `json:"lastIncomingAt,omitempty"`
	LastAutoReplyAt         time.Time `json:"lastReplyAt,omitempty"`
	DebounceUntil           time.Time `json:"debounceUntil,omitempty"`
	PendingTriggerMessageID string    `json:"pendingTriggerMessageId,omitempty"`
	PendingPreview          string    `json:"pendingPreview,omitempty"`
	ConsecutiveFailures     int       `json:"failureCount"`
	EscalatedManual         bool      `json:"escalatedManual"`
	EscalationReason        string    `json:"reason,omitempty"`
	UpdatedAt               time.Time `json:"updatedAt,omitempty"`
}

type Decision struct {
	AllowReply      bool        `json:"allowReply"`
	SkipReason      string      `json:"skipReason,omitempty"`
	ForceManual     bool        `json:"forceManual"`
	Constraints     Constraints `json:"constraints"`
	DebounceSeconds int         `json:"debounceSeconds"`
}

func Evaluate(now time.Time, policy Policy, state RuntimeState, latestIncoming string) Decision {
	policy = NormalizePolicy(policy)
	decision := Decision{
		AllowReply:      true,
		ForceManual:     state.EscalatedManual,
		DebounceSeconds: policy.DebounceSeconds,
		Constraints:     BaseConstraints(policy),
	}

	if state.EscalatedManual {
		decision.AllowReply = false
		decision.SkipReason = skipReasonFromEscalation(state.EscalationReason)
		return decision
	}

	if keyword := matchedTriggerKeyword(policy.Escalation.TriggerKeywords, latestIncoming); keyword != "" {
		decision.AllowReply = false
		decision.ForceManual = true
		decision.SkipReason = "manual_trigger_keyword"
		return decision
	}

	if isQuietHours(policy.QuietHours, now) {
		decision.AllowReply = false
		decision.SkipReason = "quiet_hours"
		return decision
	}

	if !state.LastAutoReplyAt.IsZero() && policy.CooldownSeconds > 0 {
		if now.Before(state.LastAutoReplyAt.Add(time.Duration(policy.CooldownSeconds) * time.Second)) {
			decision.AllowReply = false
			decision.SkipReason = "cooldown_active"
			return decision
		}
	}

	return decision
}

func BaseConstraints(policy Policy) Constraints {
	policy = NormalizePolicy(policy)
	return Constraints{
		MaxReplyWords: policy.MaxReplyWords,
		AllowedTones:  append([]string{}, policy.ToneRules.Allow...),
		DeniedTones:   append([]string{}, policy.ToneRules.Deny...),
	}
}

func SampleConstraints(policy Policy) Constraints {
	return sampleConstraints(policy, rand.Float64)
}

func sampleConstraints(policy Policy, nextFloat func() float64) Constraints {
	constraints := BaseConstraints(policy)
	policy = NormalizePolicy(policy)

	if nextFloat == nil {
		nextFloat = rand.Float64
	}

	oneWord := nextFloat() < policy.OneWordReplyProbability
	shortReply := oneWord || nextFloat() < policy.ShortReplyProbability
	followUp := !oneWord && nextFloat() < policy.FollowUpProbability
	splitMessages := !oneWord && nextFloat() < policy.SplitMessageProbability

	constraints.PreferOneWordReply = oneWord
	constraints.PreferShortReply = shortReply
	constraints.PreferFollowUpQuestion = followUp
	constraints.PreferSplitMessages = splitMessages
	return constraints
}

func BuildInstructionText(c Constraints) string {
	lines := make([]string, 0, 10)
	maxWords := c.MaxReplyWords
	if c.PreferOneWordReply {
		maxWords = 1
		lines = append(lines, "Prefer a one-word reply if it still feels natural.")
	} else if c.PreferShortReply {
		if maxWords <= 0 || maxWords > 4 {
			maxWords = 4
		}
		lines = append(lines, "Prefer an extra-short reply, ideally just a few words.")
	}
	if maxWords > 0 {
		lines = append(lines, fmt.Sprintf("Keep the final reply under %d words.", maxWords))
	}
	if len(c.AllowedTones) > 0 {
		lines = append(lines, "Prefer tones: "+strings.Join(c.AllowedTones, ", ")+".")
	}
	if len(c.DeniedTones) > 0 {
		lines = append(lines, "Avoid tones: "+strings.Join(c.DeniedTones, ", ")+".")
	}
	if c.PreferFollowUpQuestion {
		lines = append(lines, "If it feels natural, ask one short follow-up question.")
	}
	if c.PreferSplitMessages {
		lines = append(lines, "If it feels natural, format as up to two short Telegram messages separated by a blank line.")
	}
	return strings.TrimSpace(strings.Join(lines, "\n"))
}

func ApplyOutputConstraints(text string, c Constraints, fallbackMax int) string {
	maxWords := effectiveWordLimit(c, fallbackMax)
	if c.PreferSplitMessages && !c.PreferOneWordReply {
		parts := splitTelegramMessages(text)
		if len(parts) == 0 {
			return ""
		}
		partLimit := maxWords
		if len(parts) > 1 && maxWords > 1 {
			partLimit = maxWords / len(parts)
			if partLimit < 1 {
				partLimit = 1
			}
		}
		if c.PreferShortReply && partLimit > 3 {
			partLimit = 3
		}
		for i := range parts {
			parts[i] = normalizeInline(parts[i])
			if partLimit > 0 {
				parts[i] = clampWords(parts[i], partLimit)
			}
		}
		return strings.Join(filterNonEmpty(parts), "\n\n")
	}

	normalized := normalizeInline(text)
	if maxWords <= 0 {
		return normalized
	}
	return strings.TrimSpace(clampWords(normalized, maxWords))
}

func ShouldEscalateAfterFailure(policy Policy, nextFailures int) bool {
	policy = NormalizePolicy(policy)
	return nextFailures >= policy.Escalation.FailureThreshold
}

func isQuietHours(windows []QuietHoursWindow, now time.Time) bool {
	if len(windows) == 0 {
		return false
	}
	currentMinutes := now.Hour()*60 + now.Minute()
	for _, window := range windows {
		start, end, ok := parseWindow(window)
		if !ok {
			continue
		}
		if start == end {
			return true
		}
		if start < end && currentMinutes >= start && currentMinutes < end {
			return true
		}
		if start > end && (currentMinutes >= start || currentMinutes < end) {
			return true
		}
	}
	return false
}

func parseWindow(window QuietHoursWindow) (int, int, bool) {
	startParsed, err := time.Parse("15:04", window.Start)
	if err != nil {
		return 0, 0, false
	}
	endParsed, err := time.Parse("15:04", window.End)
	if err != nil {
		return 0, 0, false
	}
	return startParsed.Hour()*60 + startParsed.Minute(), endParsed.Hour()*60 + endParsed.Minute(), true
}

func matchedTriggerKeyword(keywords []string, latestIncoming string) string {
	text := strings.ToLower(strings.TrimSpace(latestIncoming))
	if text == "" {
		return ""
	}
	for _, keyword := range keywords {
		trimmed := strings.ToLower(strings.TrimSpace(keyword))
		if trimmed != "" && strings.Contains(text, trimmed) {
			return trimmed
		}
	}
	return ""
}

func skipReasonFromEscalation(reason string) string {
	if strings.TrimSpace(reason) == "" {
		return "manual_escalated"
	}
	return reason
}

func clampWords(s string, maxWords int) string {
	parts := strings.Fields(s)
	if len(parts) <= maxWords {
		return strings.Join(parts, " ")
	}
	return strings.Join(parts[:maxWords], " ")
}

func SplitOutgoingMessages(text string) []string {
	return splitTelegramMessages(text)
}

func effectiveWordLimit(c Constraints, fallbackMax int) int {
	maxWords := fallbackMax
	if c.MaxReplyWords > 0 {
		maxWords = c.MaxReplyWords
	}
	if c.PreferOneWordReply {
		return 1
	}
	if c.PreferShortReply && (maxWords <= 0 || maxWords > 4) {
		return 4
	}
	return maxWords
}

var splitSentencePattern = regexp.MustCompile(`(?m)([.!?]\s+)`)

func splitTelegramMessages(text string) []string {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return nil
	}

	segments := make([]string, 0)
	for _, chunk := range strings.Split(trimmed, "\n\n") {
		for _, line := range strings.Split(chunk, "\n") {
			line = normalizeInline(line)
			if line != "" {
				segments = append(segments, line)
			}
		}
	}
	if len(segments) > 1 {
		return segments
	}

	normalized := normalizeInline(trimmed)
	if normalized == "" {
		return nil
	}

	sentences := splitSentencePattern.Split(normalized, -1)
	out := make([]string, 0, 2)
	for _, sentence := range sentences {
		sentence = normalizeInline(sentence)
		if sentence != "" {
			out = append(out, sentence)
		}
		if len(out) == 2 {
			break
		}
	}
	if len(out) > 1 {
		return out
	}
	return []string{normalized}
}

func normalizeInline(text string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
}

func filterNonEmpty(parts []string) []string {
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}
