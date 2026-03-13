package config

import "testing"

func TestGetNonNegativeInt(t *testing.T) {
	t.Setenv("AUTO_REPLY_DEBOUNCE_SECONDS", "0")
	if got := getNonNegativeInt("AUTO_REPLY_DEBOUNCE_SECONDS", 10); got != 0 {
		t.Fatalf("expected 0, got %d", got)
	}

	t.Setenv("AUTO_REPLY_DEBOUNCE_SECONDS", "-5")
	if got := getNonNegativeInt("AUTO_REPLY_DEBOUNCE_SECONDS", 10); got != 10 {
		t.Fatalf("expected fallback 10 for negative input, got %d", got)
	}

	t.Setenv("AUTO_REPLY_DEBOUNCE_SECONDS", "7")
	if got := getNonNegativeInt("AUTO_REPLY_DEBOUNCE_SECONDS", 10); got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}
