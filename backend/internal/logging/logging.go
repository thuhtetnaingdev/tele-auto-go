package logging

import (
	"context"
	"log/slog"
	"os"
	"strings"

	"tele-auto-go/internal/logstream"
)

func New(level string) *slog.Logger {
	return NewWithHub(level, nil)
}

func NewWithHub(level string, hub *logstream.Hub) *slog.Logger {
	lvl := slog.LevelInfo
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	}

	base := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: lvl,
	})
	handler := slog.Handler(base)
	if hub != nil {
		handler = &hubHandler{
			base: base,
			hub:  hub,
		}
	}
	return slog.New(handler)
}

type hubHandler struct {
	base slog.Handler
	hub  *logstream.Hub
}

func (h *hubHandler) Enabled(ctx context.Context, level slog.Level) bool {
	return h.base.Enabled(ctx, level)
}

func (h *hubHandler) Handle(ctx context.Context, record slog.Record) error {
	if err := h.base.Handle(ctx, record); err != nil {
		return err
	}
	attrs := make(map[string]any)
	record.Attrs(func(attr slog.Attr) bool {
		attrs[attr.Key] = attr.Value.Any()
		return true
	})

	h.hub.Publish(logstream.Entry{
		Time:    record.Time.UTC().Format("2006-01-02T15:04:05.000000000Z07:00"),
		Level:   strings.ToLower(record.Level.String()),
		Message: record.Message,
		Attrs:   attrs,
	})
	return nil
}

func (h *hubHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &hubHandler{
		base: h.base.WithAttrs(attrs),
		hub:  h.hub,
	}
}

func (h *hubHandler) WithGroup(name string) slog.Handler {
	return &hubHandler{
		base: h.base.WithGroup(name),
		hub:  h.hub,
	}
}
