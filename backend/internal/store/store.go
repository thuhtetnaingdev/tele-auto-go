package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type MessageRecord struct {
	ChatID            string
	TelegramMessageID string
	SenderID          string
	SenderName        string
	Direction         string
	Text              string
	CreatedAt         time.Time
}

type Store struct {
	db *sql.DB
}

type GlobalVariable struct {
	Key       string `json:"key"`
	Type      string `json:"type"`
	Value     string `json:"value"`
	UpdatedAt string `json:"updatedAt,omitempty"`
}

type OrchestrationRun struct {
	ChatID           string
	TriggerMessage   string
	SelectedAgentID  string
	RouterReason     string
	RouterConfidence float64
	ToolName         string
	ToolStatus       string
	Status           string
	ErrorMessage     string
	FinalReply       string
	DurationMS       int64
}

func Open(path string) (*Store, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		return nil, err
	}

	db, err := sql.Open("sqlite", abs)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)

	if _, err := db.Exec(`
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS messages (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id TEXT NOT NULL,
  telegram_message_id TEXT NOT NULL,
  sender_id TEXT,
  sender_name TEXT,
  direction TEXT NOT NULL,
  text TEXT NOT NULL,
  created_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_messages_chat_msg ON messages(chat_id, telegram_message_id);

CREATE TABLE IF NOT EXISTS processed_events (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  telegram_message_id TEXT NOT NULL,
  chat_id TEXT NOT NULL,
  processed_at TEXT NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_processed_events_chat_msg ON processed_events(chat_id, telegram_message_id);

CREATE TABLE IF NOT EXISTS auto_replies (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id TEXT NOT NULL,
  trigger_message_id TEXT NOT NULL,
  context_message_count INTEGER NOT NULL,
  reply_text TEXT,
  model TEXT NOT NULL,
  status TEXT NOT NULL,
  error_message TEXT,
  created_at TEXT NOT NULL,
  sent_at TEXT
);

CREATE TABLE IF NOT EXISTS global_variables (
  key TEXT PRIMARY KEY,
  type TEXT NOT NULL,
  value TEXT NOT NULL,
  updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS orchestration_runs (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  chat_id TEXT NOT NULL,
  trigger_message TEXT NOT NULL,
  selected_agent_id TEXT,
  router_reason TEXT,
  router_confidence REAL,
  tool_name TEXT,
  tool_status TEXT,
  status TEXT NOT NULL,
  error_message TEXT,
  final_reply TEXT,
  duration_ms INTEGER NOT NULL,
  created_at TEXT NOT NULL
);
`); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
}

