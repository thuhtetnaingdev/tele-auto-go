package health

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
)

func NewServer(port int) *http.Server {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Any("/health", func(c *gin.Context) {
		w := c.Writer
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status":    "ok",
			"service":   "telegram-native-ai-auto-reply-go",
			"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		})
	})
	_ = router.SetTrustedProxies(nil)

	return &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}
}

func Shutdown(ctx context.Context, srv *http.Server) error {
	if srv == nil {
		return nil
	}
	return srv.Shutdown(ctx)
}
