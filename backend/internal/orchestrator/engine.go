package orchestrator

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"tele-auto-go/internal/agents"
	"tele-auto-go/internal/ai"
	"tele-auto-go/internal/contextbuilder"
	"tele-auto-go/internal/store"
	"tele-auto-go/internal/util"
)

type RouterResult struct {
	AgentID    string  `json:"agent_id"`
	Reason     string  `json:"reason,omitempty"`
	Confidence float64 `json:"confidence,omitempty"`
}

type MessageContext struct {
	ChatID          string
	ChatName        string
	LatestIncoming  string
	RecentMessages  []contextbuilder.MessageLine
	TriggerMessage  string
	TriggerUserID   string
	TriggerUsername string
}

type Engine struct {
	ai     *ai.Client
	agents *agents.Manager
	store  *store.Store
	logger *slog.Logger
	http   *http.Client
}

func New(aiClient *ai.Client, agentManager *agents.Manager, st *store.Store, logger *slog.Logger) *Engine {
	return &Engine{
		ai:     aiClient,
		agents: agentManager,
		store:  st,
		logger: logger,
		http: &http.Client{
			Timeout: 20 * time.Second,
		},
	}
}

func (e *Engine) Handle(ctx context.Context, mc MessageContext, soulPrompt string) (string, error) {
	started := time.Now()
	run := store.OrchestrationRun{
		ChatID:         mc.ChatID,
		TriggerMessage: mc.TriggerMessage,
		Status:         "failed",
	}

	defer func() {
		run.DurationMS = int64(time.Since(started).Milliseconds())
		if err := e.store.SaveOrchestrationRun(context.Background(), run); err != nil {
			e.logger.Warn("failed to save orchestration run", "error", err.Error())
		}
	}()

	agentList := e.agents.List()
	if len(agentList) == 0 {
		run.ErrorMessage = "no agents configured"
		return e.fail(mc, run, started, "load_agents", "Sorry, no agent is configured yet.")
	}

	vars, types, err := e.store.GlobalVariablesMap(ctx)
	if err != nil {
		run.ErrorMessage = "load variables: " + err.Error()
		return e.fail(mc, run, started, "load_variables", "Sorry, failed to load runtime variables.")
	}

	route, err := e.routeAgent(ctx, mc, agentList)
	if err != nil {
		run.ErrorMessage = "route agent: " + err.Error()
		return e.fail(mc, run, started, "route_agent", "Sorry, I couldn't decide the right handler right now.")
	}
	run.SelectedAgentID = route.AgentID
	run.RouterReason = route.Reason
	run.RouterConfidence = route.Confidence
	e.logger.Info("orchestrator_routed",
		"chat_id", mc.ChatID,
		"trigger_message", mc.TriggerMessage,
		"agent_id", route.AgentID,
		"reason", route.Reason,
		"confidence", route.Confidence,
	)

	agent, ok := e.agents.Get(route.AgentID)
	if !ok {
		run.ErrorMessage = "agent not found: " + route.AgentID
		return e.fail(mc, run, started, "load_selected_agent", "Sorry, selected agent is unavailable.")
	}

	if err := ensureRequiredVariables(agent.Variables, vars); err != nil {
		e.logger.Warn("orchestrator missing required variables", "agent_id", agent.ID, "error", err.Error())
		run.ErrorMessage = err.Error()
		return e.fail(mc, run, started, "validate_required_variables", "Sorry, system configuration is incomplete for this request.")
	}

	agentInstructions, err := interpolateVars(agent.Body, vars)
	if err != nil {
		run.ErrorMessage = "interpolate agent body: " + err.Error()
		return e.fail(mc, run, started, "interpolate_agent_body", "Sorry, failed to load agent instructions.")
	}
	e.logger.Info("orchestrator_agent_loaded",
		"chat_id", mc.ChatID,
		"trigger_message", mc.TriggerMessage,
		"agent_id", agent.ID,
		"agent_updated_at", agent.UpdatedAt,
		"agent_body_preview", truncateText(agentInstructions, 260),
	)

	decision, err := e.decideToolCall(ctx, agent, agentInstructions, mc, soulPrompt)
	if err != nil {
		run.ErrorMessage = "tool decide: " + err.Error()
		return e.fail(mc, run, started, "decide_tool_call", "Sorry, failed to process request.")
	}
	e.logger.Info("orchestrator_tool_decision",
		"chat_id", mc.ChatID,
		"trigger_message", mc.TriggerMessage,
		"agent_id", agent.ID,
		"call_tool", decision.CallTool,
		"tool", decision.Tool,
		"tool_args_method", strings.ToUpper(strings.TrimSpace(decision.ToolArgs.Method)),
		"tool_args_url", decision.ToolArgs.URL,
		"tool_args_query", decision.ToolArgs.Query,
		"tool_args_headers", redactHeaders(decision.ToolArgs.Headers),
		"tool_args_headers_raw", decision.ToolArgs.Headers,
		"tool_args_json_body", redactAny(decision.ToolArgs.JSONBody),
		"tool_args_json_body_raw", decision.ToolArgs.JSONBody,
		"direct_response_preview", truncateText(decision.DirectResponse, 240),
	)

	toolResp := apiToolResponse{}
	if decision.CallTool {
		toolResp, err = e.executeAPITool(ctx, decision.ToolArgs, vars, agent.ID, mc.ChatID)
		run.ToolName = "api_call"
		run.ToolStatus = strconv.Itoa(toolResp.Status)
		if err != nil {
			run.ErrorMessage = "api_call: " + err.Error()
			return e.fail(mc, run, started, "execute_api_tool", "Sorry, external API call failed right now.")
		}
	}

	finalReply, err := e.synthesize(ctx, agent, agentInstructions, mc, decision, toolResp, soulPrompt, types)
	if err != nil {
		run.ErrorMessage = "synthesize: " + err.Error()
		return e.fail(mc, run, started, "synthesize_reply", "Sorry, failed to compose final response.")
	}

	finalReply = util.NormalizeSpace(util.StripThinking(finalReply))
	if finalReply == "" {
		run.ErrorMessage = "empty final reply"
		return e.fail(mc, run, started, "empty_final_reply", "")
	}

	run.Status = "success"
	run.FinalReply = finalReply
	e.logger.Info("orchestrator_run",
		"chat_id", mc.ChatID,
		"agent_id", run.SelectedAgentID,
		"tool", run.ToolName,
		"status", run.Status,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return util.ClampWords(finalReply, 80), nil
}

type toolDecision struct {
	CallTool       bool        `json:"call_tool"`
	Tool           string      `json:"tool"`
	ToolArgs       apiToolArgs `json:"tool_args"`
	DirectResponse string      `json:"direct_response"`
}

type apiToolArgs struct {
	Method    string            `json:"method"`
	URL       string            `json:"url"`
	Headers   map[string]string `json:"headers"`
	Query     map[string]string `json:"query"`
	JSONBody  map[string]any    `json:"json_body"`
	TimeoutMS int               `json:"timeout_ms"`
}

type apiToolResponse struct {
	Status   int               `json:"status"`
	Headers  map[string]string `json:"headers"`
	BodyText string            `json:"body_text"`
	BodyJSON any               `json:"body_json,omitempty"`
}

func (e *Engine) routeAgent(ctx context.Context, mc MessageContext, agentsList []agents.Agent) (RouterResult, error) {
	if routed, ok := routeByKeywords(mc.LatestIncoming, agentsList); ok {
		return routed, nil
	}

	type lite struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		Description string   `json:"description"`
		Intents     []string `json:"intents"`
		BodyHint    string   `json:"body_hint,omitempty"`
	}
	catalog := make([]lite, 0, len(agentsList))
	for _, agent := range agentsList {
		catalog = append(catalog, lite{
			ID:          agent.ID,
			Name:        agent.Name,
			Description: agent.Description,
			Intents:     agent.Intents,
			BodyHint:    truncateText(agent.Body, 260),
		})
	}
	cb, _ := json.Marshal(catalog)

	systemPrompt := strings.Join([]string{
		"You are a strict agent router.",
		"Return only JSON: {\"agent_id\": string, \"reason\": string, \"confidence\": number}.",
		"agent_id must be exactly one from catalog.",
		"Prefer the most specific domain agent when message indicates that domain.",
		"Use general_assistant only as fallback for broad/unknown requests.",
	}, "\n")
	userPrompt := strings.Join([]string{
		"User message:",
		mc.LatestIncoming,
		"",
		"Agents catalog JSON:",
		string(cb),
	}, "\n")

	out, err := e.ai.Chat(ctx, ai.ChatParams{SystemPrompt: systemPrompt, UserPrompt: userPrompt, Temperature: 0.1, MaxTokens: 220})
	if err != nil {
		return RouterResult{}, err
	}

	parsed := RouterResult{}
	if err := parseJSONLike(out, &parsed); err != nil {
		return RouterResult{}, fmt.Errorf("parse router output: %w", err)
	}
	parsed.AgentID = strings.TrimSpace(parsed.AgentID)
	if parsed.AgentID == "" {
		return RouterResult{}, fmt.Errorf("router returned empty agent_id")
	}
	for _, agent := range agentsList {
		if agent.ID == parsed.AgentID {
			return parsed, nil
		}
	}
	return RouterResult{}, fmt.Errorf("router selected unknown agent: %s", parsed.AgentID)
}