func (s *Store) UpsertGlobalVariables(ctx context.Context, vars []GlobalVariable) error {
	if len(vars) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT INTO global_variables(key, type, value, updated_at)
VALUES(?,?,?,?)
ON CONFLICT(key) DO UPDATE SET
  type=excluded.type,
  value=excluded.value,
  updated_at=excluded.updated_at
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	now := time.Now().UTC().Format(time.RFC3339Nano)
	for _, v := range vars {
		key := strings.TrimSpace(v.Key)
		typ := strings.TrimSpace(strings.ToLower(v.Type))
		if key == "" {
			return fmt.Errorf("variable key is required")
		}
		if typ != "text" && typ != "secret" {
			return fmt.Errorf("invalid variable type for %s: %s", key, typ)
		}
		if _, err := stmt.ExecContext(ctx, key, typ, v.Value, now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) ListGlobalVariables(ctx context.Context) ([]GlobalVariable, error) {
	rows, err := s.db.QueryContext(ctx, `
SELECT key, type, value, updated_at
FROM global_variables
ORDER BY key ASC
`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := []GlobalVariable{}
	for rows.Next() {
		var v GlobalVariable
		if err := rows.Scan(&v.Key, &v.Type, &v.Value, &v.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GlobalVariablesMap(ctx context.Context) (map[string]string, map[string]string, error) {
	list, err := s.ListGlobalVariables(ctx)
	if err != nil {
		return nil, nil, err
	}
	values := make(map[string]string, len(list))
	types := make(map[string]string, len(list))
	for _, v := range list {
		values[v.Key] = v.Value
		types[v.Key] = v.Type
	}
	return values, types, nil
}

func (s *Store) DeleteGlobalVariable(ctx context.Context, key string) (bool, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return false, fmt.Errorf("variable key is required")
	}
	res, err := s.db.ExecContext(ctx, `DELETE FROM global_variables WHERE key = ? OR UPPER(key) = UPPER(?)`, key, key)
	if err != nil {
		return false, err
	}
	affected, err := res.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func (s *Store) SaveOrchestrationRun(ctx context.Context, run OrchestrationRun) error {
	if strings.TrimSpace(run.Status) == "" {
		run.Status = "unknown"
	}
	_, err := s.db.ExecContext(ctx, `
INSERT INTO orchestration_runs(
  chat_id, trigger_message, selected_agent_id, router_reason, router_confidence,
  tool_name, tool_status, status, error_message, final_reply, duration_ms, created_at
) VALUES(?,?,?,?,?,?,?,?,?,?,?,?)
`, run.ChatID, run.TriggerMessage, run.SelectedAgentID, run.RouterReason, run.RouterConfidence, run.ToolName, run.ToolStatus, run.Status, run.ErrorMessage, run.FinalReply, run.DurationMS, time.Now().UTC().Format(time.RFC3339Nano))
	return err
}

func (s *Store) Close() error {
	return s.db.Close()
}

func (s *Store) ReserveProcessedEvent(ctx context.Context, chatID, telegramMessageID string) (bool, error) {
	_, err := s.db.ExecContext(
		ctx,
		`INSERT INTO processed_events(telegram_message_id, chat_id, processed_at) VALUES(?,?,?)`,
		telegramMessageID,
		chatID,
		time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err == nil {
		return true, nil
	}
	if isUniqueErr(err) {
		return false, nil
	}
	return false, err
}

func (s *Store) SaveMessages(ctx context.Context, rows []MessageRecord) error {
	if len(rows) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
INSERT OR REPLACE INTO messages(chat_id, telegram_message_id, sender_id, sender_name, direction, text, created_at)
VALUES(?,?,?,?,?,?,?)
`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, r := range rows {
		if _, err := stmt.ExecContext(
			ctx,
			r.ChatID,
			r.TelegramMessageID,
			r.SenderID,
			r.SenderName,
			r.Direction,
			r.Text,
			r.CreatedAt.UTC().Format(time.RFC3339Nano),
		); err != nil {
			return err
		}
	}

	return tx.Commit()
}

func (s *Store) CreateAutoReply(
	ctx context.Context,
	chatID string,
	triggerMessageID string,
	contextCount int,
	model string,
) (int64, error) {
	res, err := s.db.ExecContext(
		ctx,
		`INSERT INTO auto_replies(chat_id, trigger_message_id, context_message_count, reply_text, model, status, error_message, created_at, sent_at)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		chatID,
		triggerMessageID,
		contextCount,
		nil,
		model,
		"pending",
		nil,
		time.Now().UTC().Format(time.RFC3339Nano),
		nil,
	)
	if err != nil {
		return 0, err
	}
	return res.LastInsertId()
}

func (s *Store) MarkAutoReplySent(ctx context.Context, id int64, reply string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE auto_replies SET reply_text=?, status='sent', error_message=NULL, sent_at=? WHERE id=?`,
		reply,
		time.Now().UTC().Format(time.RFC3339Nano),
		id,
	)
	return err
}

func (s *Store) MarkAutoReplyFailed(ctx context.Context, id int64, status, errMsg string) error {
	_, err := s.db.ExecContext(
		ctx,
		`UPDATE auto_replies SET reply_text=NULL, status=?, error_message=?, sent_at=NULL WHERE id=?`,
		status,
		errMsg,
		id,
	)
	return err
}

func isUniqueErr(err error) bool {
	// modernc sqlite returns constraint failed text.
	return err != nil &&
		(strings.Contains(err.Error(), "UNIQUE constraint failed") ||
			strings.Contains(err.Error(), "constraint failed"))
}
