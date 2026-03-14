package behavior

import (
	"testing"
	"time"
)

func TestEvaluateEscalatedManualSkipsReply(t *testing.T) {
	decision := Evaluate(time.Date(2026, 3, 14, 10, 0, 0, 0, time.UTC), DefaultPolicy(), RuntimeState{
		ChatID:           "user:1",
		EscalatedManual:  true,
		EscalationReason: "execute_api_tool",
	}, "hello")
	if decision.AllowReply {
		t.Fatalf("expected escalated manual chat to skip auto reply")
	}
	if !decision.ForceManual {
		t.Fatalf("expected force manual when escalated")
	}
	if decision.SkipReason != "execute_api_tool" {
		t.Fatalf("unexpected skip reason: %q", decision.SkipReason)
	}
}

func TestEvaluateQuietHoursAndCooldown(t *testing.T) {
	policy := NormalizePolicy(Policy{
		DebounceSeconds: 5,
		CooldownSeconds: 120,
		QuietHours: []QuietHoursWindow{
			{Start: "22:00", End: "06:00"},
		},
		MaxReplyWords: 25,
		Escalation:    EscalationPolicy{FailureThreshold: 3},
	})

	quietDecision := Evaluate(time.Date(2026, 3, 14, 23, 30, 0, 0, time.UTC), policy, RuntimeState{}, "hello")
	if quietDecision.SkipReason != "quiet_hours" {
		t.Fatalf("expected quiet_hours skip, got %q", quietDecision.SkipReason)
	}

	cooldownDecision := Evaluate(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC), policy, RuntimeState{
		LastAutoReplyAt: time.Date(2026, 3, 14, 11, 59, 10, 0, time.UTC),
	}, "hello again")
	if cooldownDecision.SkipReason != "cooldown_active" {
		t.Fatalf("expected cooldown_active skip, got %q", cooldownDecision.SkipReason)
	}
	if cooldownDecision.Constraints.MaxReplyWords != 25 {
		t.Fatalf("expected constraints to include max reply words")
	}
}

func TestEvaluateTriggerKeywordForcesManual(t *testing.T) {
	policy := DefaultPolicy()
	policy.Escalation.TriggerKeywords = []string{"lawyer", "refund"}

	decision := Evaluate(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC), policy, RuntimeState{}, "I need refund now")
	if decision.AllowReply {
		t.Fatalf("expected trigger keyword to block auto reply")
	}
	if !decision.ForceManual {
		t.Fatalf("expected trigger keyword to force manual")
	}
	if decision.SkipReason != "manual_trigger_keyword" {
		t.Fatalf("unexpected skip reason: %q", decision.SkipReason)
	}
}

func TestApplyOutputConstraintsClampsWords(t *testing.T) {
	got := ApplyOutputConstraints("one two three four five", Constraints{MaxReplyWords: 3}, 10)
	if got != "one two three" {
		t.Fatalf("unexpected clamped output: %q", got)
	}
}

func TestSampleConstraintsVariability(t *testing.T) {
	policy := DefaultPolicy()
	policy.ShortReplyProbability = 0.25
	policy.OneWordReplyProbability = 0.1
	policy.FollowUpProbability = 0.15
	policy.SplitMessageProbability = 0.2

	values := []float64{0.05, 0.2, 0.1, 0.18}
	idx := 0
	constraints := sampleConstraints(policy, func() float64 {
		next := values[idx]
		idx++
		return next
	})
	if !constraints.PreferOneWordReply {
		t.Fatalf("expected one-word reply preference")
	}
	if !constraints.PreferShortReply {
		t.Fatalf("expected one-word reply to imply short reply")
	}
	if constraints.PreferFollowUpQuestion {
		t.Fatalf("one-word reply should suppress follow-up question")
	}
	if constraints.PreferSplitMessages {
		t.Fatalf("one-word reply should suppress split message mode")
	}
}

func TestApplyOutputConstraintsSplitMessages(t *testing.T) {
	got := ApplyOutputConstraints("အေး\nနားမလည်သေးဘူး", Constraints{
		MaxReplyWords:       10,
		PreferSplitMessages: true,
	}, 10)
	if got != "အေး\n\nနားမလည်သေးဘူး" {
		t.Fatalf("unexpected split output: %q", got)
	}

	parts := SplitOutgoingMessages(got)
	if len(parts) != 2 {
		t.Fatalf("expected 2 outgoing parts, got %d (%#v)", len(parts), parts)
	}
}
