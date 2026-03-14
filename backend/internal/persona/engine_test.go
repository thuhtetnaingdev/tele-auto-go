package persona

import (
	"context"
	"log/slog"
	"path/filepath"
	"testing"

	"tele-auto-go/internal/store"
)

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	p := filepath.Join(t.TempDir(), "persona-test.db")
	s, err := store.Open(p)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

func TestResolvePrecedenceUserOverrideWins(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	root := t.TempDir()
	engine := NewEngine(db, slog.Default(), Options{
		GroupRoot: filepath.Join(root, "groups"),
		UserRoot:  filepath.Join(root, "users"),
	})

	group, err := db.CreatePersonaGroup(ctx, store.PersonaGroupInput{
		Name:         "Manager",
		Slug:         "manager",
		MarkdownPath: engine.DefaultGroupMarkdownPath("manager"),
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := db.CreatePersonaGroupMember(ctx, store.PersonaGroupMemberInput{GroupID: group.ID, UserID: "123"}); err != nil {
		t.Fatalf("create group member: %v", err)
	}
	if _, err := engine.WriteGroupMarkdown(group.MarkdownPath, "Speak like manager"); err != nil {
		t.Fatalf("write group markdown: %v", err)
	}

	profile, err := db.CreatePersonaUserProfile(ctx, store.PersonaUserProfileInput{
		Label:        "Best Friend",
		UserID:       "123",
		MarkdownPath: engine.DefaultUserMarkdownPath("best friend", ""),
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create user profile: %v", err)
	}
	if _, err := engine.WriteUserMarkdown(profile.MarkdownPath, "Speak like best friend"); err != nil {
		t.Fatalf("write user markdown: %v", err)
	}

	resolved, err := engine.Resolve(ctx, ResolveInput{ChatID: "user:123", UserID: "123", Username: "@alice"}, "base soul")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Source != "user_override" {
		t.Fatalf("expected user_override, got %q", resolved.Source)
	}
	if resolved.UserProfileID == "" {
		t.Fatalf("expected user profile id in resolved result")
	}
	if resolved.GroupID != "" {
		t.Fatalf("did not expect group id when user override applied")
	}
}

func TestResolveFallbackToGroupWhenUserMarkdownMissing(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	root := t.TempDir()
	engine := NewEngine(db, slog.Default(), Options{
		GroupRoot: filepath.Join(root, "groups"),
		UserRoot:  filepath.Join(root, "users"),
	})

	group, err := db.CreatePersonaGroup(ctx, store.PersonaGroupInput{
		Name:         "Admin",
		Slug:         "admin",
		MarkdownPath: engine.DefaultGroupMarkdownPath("admin"),
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := db.CreatePersonaGroupMember(ctx, store.PersonaGroupMemberInput{GroupID: group.ID, Username: "alice"}); err != nil {
		t.Fatalf("create group member: %v", err)
	}
	if _, err := engine.WriteGroupMarkdown(group.MarkdownPath, "Speak like admin"); err != nil {
		t.Fatalf("write group markdown: %v", err)
	}

	if _, err := db.CreatePersonaUserProfile(ctx, store.PersonaUserProfileInput{
		Label:        "Broken Profile",
		Username:     "alice",
		MarkdownPath: filepath.Join(engine.UserRoot(), "missing.md"),
		Enabled:      true,
	}); err != nil {
		t.Fatalf("create user profile: %v", err)
	}

	resolved, err := engine.Resolve(ctx, ResolveInput{ChatID: "user:200", Username: "@alice"}, "base soul")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Source != "group" {
		t.Fatalf("expected group fallback, got %q", resolved.Source)
	}
	if resolved.GroupID == "" {
		t.Fatalf("expected group id to be present")
	}
}

func TestResolveUsesUserIDBeforeUsername(t *testing.T) {
	ctx := context.Background()
	db := openTestStore(t)
	root := t.TempDir()
	engine := NewEngine(db, slog.Default(), Options{
		GroupRoot: filepath.Join(root, "groups"),
		UserRoot:  filepath.Join(root, "users"),
	})

	profile, err := db.CreatePersonaUserProfile(ctx, store.PersonaUserProfileInput{
		Label:        "Teacher",
		UserID:       "777",
		MarkdownPath: engine.DefaultUserMarkdownPath("teacher", ""),
		Enabled:      true,
	})
	if err != nil {
		t.Fatalf("create profile: %v", err)
	}
	if _, err := engine.WriteUserMarkdown(profile.MarkdownPath, "Teacher tone"); err != nil {
		t.Fatalf("write user markdown: %v", err)
	}

	group, err := db.CreatePersonaGroup(ctx, store.PersonaGroupInput{
		Name:         "Manager",
		Slug:         "manager",
		MarkdownPath: engine.DefaultGroupMarkdownPath("manager"),
	})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := db.CreatePersonaGroupMember(ctx, store.PersonaGroupMemberInput{GroupID: group.ID, Username: "bob"}); err != nil {
		t.Fatalf("create group member: %v", err)
	}
	if _, err := engine.WriteGroupMarkdown(group.MarkdownPath, "Manager tone"); err != nil {
		t.Fatalf("write group markdown: %v", err)
	}

	resolved, err := engine.Resolve(ctx, ResolveInput{ChatID: "user:777", UserID: "777", Username: "@bob"}, "base soul")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if resolved.Source != "user_override" {
		t.Fatalf("expected user_override, got %q", resolved.Source)
	}
}
