package store

import (
	"context"
	"path/filepath"
	"testing"
	"time"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	p := filepath.Join(t.TempDir(), "test.db")
	s, err := Open(p)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestChatModeUpsertAndList(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()

	if err := s.UpsertChatMode(ctx, "user:1", "manual"); err != nil {
		t.Fatalf("upsert manual: %v", err)
	}
	if err := s.UpsertChatMode(ctx, "user:2", "auto"); err != nil {
		t.Fatalf("upsert auto: %v", err)
	}

	mode, ok, err := s.GetChatMode(ctx, "user:1")
	if err != nil {
		t.Fatalf("get mode: %v", err)
	}
	if !ok || mode != "manual" {
		t.Fatalf("unexpected mode: ok=%v mode=%q", ok, mode)
	}

	all, err := s.ListChatModes(ctx)
	if err != nil {
		t.Fatalf("list modes: %v", err)
	}
	if all["user:1"] != "manual" || all["user:2"] != "auto" {
		t.Fatalf("unexpected modes: %#v", all)
	}
}

func TestConversationQueries(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	base := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	err := s.SaveMessages(ctx, []MessageRecord{
		{ChatID: "user:1", TelegramMessageID: "1", SenderName: "Alice", Direction: "other_person", Text: "hello", CreatedAt: base},
		{ChatID: "user:1", TelegramMessageID: "2", SenderName: "me", Direction: "me", Text: "hi", CreatedAt: base.Add(time.Minute)},
		{ChatID: "user:1", TelegramMessageID: "3", SenderName: "Alice", Direction: "other_person", Text: "need help", CreatedAt: base.Add(2 * time.Minute)},
		{ChatID: "user:2", TelegramMessageID: "1", SenderName: "Bob", Direction: "other_person", Text: "yo", CreatedAt: base.Add(3 * time.Minute)},
	})
	if err != nil {
		t.Fatalf("save messages: %v", err)
	}

	summaries, err := s.ListConversationSummaries(ctx, 10)
	if err != nil {
		t.Fatalf("list summaries: %v", err)
	}
	if len(summaries) != 2 {
		t.Fatalf("expected 2 summaries, got %d", len(summaries))
	}
	if summaries[0].ChatID != "user:2" {
		t.Fatalf("expected latest chat first, got %s", summaries[0].ChatID)
	}
	if summaries[1].UnreadIncoming != 1 {
		t.Fatalf("expected unread count 1 for user:1, got %d", summaries[1].UnreadIncoming)
	}

	msgs, err := s.ListConversationMessages(ctx, "user:1", 2, 0)
	if err != nil {
		t.Fatalf("list messages: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(msgs))
	}
	if msgs[0].TelegramMessageID != "2" || msgs[1].TelegramMessageID != "3" {
		t.Fatalf("unexpected message order: %#v", msgs)
	}
}

func TestBehaviorRuntimeEscalationLifecycle(t *testing.T) {
	s := openTestStore(t)
	ctx := context.Background()
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)

	if err := s.UpdateBehaviorPending(ctx, "user:42", now, now.Add(10*time.Second), "99", "hello there"); err != nil {
		t.Fatalf("update behavior pending: %v", err)
	}
	pending, ok, err := s.GetBehaviorRuntimeState(ctx, "user:42")
	if err != nil {
		t.Fatalf("get behavior runtime state: %v", err)
	}
	if !ok || pending.PendingTriggerMessageID != "99" {
		t.Fatalf("unexpected pending state: %#v ok=%v", pending, ok)
	}

	failed, err := s.MarkBehaviorFailure(ctx, "user:42", "ai_error", true)
	if err != nil {
		t.Fatalf("mark behavior failure: %v", err)
	}
	if failed.ConsecutiveFailures != 1 || !failed.EscalatedManual {
		t.Fatalf("unexpected failed state: %#v", failed)
	}
	if !failed.DebounceUntil.IsZero() || failed.PendingTriggerMessageID != "" {
		t.Fatalf("expected pending debounce state to clear on failure: %#v", failed)
	}

	if err := s.ClearBehaviorEscalation(ctx, "user:42"); err != nil {
		t.Fatalf("clear behavior escalation: %v", err)
	}
	cleared, ok, err := s.GetBehaviorRuntimeState(ctx, "user:42")
	if err != nil {
		t.Fatalf("reload behavior state: %v", err)
	}
	if !ok || cleared.EscalatedManual || cleared.ConsecutiveFailures != 0 {
		t.Fatalf("unexpected cleared state: %#v", cleared)
	}
}
