package agents

import (
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"

	yaml "gopkg.in/yaml.v2"
)

type Agent struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Intents     []string `json:"intents"`
	Tools       []string `json:"tools"`
	Variables   []string `json:"variables"`
	Visibility  string   `json:"visibility"`
	AllowUsers  []string `json:"allowUsers"`
	Model       string   `json:"model,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	Body        string   `json:"body"`
	Markdown    string   `json:"markdown"`
	UpdatedAt   string   `json:"updatedAt"`
}

type UpsertAgentInput struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Intents     []string `json:"intents"`
	Tools       []string `json:"tools"`
	Variables   []string `json:"variables"`
	Visibility  string   `json:"visibility"`
	AllowUsers  []string `json:"allowUsers"`
	Model       string   `json:"model,omitempty"`
	Temperature float64  `json:"temperature,omitempty"`
	Body        string   `json:"body"`
}

type frontmatter struct {
	ID          string   `yaml:"id"`
	Name        string   `yaml:"name"`
	Description string   `yaml:"description"`
	Intents     []string `yaml:"intents"`
	Tools       []string `yaml:"tools"`
	Variables   []string `yaml:"variables"`
	Visibility  string   `yaml:"visibility"`
	AllowUsers  []string `yaml:"allow_users"`
	Model       string   `yaml:"model,omitempty"`
	Temperature float64  `yaml:"temperature,omitempty"`
}

type Manager struct {
	dir    string
	logger *slog.Logger

	mu    sync.RWMutex
	cache map[string]Agent
}

var agentIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]{2,64}$`)
var varKeyPattern = regexp.MustCompile(`^[A-Z0-9_]{2,64}$`)
var allowUserUsernamePattern = regexp.MustCompile(`^[a-zA-Z0-9_]{3,64}$`)

const (
	VisibilityPublic  = "public"
	VisibilityPrivate = "private"
)

func NewManager(dir string, logger *slog.Logger) (*Manager, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "./agents"
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, err
	}

	m := &Manager{dir: abs, logger: logger, cache: make(map[string]Agent)}
	if err := m.Reload(); err != nil {
		return nil, err
	}
	if len(m.cache) == 0 {
		if err := m.seedDefault(); err != nil {
			return nil, err
		}
		_ = m.Reload()
	}
	return m, nil
}

func (m *Manager) seedDefault() error {
	seed := UpsertAgentInput{
		ID:          "general_assistant",
		Name:        "General Assistant",
		Description: "Default agent for general inquiries and API-assisted answers.",
		Intents:     []string{"general", "faq", "support"},
		Tools:       []string{"api_call"},
		Variables:   []string{},
		Visibility:  VisibilityPublic,
		AllowUsers:  []string{},
		Body:        "You are the default support agent. Understand user intent, call api_call when external data is needed, and return concise human-friendly Burmese or same-language replies.",
	}
	if _, err := m.Upsert(seed); err != nil {
		return err
	}
	m.logger.Info("seeded default agent", "id", seed.ID)
	return nil
}

func (m *Manager) Dir() string { return m.dir }

func (m *Manager) Reload() error {
	entries, err := os.ReadDir(m.dir)
	if err != nil {
		return err
	}
	next := make(map[string]Agent)
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		full := filepath.Join(m.dir, entry.Name())
		agent, err := parseAgentFile(full)
		if err != nil {
			return fmt.Errorf("parse %s: %w", entry.Name(), err)
		}
		next[agent.ID] = agent
	}
	m.mu.Lock()
	m.cache = next
	m.mu.Unlock()
	return nil
}