func (e *Engine) decideToolCall(ctx context.Context, agent agents.Agent, agentInstructions string, mc MessageContext, soulPrompt string) (toolDecision, error) {
	history := make([]string, 0, len(mc.RecentMessages))
	for i, line := range mc.RecentMessages {
		history = append(history, fmt.Sprintf("%d. [%s] %s", i+1, line.Direction, line.Text))
	}

	systemPrompt := strings.Join([]string{
		"You are an execution planner for one agent.",
		"Return only JSON with shape:",
		"{\"call_tool\":bool,\"tool\":\"api_call\",\"tool_args\":{\"method\":\"GET|POST|PUT|PATCH|DELETE\",\"url\":string,\"headers\":object,\"query\":object,\"json_body\":object,\"timeout_ms\":number},\"direct_response\":string}",
		"If data lookup/action is needed, set call_tool=true and fill tool_args.",
		"If no tool is needed, set call_tool=false and provide direct_response.",
		"Do not include markdown.",
	}, "\n")

	userPrompt := strings.Join([]string{
		"Agent profile:",
		agentInstructions,
		"",
		"SOUL prompt:",
		soulPrompt,
		"",
		"Recent conversation:",
		strings.Join(history, "\n"),
		"",
		"Latest incoming:",
		mc.LatestIncoming,
	}, "\n")

	model := agent.Model
	if strings.TrimSpace(model) == "" {
		model = ""
	}
	temp := agent.Temperature
	if temp == 0 {
		temp = 0.2
	}

	out, err := e.ai.Chat(ctx, ai.ChatParams{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Model:        model,
		Temperature:  temp,
		MaxTokens:    600,
	})
	if err != nil {
		return toolDecision{}, err
	}

	decision := toolDecision{}
	if err := parseJSONLike(out, &decision); err != nil {
		rawDecision := map[string]any{}
		if parseJSONLike(out, &rawDecision) == nil {
			e.logger.Warn("orchestrator_tool_decision_parse_failed",
				"chat_id", mc.ChatID,
				"agent_id", agent.ID,
				"error", err.Error(),
				"raw_decision", rawDecision,
			)
		} else {
			e.logger.Warn("orchestrator_tool_decision_parse_failed",
				"chat_id", mc.ChatID,
				"agent_id", agent.ID,
				"error", err.Error(),
				"raw_output", truncateText(out, 4000),
			)
		}
		return toolDecision{}, err
	}
	if strings.TrimSpace(decision.Tool) == "" {
		decision.Tool = "api_call"
	}
	if decision.CallTool && decision.Tool != "api_call" {
		return toolDecision{}, fmt.Errorf("unsupported tool requested: %s", decision.Tool)
	}
	return decision, nil
}

