package soul

import (
	"log/slog"
	"os"
	"path/filepath"
	"strings"
)

func Load(path string, logger *slog.Logger) string {
	resolved, err := filepath.Abs(path)
	if err != nil {
		resolved = path
	}

	b, err := os.ReadFile(resolved)
	if err != nil {
		logger.Warn("SOUL prompt not loaded, using defaults", "path", resolved, "error", err.Error())
		return ""
	}

	text := strings.TrimSpace(string(b))
	if text == "" {
		logger.Warn("SOUL prompt file is empty, using defaults", "path", resolved)
		return ""
	}
	logger.Info("Loaded SOUL prompt", "path", resolved)
	return text
}
