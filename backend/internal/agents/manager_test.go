package agents

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestParseAgentFileDefaultsVisibilityToPublic(t *testing.T) {
	t.Parallel()

	content := `---
id: test_agent
name: Test Agent
description: test
intents:
- test
tools:
- api_call
variables: []
temperature: 0.35
---

test body
`
	path := filepath.Join(t.TempDir(), "test_agent.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	agent, err := parseAgentFile(path)
	if err != nil {
		t.Fatalf("parseAgentFile: %v", err)
	}
	if agent.Visibility != VisibilityPublic {
		t.Fatalf("expected visibility %q, got %q", VisibilityPublic, agent.Visibility)
	}
	if len(agent.AllowUsers) != 0 {
		t.Fatalf("expected no allow users, got %+v", agent.AllowUsers)
	}
}

func TestValidateInputPrivateRequiresAllowUsers(t *testing.T) {
	t.Parallel()

	input := UpsertAgentInput{
		ID:          "private_agent",
		Name:        "Private Agent",
		Description: "test",
		Intents:     []string{"private"},
		Tools:       []string{"api_call"},
		Variables:   []string{},
		Visibility:  VisibilityPrivate,
		AllowUsers:  []string{},
		Body:        "test body",
		Temperature: 0.35,
	}
	if err := validateInput(input); err == nil {
		t.Fatalf("expected private validation error for empty allow users")
	}
}

func TestBuildAndParseRoundTripNormalizesAllowUsers(t *testing.T) {
	t.Parallel()

	input := UpsertAgentInput{
		ID:          "private_agent",
		Name:        "Private Agent",
		Description: "test",
		Intents:     []string{"private"},
		Tools:       []string{"api_call"},
		Variables:   []string{},
		Visibility:  VisibilityPrivate,
		AllowUsers:  []string{"@Alice", "alice", "00123", "123"},
		Body:        "test body",
		Temperature: 0.35,
	}

	markdown, err := buildMarkdown(input)
	if err != nil {
		t.Fatalf("buildMarkdown: %v", err)
	}
	path := filepath.Join(t.TempDir(), "private_agent.md")
	if err := os.WriteFile(path, []byte(markdown), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}

	agent, err := parseAgentFile(path)
	if err != nil {
		t.Fatalf("parseAgentFile: %v", err)
	}

	expected := []string{"123", "alice"}
	if !reflect.DeepEqual(agent.AllowUsers, expected) {
		t.Fatalf("expected allow users %+v, got %+v", expected, agent.AllowUsers)
	}
	if agent.Visibility != VisibilityPrivate {
		t.Fatalf("expected visibility %q, got %q", VisibilityPrivate, agent.Visibility)
	}
}