func (e *Engine) synthesize(
	ctx context.Context,
	agent agents.Agent,
	agentInstructions string,
	mc MessageContext,
	decision toolDecision,
	toolResp apiToolResponse,
	soulPrompt string,
	varTypes map[string]string,
) (string, error) {
	if !decision.CallTool {
		if strings.TrimSpace(decision.DirectResponse) != "" {
			return decision.DirectResponse, nil
		}
	}

	toolJSON, _ := json.Marshal(toolResp)
	varsJSON, _ := json.Marshal(varTypes)

	systemPrompt := strings.Join([]string{
		"You are a Telegram reply composer.",
		"Return only final human message text in same language as user.",
		"No markdown, no JSON, no labels.",
	}, "\n")

	userPrompt := strings.Join([]string{
		"Agent profile:",
		agentInstructions,
		"",
		"SOUL:",
		soulPrompt,
		"",
		"Variable types:",
		string(varsJSON),
		"",
		"Latest incoming user message:",
		mc.LatestIncoming,
		"",
		"Tool decision:",
		fmt.Sprintf("call_tool=%t", decision.CallTool),
		"",
		"Tool response JSON:",
		string(toolJSON),
	}, "\n")

	return e.ai.Chat(ctx, ai.ChatParams{
		SystemPrompt: systemPrompt,
		UserPrompt:   userPrompt,
		Model:        agent.Model,
		Temperature:  maxFloat(agent.Temperature, 0.35),
		MaxTokens:    320,
	})
}

