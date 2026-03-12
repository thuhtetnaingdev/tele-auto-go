package store

import (
	"context"
	"database/sql"
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
`); err != nil {
		_ = db.Close()
		return nil, err
	}

	return &Store{db: db}, nil
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
