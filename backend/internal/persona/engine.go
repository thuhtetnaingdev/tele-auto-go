package persona

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"tele-auto-go/internal/store"
)

const (
	defaultGroupRoot    = "./personality/groups"
	defaultUserRoot     = "./personality/users"
	defaultMaxReadBytes = 16000
)

type Options struct {
	GroupRoot        string
	UserRoot         string
	MaxMarkdownBytes int
}

type Engine struct {
	db        *store.Store
	logger    *slog.Logger
	groupRoot string
	userRoot  string
	maxBytes  int
}

type ResolveInput struct {
	ChatID   string
	UserID   string
	Username string
}

type ResolvedPersona struct {
	Source         string `json:"source"`
	GroupID        string `json:"groupId,omitempty"`
	GroupName      string `json:"groupName,omitempty"`
	UserProfileID  string `json:"userProfileId,omitempty"`
	UserLabel      string `json:"userLabel,omitempty"`
	ComposedPrompt string `json:"composedPrompt"`
}

func NewEngine(db *store.Store, logger *slog.Logger, opts Options) *Engine {
	groupRoot := strings.TrimSpace(opts.GroupRoot)
	if groupRoot == "" {
		groupRoot = defaultGroupRoot
	}
	userRoot := strings.TrimSpace(opts.UserRoot)
	if userRoot == "" {
		userRoot = defaultUserRoot
	}
	maxBytes := opts.MaxMarkdownBytes
	if maxBytes <= 0 {
		maxBytes = defaultMaxReadBytes
	}
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(os.Stdout, nil))
	}
	return &Engine{
		db:        db,
		logger:    logger,
		groupRoot: filepath.Clean(groupRoot),
		userRoot:  filepath.Clean(userRoot),
		maxBytes:  maxBytes,
	}
}

func (e *Engine) GroupRoot() string { return e.groupRoot }

func (e *Engine) UserRoot() string { return e.userRoot }

func (e *Engine) MaxMarkdownBytes() int { return e.maxBytes }

func (e *Engine) DefaultGroupMarkdownPath(slug string) string {
	safe := sanitizeFilename(slug)
	if safe == "" {
		safe = "group"
	}
	return filepath.Join(e.groupRoot, safe+".md")
}

func (e *Engine) DefaultUserMarkdownPath(label, id string) string {
	seed := strings.TrimSpace(label)
	if seed == "" {
		seed = strings.TrimSpace(id)
	}
	safe := sanitizeFilename(seed)
	if safe == "" {
		safe = "user"
	}
	return filepath.Join(e.userRoot, safe+".md")
}

func (e *Engine) WriteGroupMarkdown(path, content string) (string, error) {
	return e.writeMarkdown(path, content, e.groupRoot)
}

func (e *Engine) WriteUserMarkdown(path, content string) (string, error) {
	return e.writeMarkdown(path, content, e.userRoot)
}

func (e *Engine) ReadGroupMarkdown(path string) (string, error) {
	return e.readMarkdown(path, e.groupRoot)
}

func (e *Engine) ReadUserMarkdown(path string) (string, error) {
	return e.readMarkdown(path, e.userRoot)
}

func (e *Engine) Resolve(ctx context.Context, in ResolveInput, soulPrompt string) (ResolvedPersona, error) {
	resolved := ResolvedPersona{
		Source:         "soul_only",
		ComposedPrompt: strings.TrimSpace(soulPrompt),
	}

	userID := normalizeUserID(in.UserID)
	username := strings.TrimSpace(in.Username)

	var userProfile store.PersonaUserProfile
	var matchedUser bool
	var err error
	if userID != "" {
		userProfile, matchedUser, err = e.db.FindPersonaUserProfileByUserID(ctx, userID)
		if err != nil {
			return resolved, err
		}
	}
	if !matchedUser {
		userProfile, matchedUser, err = e.db.FindPersonaUserProfileByUsername(ctx, username)
		if err != nil {
			return resolved, err
		}
	}

	if matchedUser {
		userText, readErr := e.ReadUserMarkdown(userProfile.MarkdownPath)
		if readErr != nil {
			e.logger.Warn("persona user profile markdown skipped", "profile_id", userProfile.ID, "error", readErr.Error())
		} else if strings.TrimSpace(userText) != "" {
			resolved.Source = "user_override"
			resolved.UserProfileID = userProfile.ID
			resolved.UserLabel = userProfile.Label
			resolved.ComposedPrompt = composePrompt(soulPrompt, "", "", userText, userProfile.Label)
			_ = e.db.SavePersonaMatchAudit(ctx, store.PersonaMatchAudit{
				ChatID:               strings.TrimSpace(in.ChatID),
				TriggerUserID:        userID,
				TriggerUsername:      username,
				MatchedUserProfileID: userProfile.ID,
				Source:               resolved.Source,
				MatchedAt:            time.Now().UTC().Format(time.RFC3339Nano),
			})
			return resolved, nil
		}
	}

	var group store.PersonaGroup
	var matchedGroup bool
	if userID != "" {
		group, matchedGroup, err = e.db.FindPersonaGroupByUserID(ctx, userID)
		if err != nil {
			return resolved, err
		}
	}
	if !matchedGroup {
		group, matchedGroup, err = e.db.FindPersonaGroupByUsername(ctx, username)
		if err != nil {
			return resolved, err
		}
	}

	if matchedGroup {
		groupText, readErr := e.ReadGroupMarkdown(group.MarkdownPath)
		if readErr != nil {
			e.logger.Warn("persona group markdown skipped", "group_id", group.ID, "error", readErr.Error())
		} else if strings.TrimSpace(groupText) != "" {
			resolved.Source = "group"
			resolved.GroupID = group.ID
			resolved.GroupName = group.Name
			resolved.ComposedPrompt = composePrompt(soulPrompt, groupText, group.Name, "", "")
		}
	}

	if strings.TrimSpace(resolved.ComposedPrompt) == "" {
		resolved.ComposedPrompt = strings.TrimSpace(soulPrompt)
	}
	_ = e.db.SavePersonaMatchAudit(ctx, store.PersonaMatchAudit{
		ChatID:               strings.TrimSpace(in.ChatID),
		TriggerUserID:        userID,
		TriggerUsername:      username,
		MatchedGroupID:       resolved.GroupID,
		MatchedUserProfileID: resolved.UserProfileID,
		Source:               resolved.Source,
		MatchedAt:            time.Now().UTC().Format(time.RFC3339Nano),
	})
	return resolved, nil
}