func (e *Engine) executeAPITool(ctx context.Context, args apiToolArgs, vars map[string]string, agentID, chatID string) (apiToolResponse, error) {
	started := time.Now()
	method := strings.ToUpper(strings.TrimSpace(args.Method))
	if method == "" {
		method = http.MethodGet
	}
	if args.TimeoutMS <= 0 {
		args.TimeoutMS = 15000
	}
	if args.TimeoutMS > 60000 {
		args.TimeoutMS = 60000
	}

	urlValue, err := interpolateVars(args.URL, vars)
	if err != nil {
		return apiToolResponse{}, err
	}
	if strings.TrimSpace(urlValue) == "" {
		return apiToolResponse{}, fmt.Errorf("tool arg url is required")
	}

	headers := map[string]string{}
	for k, v := range args.Headers {
		r, err := interpolateVars(v, vars)
		if err != nil {
			return apiToolResponse{}, err
		}
		headers[k] = r
	}
	query := map[string]string{}
	for k, v := range args.Query {
		r, err := interpolateVars(v, vars)
		if err != nil {
			return apiToolResponse{}, err
		}
		query[k] = r
	}

	resolvedBody, err := interpolateAny(args.JSONBody, vars)
	if err != nil {
		return apiToolResponse{}, err
	}

	e.logger.Info("agent_tool_call_request",
		"chat_id", chatID,
		"agent_id", agentID,
		"tool", "api_call",
		"method", method,
		"url", urlValue,
		"query", query,
		"headers", redactHeaders(headers),
		"headers_raw", headers,
		"json_body", redactAny(resolvedBody),
		"json_body_raw", resolvedBody,
		"timeout_ms", args.TimeoutMS,
	)

	bodyBytes, _ := json.Marshal(resolvedBody)
	if method == http.MethodGet || method == http.MethodDelete {
		bodyBytes = nil
	}

	req, err := http.NewRequestWithContext(ctx, method, urlValue, bytes.NewReader(bodyBytes))
	if err != nil {
		return apiToolResponse{}, err
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	q := req.URL.Query()
	for k, v := range query {
		q.Set(k, v)
	}
	req.URL.RawQuery = q.Encode()

	httpClient := *e.http
	httpClient.Timeout = time.Duration(args.TimeoutMS) * time.Millisecond
	resp, err := httpClient.Do(req)
	if err != nil {
		e.logger.Error("agent_tool_call_error",
			"chat_id", chatID,
			"agent_id", agentID,
			"tool", "api_call",
			"method", method,
			"url", urlValue,
			"query", query,
			"headers", redactHeaders(headers),
			"headers_raw", headers,
			"json_body", redactAny(resolvedBody),
			"json_body_raw", resolvedBody,
			"error", err.Error(),
			"duration_ms", time.Since(started).Milliseconds(),
		)
		return apiToolResponse{}, err
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return apiToolResponse{}, err
	}

	normalizedHeaders := map[string]string{}
	keys := make([]string, 0, len(resp.Header))
	for k := range resp.Header {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		normalizedHeaders[k] = strings.Join(resp.Header.Values(k), ",")
	}

	out := apiToolResponse{
		Status:   resp.StatusCode,
		Headers:  normalizedHeaders,
		BodyText: string(respBody),
	}
	var parsed any
	if json.Unmarshal(respBody, &parsed) == nil {
		out.BodyJSON = parsed
	}

	e.logger.Info("agent_tool_call_response",
		"chat_id", chatID,
		"agent_id", agentID,
		"tool", "api_call",
		"method", method,
		"url", urlValue,
		"status", out.Status,
		"response_headers", normalizedHeaders,
		"response_body_json", redactAny(out.BodyJSON),
		"response_body_text", truncateText(out.BodyText, 2000),
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return out, nil
}

func ensureRequiredVariables(required []string, vars map[string]string) error {
	for _, key := range required {
		if strings.TrimSpace(vars[key]) == "" {
			return fmt.Errorf("missing required variable: %s", key)
		}
	}
	return nil
}

var varPattern = regexp.MustCompile(`\$\{([A-Z0-9_]+)\}`)

func interpolateVars(text string, vars map[string]string) (string, error) {
	missing := ""
	out := varPattern.ReplaceAllStringFunc(text, func(match string) string {
		if missing != "" {
			return match
		}
		groups := varPattern.FindStringSubmatch(match)
		if len(groups) != 2 {
			return match
		}
		key := groups[1]
		v, ok := vars[key]
		if !ok {
			missing = key
			return match
		}
		return v
	})
	if missing != "" {
		return "", fmt.Errorf("missing variable: %s", missing)
	}
	return out, nil
}

func interpolateAny(value any, vars map[string]string) (any, error) {
	switch t := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for k, v := range t {
			r, err := interpolateAny(v, vars)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			r, err := interpolateAny(item, vars)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, nil
	case string:
		return interpolateVars(t, vars)
	default:
		return value, nil
	}
}

func parseJSONLike(raw string, out any) error {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return fmt.Errorf("empty output")
	}
	if err := json.Unmarshal([]byte(raw), out); err == nil {
		return nil
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		return json.Unmarshal([]byte(raw[start:end+1]), out)
	}
	return fmt.Errorf("output is not valid json")
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func (e *Engine) fail(mc MessageContext, run store.OrchestrationRun, started time.Time, stage, userReply string) (string, error) {
	e.logger.Warn("orchestrator_failure",
		"chat_id", mc.ChatID,
		"trigger_message", mc.TriggerMessage,
		"stage", stage,
		"agent_id", run.SelectedAgentID,
		"tool", run.ToolName,
		"tool_status", run.ToolStatus,
		"error", run.ErrorMessage,
		"duration_ms", time.Since(started).Milliseconds(),
	)
	return userReply, nil
}

func routeByKeywords(message string, agentsList []agents.Agent) (RouterResult, bool) {
	if len(agentsList) == 0 {
		return RouterResult{}, false
	}
	msgNorm := normalizeRouteText(message)
	if msgNorm == "" {
		return RouterResult{}, false
	}
	msgWords := wordsSet(msgNorm)
	bestScore := 0
	bestAgent := agents.Agent{}
	bestMatches := []string{}

	for _, agent := range agentsList {
		score, matches := scoreAgentKeywords(msgNorm, msgWords, agent)
		if score == 0 {
			continue
		}
		if score > bestScore || (score == bestScore && preferAgent(agent, bestAgent)) {
			bestScore = score
			bestAgent = agent
			bestMatches = matches
		}
	}

	if bestScore < 3 || strings.TrimSpace(bestAgent.ID) == "" {
		return RouterResult{}, false
	}
	conf := 0.55 + (float64(bestScore) * 0.06)
	if conf > 0.95 {
		conf = 0.95
	}
	return RouterResult{
		AgentID:    bestAgent.ID,
		Reason:     fmt.Sprintf("keyword route matched %s (score=%d)", strings.Join(bestMatches, ","), bestScore),
		Confidence: conf,
	}, true
}

func scoreAgentKeywords(msgNorm string, msgWords map[string]struct{}, agent agents.Agent) (int, []string) {
	score := 0
	matches := []string{}

	for _, intent := range agent.Intents {
		phrase := normalizeRouteText(strings.ReplaceAll(intent, "_", " "))
		if phrase == "" {
			continue
		}
		if containsRoutePhrase(msgNorm, phrase) {
			score += 6
			matches = append(matches, "intent:"+intent)
		}
		tokens := strings.Fields(phrase)
		acr := acronym(tokens)
		if len(acr) >= 2 {
			if _, ok := msgWords[acr]; ok {
				score += 4
				matches = append(matches, "acronym:"+acr)
			}
		}
		for _, token := range tokens {
			if len(token) < 4 {
				continue
			}
			if _, ok := msgWords[token]; ok {
				score += 1
				matches = append(matches, "token:"+token)
			}
		}
	}

	idPhrase := normalizeRouteText(strings.ReplaceAll(agent.ID, "_", " "))
	if idPhrase != "" && containsRoutePhrase(msgNorm, idPhrase) {
		score += 4
		matches = append(matches, "id")
	}
	namePhrase := normalizeRouteText(agent.Name)
	if namePhrase != "" && containsRoutePhrase(msgNorm, namePhrase) {
		score += 3
		matches = append(matches, "name")
	}

	return score, dedupeStrings(matches)
}

func preferAgent(candidate, current agents.Agent) bool {
	if strings.TrimSpace(current.ID) == "" {
		return true
	}
	cCandidate := strings.Contains(strings.ToLower(candidate.ID), "general")
	cCurrent := strings.Contains(strings.ToLower(current.ID), "general")
	if cCandidate == cCurrent {
		return strings.TrimSpace(candidate.ID) < strings.TrimSpace(current.ID)
	}
	return !cCandidate && cCurrent
}

func normalizeRouteText(s string) string {
	lower := strings.ToLower(strings.TrimSpace(s))
	if lower == "" {
		return ""
	}
	var b strings.Builder
	lastSpace := false
	for _, r := range lower {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			lastSpace = false
			continue
		}
		if !lastSpace {
			b.WriteByte(' ')
			lastSpace = true
		}
	}
	return strings.TrimSpace(b.String())
}

func wordsSet(s string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, w := range strings.Fields(s) {
		out[w] = struct{}{}
	}
	return out
}

func containsRoutePhrase(text, phrase string) bool {
	if text == "" || phrase == "" {
		return false
	}
	textPadded := " " + text + " "
	phrasePadded := " " + phrase + " "
	return strings.Contains(textPadded, phrasePadded)
}

func acronym(tokens []string) string {
	if len(tokens) == 0 {
		return ""
	}
	var b strings.Builder
	for _, token := range tokens {
		if token == "" {
			continue
		}
		b.WriteByte(token[0])
	}
	return b.String()
}

func dedupeStrings(in []string) []string {
	if len(in) == 0 {
		return in
	}
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func redactHeaders(headers map[string]string) map[string]string {
	out := make(map[string]string, len(headers))
	for k, v := range headers {
		lk := strings.ToLower(strings.TrimSpace(k))
		if strings.Contains(lk, "authorization") || strings.Contains(lk, "api-key") || strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "cookie") {
			out[k] = "***REDACTED***"
			continue
		}
		out[k] = v
	}
	return out
}

func redactAny(value any) any {
	switch t := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, v := range t {
			lk := strings.ToLower(strings.TrimSpace(k))
			if strings.Contains(lk, "authorization") || strings.Contains(lk, "api-key") || strings.Contains(lk, "token") || strings.Contains(lk, "secret") || strings.Contains(lk, "password") || strings.Contains(lk, "cookie") {
				out[k] = "***REDACTED***"
				continue
			}
			out[k] = redactAny(v)
		}
		return out
	case []any:
		out := make([]any, 0, len(t))
		for _, item := range t {
			out = append(out, redactAny(item))
		}
		return out
	case string:
		return truncateText(t, 2000)
	default:
		return value
	}
}

func truncateText(s string, max int) string {
	if max <= 0 || len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}
