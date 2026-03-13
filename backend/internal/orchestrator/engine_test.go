package orchestrator

import (
	"testing"

	"tele-auto-go/internal/agents"
)

func TestParseJSONLikeRepairsEscapedPlannerJSON(t *testing.T) {
	raw := `{"call_tool":false,"tool":"api_call","tool_args":{"method":"GET","url":"","headers":{},\"query\":{},\"json_body\":{},\"timeout_ms\":30000},\"direct_response\":\"Who are you?\"}`

	var decision toolDecision
	if err := parseJSONLike(raw, &decision); err != nil {
		t.Fatalf("parseJSONLike returned error: %v", err)
	}
	if decision.CallTool {
		t.Fatalf("expected call_tool to be false")
	}
	if decision.DirectResponse != "Who are you?" {
		t.Fatalf("unexpected direct_response: %q", decision.DirectResponse)
	}
}

func TestParseJSONLikeStringifiesQueryAndHeaderValues(t *testing.T) {
	raw := `{"call_tool":true,"tool":"api_call","tool_args":{"method":"GET","url":"https://example.com","headers":{"X-Trace":123},"query":{"limit":10,"page":1,"enabled":true},"json_body":null,"timeout_ms":10000},"direct_response":""}`

	var decision toolDecision
	if err := parseJSONLike(raw, &decision); err != nil {
		t.Fatalf("parseJSONLike returned error: %v", err)
	}
	if decision.ToolArgs.Headers["X-Trace"] != "123" {
		t.Fatalf("expected header to be stringified, got %q", decision.ToolArgs.Headers["X-Trace"])
	}
	if decision.ToolArgs.Query["limit"] != "10" {
		t.Fatalf("expected limit query to be stringified, got %q", decision.ToolArgs.Query["limit"])
	}
	if decision.ToolArgs.Query["page"] != "1" {
		t.Fatalf("expected page query to be stringified, got %q", decision.ToolArgs.Query["page"])
	}
	if decision.ToolArgs.Query["enabled"] != "true" {
		t.Fatalf("expected enabled query to be stringified, got %q", decision.ToolArgs.Query["enabled"])
	}
}

func TestFilterAccessibleAgentsByIDAndUsername(t *testing.T) {
	list := []agents.Agent{
		{ID: "general_assistant", Visibility: agents.VisibilityPublic},
		{ID: "po_agent", Visibility: agents.VisibilityPrivate, AllowUsers: []string{"123", "alice"}},
		{ID: "ops_agent", Visibility: agents.VisibilityPrivate, AllowUsers: []string{"999"}},
	}

	allowedByID, deniedByID := filterAccessibleAgents(list, "123", "")
	if deniedByID != 1 {
		t.Fatalf("expected 1 denied private agent, got %d", deniedByID)
	}
	if len(allowedByID) != 2 {
		t.Fatalf("expected 2 allowed agents for user id, got %d", len(allowedByID))
	}

	allowedByUsername, deniedByUsername := filterAccessibleAgents(list, "", "@ALICE")
	if deniedByUsername != 1 {
		t.Fatalf("expected 1 denied private agent, got %d", deniedByUsername)
	}
	if len(allowedByUsername) != 2 {
		t.Fatalf("expected 2 allowed agents for username, got %d", len(allowedByUsername))
	}
}

func TestFilterAccessibleAgentsFallbackToPublic(t *testing.T) {
	list := []agents.Agent{
		{ID: "general_assistant", Visibility: agents.VisibilityPublic},
		{ID: "private_only", Visibility: agents.VisibilityPrivate, AllowUsers: []string{"777"}},
	}

	allowed, denied := filterAccessibleAgents(list, "888", "other_user")
	if denied != 1 {
		t.Fatalf("expected denied private count 1, got %d", denied)
	}
	if len(allowed) != 1 || allowed[0].ID != "general_assistant" {
		t.Fatalf("expected public fallback agent only, got %+v", allowed)
	}
}