func (m *Manager) List() []Agent {
	m.refreshBestEffort()
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := make([]Agent, 0, len(m.cache))
	for _, agent := range m.cache {
		out = append(out, agent)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (m *Manager) Get(id string) (Agent, bool) {
	m.refreshBestEffort()
	m.mu.RLock()
	defer m.mu.RUnlock()
	agent, ok := m.cache[strings.TrimSpace(id)]
	return agent, ok
}

func (m *Manager) Upsert(input UpsertAgentInput) (Agent, error) {
	if err := validateInput(input); err != nil {
		return Agent{}, err
	}
	content, err := buildMarkdown(input)
	if err != nil {
		return Agent{}, err
	}
	file := filepath.Join(m.dir, input.ID+".md")
	if err := os.WriteFile(file, []byte(content), 0o644); err != nil {
		return Agent{}, err
	}
	if err := m.Reload(); err != nil {
		return Agent{}, err
	}
	agent, ok := m.Get(input.ID)
	if !ok {
		return Agent{}, errors.New("agent saved but not found")
	}
	return agent, nil
}

func (m *Manager) Delete(id string) error {
	id = strings.TrimSpace(id)
	if !agentIDPattern.MatchString(id) {
		return errors.New("invalid agent id")
	}
	file := filepath.Join(m.dir, id+".md")
	if err := os.Remove(file); err != nil {
		if os.IsNotExist(err) {
			return errors.New("agent not found")
		}
		return err
	}
	return m.Reload()
}

func (m *Manager) refreshBestEffort() {
	if err := m.Reload(); err != nil {
		m.logger.Warn("failed to reload agents", "error", err.Error())
	}
}

func parseAgentFile(path string) (Agent, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Agent{}, err
	}
	text := string(b)
	fm, body, err := splitFrontmatter(text)
	if err != nil {
		return Agent{}, err
	}
	var meta frontmatter
	if err := yaml.Unmarshal([]byte(fm), &meta); err != nil {
		return Agent{}, fmt.Errorf("invalid yaml frontmatter: %w", err)
	}
	input := UpsertAgentInput{
		ID:          strings.TrimSpace(meta.ID),
		Name:        strings.TrimSpace(meta.Name),
		Description: strings.TrimSpace(meta.Description),
		Intents:     meta.Intents,
		Tools:       meta.Tools,
		Variables:   meta.Variables,
		Visibility:  strings.TrimSpace(meta.Visibility),
		AllowUsers:  meta.AllowUsers,
		Model:       strings.TrimSpace(meta.Model),
		Temperature: meta.Temperature,
		Body:        strings.TrimSpace(body),
	}
	if err := validateInput(input); err != nil {
		return Agent{}, err
	}
	normalizedAllowUsers, err := normalizeAllowUsers(input.AllowUsers)
	if err != nil {
		return Agent{}, err
	}
	st, _ := os.Stat(path)
	updatedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if st != nil {
		updatedAt = st.ModTime().UTC().Format(time.RFC3339Nano)
	}
	return Agent{
		ID:          input.ID,
		Name:        input.Name,
		Description: input.Description,
		Intents:     normalizeList(input.Intents),
		Tools:       normalizeList(input.Tools),
		Variables:   normalizeList(input.Variables),
		Visibility:  normalizeVisibility(input.Visibility),
		AllowUsers:  normalizedAllowUsers,
		Model:       input.Model,
		Temperature: input.Temperature,
		Body:        input.Body,
		Markdown:    text,
		UpdatedAt:   updatedAt,
	}, nil
}

func buildMarkdown(input UpsertAgentInput) (string, error) {
	meta := frontmatter{
		ID:          strings.TrimSpace(input.ID),
		Name:        strings.TrimSpace(input.Name),
		Description: strings.TrimSpace(input.Description),
		Intents:     normalizeList(input.Intents),
		Tools:       normalizeList(input.Tools),
		Variables:   normalizeList(input.Variables),
		Visibility:  normalizeVisibility(input.Visibility),
		Model:       strings.TrimSpace(input.Model),
		Temperature: input.Temperature,
	}
	normalizedAllowUsers, err := normalizeAllowUsers(input.AllowUsers)
	if err != nil {
		return "", err
	}
	meta.AllowUsers = normalizedAllowUsers
	yb, err := yaml.Marshal(&meta)
	if err != nil {
		return "", err
	}
	body := strings.TrimSpace(input.Body)
	if body == "" {
		body = "You are a helpful agent."
	}
	return strings.TrimSpace("---\n"+string(yb)+"---\n\n"+body+"\n") + "\n", nil
}

func splitFrontmatter(content string) (string, string, error) {
	trimmed := strings.TrimSpace(content)
	if !strings.HasPrefix(trimmed, "---") {
		return "", "", errors.New("markdown must start with yaml frontmatter")
	}
	lines := strings.Split(trimmed, "\n")
	if len(lines) < 3 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", errors.New("invalid frontmatter start")
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end == -1 {
		return "", "", errors.New("frontmatter end marker not found")
	}
	fm := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")
	return fm, strings.TrimSpace(body), nil
}

func validateInput(input UpsertAgentInput) error {
	id := strings.TrimSpace(input.ID)
	if !agentIDPattern.MatchString(id) {
		return errors.New("id must match [a-zA-Z0-9_-]{2,64}")
	}
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("name is required")
	}
	if strings.TrimSpace(input.Body) == "" {
		return errors.New("body is required")
	}
	tools := normalizeList(input.Tools)
	if len(tools) == 0 {
		return errors.New("at least one tool is required")
	}
	for _, tool := range tools {
		if tool != "api_call" {
			return fmt.Errorf("unsupported tool: %s", tool)
		}
	}
	for _, key := range normalizeList(input.Variables) {
		if !varKeyPattern.MatchString(key) {
			return fmt.Errorf("invalid variable key in variables: %s", key)
		}
	}
	rawVisibility := strings.TrimSpace(input.Visibility)
	if rawVisibility != "" {
		normalizedRaw := strings.ToLower(rawVisibility)
		if normalizedRaw != VisibilityPublic && normalizedRaw != VisibilityPrivate {
			return errors.New("visibility must be public or private")
		}
	}
	visibility := normalizeVisibility(rawVisibility)
	allowUsers, err := normalizeAllowUsers(input.AllowUsers)
	if err != nil {
		return err
	}
	if visibility == VisibilityPrivate && len(allowUsers) == 0 {
		return errors.New("allow users is required for private agents")
	}
	if input.Temperature < 0 || input.Temperature > 2 {
		return errors.New("temperature must be between 0 and 2")
	}
	return nil
}