func composePrompt(soulPrompt, groupPrompt, groupName, userPrompt, userLabel string) string {
	parts := make([]string, 0, 4)
	soulPrompt = strings.TrimSpace(soulPrompt)
	if soulPrompt != "" {
		parts = append(parts, strings.Join([]string{
			"Base personality (SOUL.md):",
			soulPrompt,
		}, "\n"))
	}
	groupPrompt = strings.TrimSpace(groupPrompt)
	if groupPrompt != "" {
		title := "Group sub personality"
		if strings.TrimSpace(groupName) != "" {
			title += " (" + strings.TrimSpace(groupName) + ")"
		}
		parts = append(parts, strings.Join([]string{title + ":", groupPrompt}, "\n"))
	}
	userPrompt = strings.TrimSpace(userPrompt)
	if userPrompt != "" {
		title := "User-specific sub personality"
		if strings.TrimSpace(userLabel) != "" {
			title += " (" + strings.TrimSpace(userLabel) + ")"
		}
		parts = append(parts, strings.Join([]string{title + ":", userPrompt}, "\n"))
	}
	if len(parts) == 0 {
		return ""
	}
	parts = append(parts, "Precedence: user-specific > group > base SOUL.")
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

func (e *Engine) readMarkdown(path string, root string) (string, error) {
	fullPath, err := securePath(path, root)
	if err != nil {
		return "", err
	}
	b, err := os.ReadFile(fullPath)
	if err != nil {
		return "", err
	}
	if len(b) > e.maxBytes {
		b = b[:e.maxBytes]
	}
	return strings.TrimSpace(string(b)), nil
}

func (e *Engine) writeMarkdown(path, content, root string) (string, error) {
	fullPath, err := securePath(path, root)
	if err != nil {
		return "", err
	}
	if len([]byte(content)) > e.maxBytes {
		return "", fmt.Errorf("markdown content exceeds %d bytes", e.maxBytes)
	}
	if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
		return "", err
	}
	return fullPath, nil
}

func securePath(path, root string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("markdown path is required")
	}
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	var candidateAbs string
	if filepath.IsAbs(path) {
		candidateAbs = filepath.Clean(path)
	} else {
		// Prefer repo-relative paths that already resolve inside the persona root.
		// This avoids duplicating root segments when DB stores paths like personality/groups/x.md.
		fromCWD, err := filepath.Abs(path)
		if err == nil && (fromCWD == rootAbs || strings.HasPrefix(fromCWD, rootAbs+string(os.PathSeparator))) {
			candidateAbs = filepath.Clean(fromCWD)
		} else {
			candidateAbs = filepath.Clean(filepath.Join(rootAbs, path))
		}
	}
	if candidateAbs != rootAbs && !strings.HasPrefix(candidateAbs, rootAbs+string(os.PathSeparator)) {
		return "", fmt.Errorf("path escapes allowed root")
	}
	return candidateAbs, nil
}

func normalizeUserID(raw string) string {
	v := strings.TrimSpace(raw)
	if v == "" {
		return ""
	}
	if _, err := strconv.ParseInt(v, 10, 64); err != nil {
		return ""
	}
	return v
}

var filenamePattern = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeFilename(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	v = strings.ReplaceAll(v, "@", "")
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.ReplaceAll(v, " ", "-")
	v = filenamePattern.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	if len(v) > 120 {
		v = strings.Trim(v[:120], "-")
	}
	return v
}
