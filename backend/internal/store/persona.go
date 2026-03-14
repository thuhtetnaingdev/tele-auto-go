package store

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type PersonaGroup struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Slug         string `json:"slug"`
	Description  string `json:"description"`
	MarkdownPath string `json:"path"`
	MemberCount  int    `json:"memberCount"`
	CreatedAt    string `json:"createdAt,omitempty"`
	UpdatedAt    string `json:"updatedAt,omitempty"`
}

type PersonaGroupMember struct {
	ID                 int64  `json:"id"`
	GroupID            string `json:"groupId"`
	UserID             string `json:"userId,omitempty"`
	Username           string `json:"username,omitempty"`
	NormalizedUsername string `json:"normalizedUsername,omitempty"`
	CreatedAt          string `json:"createdAt,omitempty"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

type PersonaUserProfile struct {
	ID                 string `json:"id"`
	Label              string `json:"label"`
	UserID             string `json:"userId,omitempty"`
	Username           string `json:"username,omitempty"`
	NormalizedUsername string `json:"normalizedUsername,omitempty"`
	MarkdownPath       string `json:"path"`
	Enabled            bool   `json:"enabled"`
	CreatedAt          string `json:"createdAt,omitempty"`
	UpdatedAt          string `json:"updatedAt,omitempty"`
}

type PersonaMatchAudit struct {
	ID                   int64  `json:"id"`
	ChatID               string `json:"chatId"`
	TriggerUserID        string `json:"triggerUserId,omitempty"`
	TriggerUsername      string `json:"triggerUsername,omitempty"`
	MatchedGroupID       string `json:"matchedGroupId,omitempty"`
	MatchedUserProfileID string `json:"matchedUserProfileId,omitempty"`
	Source               string `json:"source"`
	MatchedAt            string `json:"matchedAt"`
}

type PersonaGroupInput struct {
	ID           string
	Name         string
	Slug         string
	Description  string
	MarkdownPath string
}

type PersonaGroupMemberInput struct {
	GroupID  string
	UserID   string
	Username string
}

type PersonaUserProfileInput struct {
	ID           string
	Label        string
	UserID       string
	Username     string
	MarkdownPath string
	Enabled      bool
}

func ensurePersonaSchema(db *sql.DB) error {
	_, err := db.Exec(`
CREATE TABLE IF NOT EXISTS persona_groups (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  slug TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  markdown_path TEXT NOT NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_persona_groups_slug ON persona_groups(slug);

CREATE TABLE IF NOT EXISTS persona_group_members (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  group_id TEXT NOT NULL,
  user_id TEXT,
  username TEXT,
  normalized_username TEXT,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  FOREIGN KEY(group_id) REFERENCES persona_groups(id) ON DELETE CASCADE,
  CHECK (TRIM(COALESCE(user_id, '')) <> '' OR TRIM(COALESCE(username, '')) <> '')
);
CREATE INDEX IF NOT EXISTS idx_persona_group_members_group ON persona_group_members(group_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_persona_group_members_user_unique
  ON persona_group_members(user_id)
  WHERE user_id IS NOT NULL AND TRIM(user_id) <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_persona_group_members_username_unique
  ON persona_group_members(normalized_username)
  WHERE normalized_username IS NOT NULL AND TRIM(normalized_username) <> '';

CREATE TABLE IF NOT EXISTS persona_user_profiles (
  id TEXT PRIMARY KEY,
  label TEXT NOT NULL,
  user_id TEXT,
  username TEXT,
  normalized_username TEXT,
  markdown_path TEXT NOT NULL,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL,
  CHECK (TRIM(COALESCE(user_id, '')) <> '' OR TRIM(COALESCE(username, '')) <> '')
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_persona_user_profiles_user_unique
  ON persona_user_profiles(user_id)
  WHERE user_id IS NOT NULL AND TRIM(user_id) <> '';
CREATE UNIQUE INDEX IF NOT EXISTS idx_persona_user_profiles_username_unique
  ON persona_user_profiles(normalized_username)
  WHERE normalized_username IS NOT NULL AND TRIM(normalized_username) <> '';

CREATE TABLE IF NOT EXISTS persona_match_audit (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id TEXT NOT NULL,
  trigger_user_id TEXT,
  trigger_username TEXT,
  matched_group_id TEXT,
  matched_user_profile_id TEXT,
  source TEXT NOT NULL,
  matched_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_persona_match_audit_chat_id ON persona_match_audit(chat_id);
CREATE INDEX IF NOT EXISTS idx_persona_match_audit_matched_at ON persona_match_audit(matched_at DESC);
`)
	return err
}

func NormalizePersonaUsername(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	v = strings.TrimPrefix(v, "@")
	return strings.TrimSpace(v)
}

func (s *Store) ListPersonaGroups(ctx context.Context) ([]PersonaGroup, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  g.id,
  g.name,
  g.slug,
  g.description,
  g.markdown_path,
  COUNT(m.id) AS member_count,
  g.created_at,
  g.updated_at
FROM persona_groups g
LEFT JOIN persona_group_members m ON m.group_id = g.id
GROUP BY g.id
ORDER BY g.updated_at DESC, g.id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PersonaGroup, 0)
	for rows.Next() {
		var item PersonaGroup
		if err := rows.Scan(
			&item.ID,
			&item.Name,
			&item.Slug,
			&item.Description,
			&item.MarkdownPath,
			&item.MemberCount,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetPersonaGroup(ctx context.Context, id string) (PersonaGroup, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PersonaGroup{}, false, fmt.Errorf("group id is required")
	}
	var item PersonaGroup
	err := s.db.QueryRowContext(ctx, `
SELECT
  g.id,
  g.name,
  g.slug,
  g.description,
  g.markdown_path,
  COUNT(m.id) AS member_count,
  g.created_at,
  g.updated_at
FROM persona_groups g
LEFT JOIN persona_group_members m ON m.group_id = g.id
WHERE g.id = ?
GROUP BY g.id
`, id).Scan(
		&item.ID,
		&item.Name,
		&item.Slug,
		&item.Description,
		&item.MarkdownPath,
		&item.MemberCount,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return PersonaGroup{}, false, nil
	}
	if err != nil {
		return PersonaGroup{}, false, err
	}
	return item, true, nil
}

func (s *Store) CreatePersonaGroup(ctx context.Context, in PersonaGroupInput) (PersonaGroup, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := strings.TrimSpace(in.ID)
	name := strings.TrimSpace(in.Name)
	slug := sanitizeSlug(in.Slug)
	desc := strings.TrimSpace(in.Description)
	mdPath := strings.TrimSpace(in.MarkdownPath)

	if name == "" {
		return PersonaGroup{}, fmt.Errorf("name is required")
	}
	if id == "" {
		id = makeEntityID("grp", name)
	}
	if slug == "" {
		slug = sanitizeSlug(name)
	}
	if slug == "" {
		return PersonaGroup{}, fmt.Errorf("slug is required")
	}
	if mdPath == "" {
		return PersonaGroup{}, fmt.Errorf("markdown path is required")
	}

	_, err := s.db.ExecContext(ctx, `
INSERT INTO persona_groups(id, name, slug, description, markdown_path, created_at, updated_at)
VALUES(?,?,?,?,?,?,?)
`, id, name, slug, desc, mdPath, now, now)
	if err != nil {
		if isUniqueErr(err) {
			return PersonaGroup{}, fmt.Errorf("group id or slug already exists")
		}
		return PersonaGroup{}, err
	}
	item, _, err := s.GetPersonaGroup(ctx, id)
	if err != nil {
		return PersonaGroup{}, err
	}
	return item, nil
}

func (s *Store) UpdatePersonaGroup(ctx context.Context, id string, in PersonaGroupInput) (PersonaGroup, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PersonaGroup{}, fmt.Errorf("group id is required")
	}
	name := strings.TrimSpace(in.Name)
	slug := sanitizeSlug(in.Slug)
	desc := strings.TrimSpace(in.Description)
	mdPath := strings.TrimSpace(in.MarkdownPath)
	if name == "" {
		return PersonaGroup{}, fmt.Errorf("name is required")
	}
	if slug == "" {
		slug = sanitizeSlug(name)
	}
	if slug == "" {
		return PersonaGroup{}, fmt.Errorf("slug is required")
	}
	if mdPath == "" {
		return PersonaGroup{}, fmt.Errorf("markdown path is required")
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE persona_groups
SET name = ?, slug = ?, description = ?, markdown_path = ?, updated_at = ?
WHERE id = ?
`, name, slug, desc, mdPath, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		if isUniqueErr(err) {
			return PersonaGroup{}, fmt.Errorf("group slug already exists")
		}
		return PersonaGroup{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return PersonaGroup{}, err
	}
	if affected == 0 {
		return PersonaGroup{}, fmt.Errorf("group not found")
	}
	item, _, err := s.GetPersonaGroup(ctx, id)
	if err != nil {
		return PersonaGroup{}, err
	}
	return item, nil
}

func (s *Store) DeletePersonaGroup(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("group id is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM persona_groups WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) ListPersonaGroupMembers(ctx context.Context, groupID string) ([]PersonaGroupMember, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return nil, fmt.Errorf("group id is required")
	}
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id,
  group_id,
  COALESCE(user_id, '') AS user_id,
  COALESCE(username, '') AS username,
  COALESCE(normalized_username, '') AS normalized_username,
  created_at,
  updated_at
FROM persona_group_members
WHERE group_id = ?
ORDER BY id ASC
`, groupID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]PersonaGroupMember, 0)
	for rows.Next() {
		var item PersonaGroupMember
		if err := rows.Scan(
			&item.ID,
			&item.GroupID,
			&item.UserID,
			&item.Username,
			&item.NormalizedUsername,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) CreatePersonaGroupMember(ctx context.Context, in PersonaGroupMemberInput) (PersonaGroupMember, error) {
	groupID := strings.TrimSpace(in.GroupID)
	userID := normalizeUserID(in.UserID)
	username := strings.TrimSpace(in.Username)
	normalized := NormalizePersonaUsername(username)
	if groupID == "" {
		return PersonaGroupMember{}, fmt.Errorf("group id is required")
	}
	if userID == "" && normalized == "" {
		return PersonaGroupMember{}, fmt.Errorf("userId or username is required")
	}
	now := time.Now().UTC().Format(time.RFC3339Nano)
	res, err := s.db.ExecContext(ctx, `
INSERT INTO persona_group_members(group_id, user_id, username, normalized_username, created_at, updated_at)
VALUES(?,?,?,?,?,?)
`, groupID, nullableString(userID), nullableString(username), nullableString(normalized), now, now)
	if err != nil {
		if isUniqueErr(err) {
			return PersonaGroupMember{}, fmt.Errorf("member identity already assigned to another group")
		}
		return PersonaGroupMember{}, err
	}
	id, err := res.LastInsertId()
	if err != nil {
		return PersonaGroupMember{}, err
	}
	row := s.db.QueryRowContext(ctx, `
SELECT
  id,
  group_id,
  COALESCE(user_id, '') AS user_id,
  COALESCE(username, '') AS username,
  COALESCE(normalized_username, '') AS normalized_username,
  created_at,
  updated_at
FROM persona_group_members
WHERE id = ?
`, id)
	var item PersonaGroupMember
	if err := row.Scan(
		&item.ID,
		&item.GroupID,
		&item.UserID,
		&item.Username,
		&item.NormalizedUsername,
		&item.CreatedAt,
		&item.UpdatedAt,
	); err != nil {
		return PersonaGroupMember{}, err
	}
	return item, nil
}

func (s *Store) DeletePersonaGroupMember(ctx context.Context, groupID string, memberID int64) (bool, error) {
	groupID = strings.TrimSpace(groupID)
	if groupID == "" {
		return false, fmt.Errorf("group id is required")
	}
	if memberID <= 0 {
		return false, fmt.Errorf("member id is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM persona_group_members WHERE id = ? AND group_id = ?`, memberID, groupID)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) ListPersonaUserProfiles(ctx context.Context) ([]PersonaUserProfile, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT
  id,
  label,
  COALESCE(user_id, '') AS user_id,
  COALESCE(username, '') AS username,
  COALESCE(normalized_username, '') AS normalized_username,
  markdown_path,
  enabled,
  created_at,
  updated_at
FROM persona_user_profiles
ORDER BY updated_at DESC, id ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]PersonaUserProfile, 0)
	for rows.Next() {
		var item PersonaUserProfile
		var enabled int
		if err := rows.Scan(
			&item.ID,
			&item.Label,
			&item.UserID,
			&item.Username,
			&item.NormalizedUsername,
			&item.MarkdownPath,
			&enabled,
			&item.CreatedAt,
			&item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		item.Enabled = enabled == 1
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetPersonaUserProfile(ctx context.Context, id string) (PersonaUserProfile, bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PersonaUserProfile{}, false, fmt.Errorf("profile id is required")
	}
	var item PersonaUserProfile
	var enabled int
	err := s.db.QueryRowContext(ctx, `
SELECT
  id,
  label,
  COALESCE(user_id, '') AS user_id,
  COALESCE(username, '') AS username,
  COALESCE(normalized_username, '') AS normalized_username,
  markdown_path,
  enabled,
  created_at,
  updated_at
FROM persona_user_profiles
WHERE id = ?
`, id).Scan(
		&item.ID,
		&item.Label,
		&item.UserID,
		&item.Username,
		&item.NormalizedUsername,
		&item.MarkdownPath,
		&enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return PersonaUserProfile{}, false, nil
	}
	if err != nil {
		return PersonaUserProfile{}, false, err
	}
	item.Enabled = enabled == 1
	return item, true, nil
}

func (s *Store) CreatePersonaUserProfile(ctx context.Context, in PersonaUserProfileInput) (PersonaUserProfile, error) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	id := strings.TrimSpace(in.ID)
	label := strings.TrimSpace(in.Label)
	userID := normalizeUserID(in.UserID)
	username := strings.TrimSpace(in.Username)
	normalized := NormalizePersonaUsername(username)
	mdPath := strings.TrimSpace(in.MarkdownPath)
	if id == "" {
		id = makeEntityID("usr", label)
	}
	if label == "" {
		return PersonaUserProfile{}, fmt.Errorf("label is required")
	}
	if userID == "" && normalized == "" {
		return PersonaUserProfile{}, fmt.Errorf("userId or username is required")
	}
	if mdPath == "" {
		return PersonaUserProfile{}, fmt.Errorf("markdown path is required")
	}
	enabled := 0
	if in.Enabled {
		enabled = 1
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO persona_user_profiles(id, label, user_id, username, normalized_username, markdown_path, enabled, created_at, updated_at)
VALUES(?,?,?,?,?,?,?,?,?)
`, id, label, nullableString(userID), nullableString(username), nullableString(normalized), mdPath, enabled, now, now)
	if err != nil {
		if isUniqueErr(err) {
			return PersonaUserProfile{}, fmt.Errorf("profile identity already exists")
		}
		return PersonaUserProfile{}, err
	}
	item, _, err := s.GetPersonaUserProfile(ctx, id)
	if err != nil {
		return PersonaUserProfile{}, err
	}
	return item, nil
}

func (s *Store) UpdatePersonaUserProfile(ctx context.Context, id string, in PersonaUserProfileInput) (PersonaUserProfile, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return PersonaUserProfile{}, fmt.Errorf("profile id is required")
	}
	label := strings.TrimSpace(in.Label)
	userID := normalizeUserID(in.UserID)
	username := strings.TrimSpace(in.Username)
	normalized := NormalizePersonaUsername(username)
	mdPath := strings.TrimSpace(in.MarkdownPath)
	if label == "" {
		return PersonaUserProfile{}, fmt.Errorf("label is required")
	}
	if userID == "" && normalized == "" {
		return PersonaUserProfile{}, fmt.Errorf("userId or username is required")
	}
	if mdPath == "" {
		return PersonaUserProfile{}, fmt.Errorf("markdown path is required")
	}
	enabled := 0
	if in.Enabled {
		enabled = 1
	}

	res, err := s.db.ExecContext(ctx, `
UPDATE persona_user_profiles
SET label = ?, user_id = ?, username = ?, normalized_username = ?, markdown_path = ?, enabled = ?, updated_at = ?
WHERE id = ?
`, label, nullableString(userID), nullableString(username), nullableString(normalized), mdPath, enabled, time.Now().UTC().Format(time.RFC3339Nano), id)
	if err != nil {
		if isUniqueErr(err) {
			return PersonaUserProfile{}, fmt.Errorf("profile identity already exists")
		}
		return PersonaUserProfile{}, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return PersonaUserProfile{}, err
	}
	if affected == 0 {
		return PersonaUserProfile{}, fmt.Errorf("profile not found")
	}
	item, _, err := s.GetPersonaUserProfile(ctx, id)
	if err != nil {
		return PersonaUserProfile{}, err
	}
	return item, nil
}

func (s *Store) DeletePersonaUserProfile(ctx context.Context, id string) (bool, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return false, fmt.Errorf("profile id is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM persona_user_profiles WHERE id = ?`, id)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) FindPersonaUserProfileByUserID(ctx context.Context, userID string) (PersonaUserProfile, bool, error) {
	userID = normalizeUserID(userID)
	if userID == "" {
		return PersonaUserProfile{}, false, nil
	}
	return s.findPersonaUserProfileByWhere(ctx, `enabled = 1 AND user_id = ?`, userID)
}

func (s *Store) FindPersonaUserProfileByUsername(ctx context.Context, username string) (PersonaUserProfile, bool, error) {
	normalized := NormalizePersonaUsername(username)
	if normalized == "" {
		return PersonaUserProfile{}, false, nil
	}
	return s.findPersonaUserProfileByWhere(ctx, `enabled = 1 AND normalized_username = ?`, normalized)
}

func (s *Store) findPersonaUserProfileByWhere(ctx context.Context, where string, arg any) (PersonaUserProfile, bool, error) {
	query := `
SELECT
  id,
  label,
  COALESCE(user_id, '') AS user_id,
  COALESCE(username, '') AS username,
  COALESCE(normalized_username, '') AS normalized_username,
  markdown_path,
  enabled,
  created_at,
  updated_at
FROM persona_user_profiles
WHERE ` + where + `
ORDER BY updated_at DESC
LIMIT 1
`
	var item PersonaUserProfile
	var enabled int
	err := s.db.QueryRowContext(ctx, query, arg).Scan(
		&item.ID,
		&item.Label,
		&item.UserID,
		&item.Username,
		&item.NormalizedUsername,
		&item.MarkdownPath,
		&enabled,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return PersonaUserProfile{}, false, nil
	}
	if err != nil {
		return PersonaUserProfile{}, false, err
	}
	item.Enabled = enabled == 1
	return item, true, nil
}

func (s *Store) FindPersonaGroupByUserID(ctx context.Context, userID string) (PersonaGroup, bool, error) {
	userID = normalizeUserID(userID)
	if userID == "" {
		return PersonaGroup{}, false, nil
	}
	return s.findPersonaGroupByWhere(ctx, `m.user_id = ?`, userID)
}

func (s *Store) FindPersonaGroupByUsername(ctx context.Context, username string) (PersonaGroup, bool, error) {
	normalized := NormalizePersonaUsername(username)
	if normalized == "" {
		return PersonaGroup{}, false, nil
	}
	return s.findPersonaGroupByWhere(ctx, `m.normalized_username = ?`, normalized)
}

func (s *Store) findPersonaGroupByWhere(ctx context.Context, where string, arg any) (PersonaGroup, bool, error) {
	query := `
SELECT
  g.id,
  g.name,
  g.slug,
  g.description,
  g.markdown_path,
  (
    SELECT COUNT(1)
    FROM persona_group_members pm
    WHERE pm.group_id = g.id
  ) AS member_count,
  g.created_at,
  g.updated_at
FROM persona_group_members m
JOIN persona_groups g ON g.id = m.group_id
WHERE ` + where + `
LIMIT 1
`
	var item PersonaGroup
	err := s.db.QueryRowContext(ctx, query, arg).Scan(
		&item.ID,
		&item.Name,
		&item.Slug,
		&item.Description,
		&item.MarkdownPath,
		&item.MemberCount,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return PersonaGroup{}, false, nil
	}
	if err != nil {
		return PersonaGroup{}, false, err
	}
	return item, true, nil
}

func (s *Store) SavePersonaMatchAudit(ctx context.Context, item PersonaMatchAudit) error {
	if strings.TrimSpace(item.ChatID) == "" {
		return fmt.Errorf("chat id is required")
	}
	source := strings.TrimSpace(item.Source)
	if source == "" {
		source = "soul_only"
	}
	when := strings.TrimSpace(item.MatchedAt)
	if when == "" {
		when = time.Now().UTC().Format(time.RFC3339Nano)
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO persona_match_audit(
  chat_id,
  trigger_user_id,
  trigger_username,
  matched_group_id,
  matched_user_profile_id,
  source,
  matched_at
) VALUES(?,?,?,?,?,?,?)
`, strings.TrimSpace(item.ChatID), nullableString(normalizeUserID(item.TriggerUserID)), nullableString(strings.TrimSpace(item.TriggerUsername)), nullableString(strings.TrimSpace(item.MatchedGroupID)), nullableString(strings.TrimSpace(item.MatchedUserProfileID)), source, when)
	return err
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

var slugRegex = regexp.MustCompile(`[^a-z0-9-]+`)

func sanitizeSlug(raw string) string {
	v := strings.TrimSpace(strings.ToLower(raw))
	v = strings.ReplaceAll(v, "_", "-")
	v = strings.ReplaceAll(v, " ", "-")
	v = slugRegex.ReplaceAllString(v, "-")
	v = strings.Trim(v, "-")
	for strings.Contains(v, "--") {
		v = strings.ReplaceAll(v, "--", "-")
	}
	return v
}

func makeEntityID(prefix, value string) string {
	slug := sanitizeSlug(value)
	if slug == "" {
		slug = "item"
	}
	ts := time.Now().UTC().UnixNano() / int64(time.Millisecond)
	return filepath.ToSlash(strings.TrimSpace(prefix) + "_" + slug + "_" + strconv.FormatInt(ts, 10))
}