func normalizeVisibility(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case VisibilityPrivate:
		return VisibilityPrivate
	default:
		return VisibilityPublic
	}
}

func normalizeAllowUsers(in []string) ([]string, error) {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		v, err := normalizeAllowUser(item)
		if err != nil {
			return nil, err
		}
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeAllowUser(raw string) (string, error) {
	v := strings.TrimSpace(raw)
	if v == "" {
		return "", nil
	}
	v = strings.TrimPrefix(v, "@")
	if v == "" {
		return "", fmt.Errorf("invalid allow user: %q", raw)
	}
	if strings.IndexFunc(v, func(r rune) bool { return r == ',' || r == '\n' || r == '\r' || r == '\t' || r == ' ' }) >= 0 {
		return "", fmt.Errorf("invalid allow user: %q", raw)
	}
	if allDigits(v) {
		v = strings.TrimLeft(v, "0")
		if v == "" {
			v = "0"
		}
		return v, nil
	}
	v = strings.ToLower(v)
	if !allowUserUsernamePattern.MatchString(v) {
		return "", fmt.Errorf("invalid allow user: %q", raw)
	}
	return v, nil
}

func allDigits(v string) bool {
	for _, r := range v {
		if r < '0' || r > '9' {
			return false
		}
	}
	return v != ""
}

func normalizeList(in []string) []string {
	out := make([]string, 0, len(in))
	seen := map[string]struct{}{}
	for _, item := range in {
		v := strings.TrimSpace(item)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}
