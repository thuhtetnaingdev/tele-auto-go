package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gotd/td/session"
	"github.com/gotd/td/telegram"
	"github.com/gotd/td/tg"
	"github.com/joho/godotenv"

	"tele-auto-go/internal/adminauth"
	"tele-auto-go/internal/agents"
	"tele-auto-go/internal/behavior"
	"tele-auto-go/internal/config"
	"tele-auto-go/internal/control"
	"tele-auto-go/internal/conversationstream"
	"tele-auto-go/internal/logging"
	"tele-auto-go/internal/logstream"
	"tele-auto-go/internal/persona"
	"tele-auto-go/internal/store"
	tgsvc "tele-auto-go/internal/telegram"
	"tele-auto-go/internal/tgauth"
)

type apiServer struct {
	logger       *slog.Logger
	manager      *control.Manager
	logs         *logstream.Hub
	convEvents   *conversationstream.Hub
	frontendBase string
	webDir       string
	adminAuth    *adminauth.Manager
	agentMgr     *agents.Manager
	db           *store.Store
	persona      *persona.Engine
	adminMu      sync.RWMutex

	otpMu      sync.Mutex
	otpByPhone map[string]otpState
}

type otpState struct {
	phoneCodeHash string
	createdAt     time.Time
	timeoutSec    int
}

func main() {
	_ = godotenv.Load()

	level := strings.TrimSpace(os.Getenv("LOG_LEVEL"))
	if level == "" {
		level = "info"
	}
	logs := logstream.NewHub(500)
	convs := conversationstream.NewHub(500)
	logger := logging.NewWithHub(level, logs)
	manager := control.NewManager(logger, func(ev tgsvc.Event) {
		convs.Publish(conversationstream.Event{
			Type:      ev.Type,
			ChatID:    ev.ChatID,
			MessageID: ev.TelegramMessageID,
			Direction: ev.Direction,
			Text:      ev.Text,
			Mode:      ev.Mode,
			Reason:    ev.Reason,
			CreatedAt: ev.CreatedAt,
		})
	})
	admin, err := adminauth.NewManager(adminauth.Config{
		Username:      strings.TrimSpace(os.Getenv("ADMIN_USERNAME")),
		PasswordHash:  strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_HASH")),
		PasswordSalt:  strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_SALT")),
		SessionSecret: strings.TrimSpace(os.Getenv("ADMIN_SESSION_SECRET")),
		SessionTTL:    time.Duration(readIntEnv("ADMIN_SESSION_TTL_HOURS", 24*7)) * time.Hour,
		CookieSecure:  readBoolEnv("COOKIE_SECURE", false),
	})
	if err != nil {
		logger.Error("invalid admin auth config", "error", err.Error())
		os.Exit(1)
	}

	server := &apiServer{
		logger:       logger,
		manager:      manager,
		logs:         logs,
		convEvents:   convs,
		frontendBase: strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")),
		webDir:       readWebDir(),
		adminAuth:    admin,
		otpByPhone:   make(map[string]otpState),
	}
	sqlitePath := strings.TrimSpace(os.Getenv("SQLITE_PATH"))
	if sqlitePath == "" {
		sqlitePath = "./data/app.db"
	}
	st, err := store.Open(sqlitePath)
	if err != nil {
		logger.Error("failed to open sqlite store", "error", err.Error())
		os.Exit(1)
	}
	defer st.Close()
	server.db = st
	server.persona = persona.NewEngine(st, logger, persona.Options{
		GroupRoot:        readPersonaGroupsDir(),
		UserRoot:         readPersonaUsersDir(),
		MaxMarkdownBytes: readIntEnv("PERSONA_MAX_MARKDOWN_BYTES", 16000),
	})

	agentsDir := strings.TrimSpace(os.Getenv("AGENTS_DIR"))
	if agentsDir == "" {
		agentsDir = "./agents"
	}
	agentMgr, err := agents.NewManager(agentsDir, logger)
	if err != nil {
		logger.Error("failed to initialize agent manager", "error", err.Error())
		os.Exit(1)
	}
	server.agentMgr = agentMgr

	if info, err := os.Stat(server.webDir); err == nil && info.IsDir() {
		logger.Info("frontend web dir detected", "web_dir", server.webDir)
	} else {
		logger.Warn("frontend web dir not found; API-only mode", "web_dir", server.webDir)
	}
	if !server.getAdminAuth().Enabled() {
		logger.Error("admin auth is required but not configured", "hint", "set ADMIN_USERNAME, ADMIN_PASSWORD_HASH, ADMIN_PASSWORD_SALT, ADMIN_SESSION_SECRET")
		os.Exit(1)
	}
	logger.Info("admin auth enabled", "username", server.getAdminAuth().Username())

	if err := manager.Start(); err != nil {
		logger.Warn("telegram worker did not auto-start", "error", err.Error())
	}

	port := readServerPort()
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.HandleMethodNotAllowed = true
	router.Use(gin.Recovery())
	router.Use(corsMiddleware(server.frontendBase))
	_ = router.SetTrustedProxies(nil)

	router.GET("/health", gin.WrapF(server.handleHealth))
	router.GET("/api/admin/me", gin.WrapF(server.handleAdminMe))
	router.POST("/api/admin/login", gin.WrapF(server.handleAdminLogin))
	router.POST("/api/admin/logout", gin.WrapF(server.handleAdminLogout))

	protected := router.Group("/api")
	protected.Use(server.requireAdminGin())
	{
		protected.PUT("/admin/credentials", gin.WrapF(server.handleAdminCredentials))
		protected.GET("/auth/status", gin.WrapF(server.handleAuthStatus))
		protected.POST("/auth/login", gin.WrapF(server.handleLogin))
		protected.POST("/auth/logout", gin.WrapF(server.handleLogout))
		protected.GET("/service/status", gin.WrapF(server.handleServiceStatus))
		protected.POST("/service/start", gin.WrapF(server.handleServiceStart))
		protected.POST("/service/stop", gin.WrapF(server.handleServiceStop))
		protected.POST("/service/restart", gin.WrapF(server.handleServiceRestart))
		protected.GET("/settings", gin.WrapF(server.handleSettings))
		protected.PUT("/settings", gin.WrapF(server.handleSettings))
		protected.GET("/variables", gin.WrapF(server.handleVariables))
		protected.PUT("/variables", gin.WrapF(server.handleVariables))
		protected.DELETE("/variables/:key", gin.WrapF(server.handleVariableByKey))
		protected.GET("/agents", gin.WrapF(server.handleAgents))
		protected.POST("/agents", gin.WrapF(server.handleAgents))
		protected.GET("/agents/:id", gin.WrapF(server.handleAgentByID))
		protected.PUT("/agents/:id", gin.WrapF(server.handleAgentByID))
		protected.DELETE("/agents/:id", gin.WrapF(server.handleAgentByID))
		protected.GET("/conversations/stream", gin.WrapF(server.handleConversationStream))
		protected.GET("/conversations", gin.WrapF(server.handleConversations))
		protected.GET("/conversations/:chatID/:action", gin.WrapF(server.handleConversationByID))
		protected.POST("/conversations/:chatID/:action", gin.WrapF(server.handleConversationByID))
		protected.PUT("/conversations/:chatID/:action", gin.WrapF(server.handleConversationByID))
		protected.GET("/behavior", gin.WrapF(server.handleBehavior))
		protected.PUT("/behavior", gin.WrapF(server.handleBehavior))
		protected.GET("/behavior/runtime", gin.WrapF(server.handleBehaviorRuntime))
		protected.GET("/persona/groups", gin.WrapF(server.handlePersonaGroups))
		protected.POST("/persona/groups", gin.WrapF(server.handlePersonaGroups))
		protected.GET("/persona/groups/:id", gin.WrapF(server.handlePersonaGroupByID))
		protected.PUT("/persona/groups/:id", gin.WrapF(server.handlePersonaGroupByID))
		protected.DELETE("/persona/groups/:id", gin.WrapF(server.handlePersonaGroupByID))
		protected.GET("/persona/groups/:id/members", gin.WrapF(server.handlePersonaGroupMembers))
		protected.POST("/persona/groups/:id/members", gin.WrapF(server.handlePersonaGroupMembers))
		protected.DELETE("/persona/groups/:id/members/:memberID", gin.WrapF(server.handlePersonaGroupMembers))
		protected.PUT("/persona/groups/:id/content", gin.WrapF(server.handlePersonaGroupContent))
		protected.GET("/persona/users", gin.WrapF(server.handlePersonaUsers))
		protected.POST("/persona/users", gin.WrapF(server.handlePersonaUsers))
		protected.GET("/persona/users/:id", gin.WrapF(server.handlePersonaUserByID))
		protected.PUT("/persona/users/:id", gin.WrapF(server.handlePersonaUserByID))
		protected.DELETE("/persona/users/:id", gin.WrapF(server.handlePersonaUserByID))
		protected.PUT("/persona/users/:id/content", gin.WrapF(server.handlePersonaUserContent))
		protected.GET("/persona/resolve", gin.WrapF(server.handlePersonaResolve))
		protected.GET("/soul", gin.WrapF(server.handleSoul))
		protected.PUT("/soul", gin.WrapF(server.handleSoul))
		protected.GET("/logs", gin.WrapF(server.handleLogs))
		protected.GET("/logs/stream", gin.WrapF(server.handleLogStream))
	}

	router.NoMethod(func(c *gin.Context) {
		methodNotAllowed(c.Writer)
	})
	router.NoRoute(gin.WrapF(server.handleFrontend))

	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           router,
		ReadHeaderTimeout: 8 * time.Second,
	}

	logger.Info("control server started", "addr", httpServer.Addr)
	if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("control server failed", "error", err.Error())
		os.Exit(1)
	}
}

func (s *apiServer) handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"status":    "ok",
		"service":   "tele-auto-control",
		"timestamp": time.Now().UTC().Format(time.RFC3339Nano),
		"worker":    s.manager.Status(),
	})
}

type adminLoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

func (s *apiServer) handleAdminMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	admin := s.getAdminAuth()
	if !admin.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":    false,
			"authenticated": false,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":    true,
		"authenticated": s.isAdminAuthenticated(r),
		"username":      admin.Username(),
	})
}

func (s *apiServer) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	admin := s.getAdminAuth()
	if !admin.Enabled() {
		errorJSON(w, http.StatusServiceUnavailable, errors.New("admin login is not configured on server"))
		return
	}

	var req adminLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	if !admin.VerifyCredentials(req.Username, req.Password) {
		errorJSON(w, http.StatusUnauthorized, errors.New("invalid username or password"))
		return
	}

	token, err := admin.IssueToken()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}
	admin.SetCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"configured":    true,
		"authenticated": true,
		"username":      admin.Username(),
	})
}

func (s *apiServer) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.getAdminAuth().ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

type updateAdminCredentialsRequest struct {
	CurrentPassword string `json:"currentPassword"`
	NewUsername     string `json:"newUsername"`
	NewPassword     string `json:"newPassword"`
}

func (s *apiServer) handleAdminCredentials(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}

	admin := s.getAdminAuth()
	if !admin.Enabled() {
		errorJSON(w, http.StatusServiceUnavailable, errors.New("admin login is not configured on server"))
		return
	}

	var req updateAdminCredentialsRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}

	currentPassword := req.CurrentPassword
	newUsername := strings.TrimSpace(req.NewUsername)
	newPassword := req.NewPassword
	currentUsername := admin.Username()

	if strings.TrimSpace(currentPassword) == "" {
		errorJSON(w, http.StatusBadRequest, errors.New("current password is required"))
		return
	}
	if !admin.VerifyCredentials(currentUsername, currentPassword) {
		errorJSON(w, http.StatusUnauthorized, errors.New("current password is incorrect"))
		return
	}
	if newUsername == "" {
		newUsername = currentUsername
	}
	if newUsername == currentUsername && strings.TrimSpace(newPassword) == "" {
		errorJSON(w, http.StatusBadRequest, errors.New("provide new username or new password"))
		return
	}
	if strings.TrimSpace(newPassword) != "" && len(newPassword) < 4 {
		errorJSON(w, http.StatusBadRequest, errors.New("new password must be at least 4 characters"))
		return
	}

	env, err := readDotEnv(".env")
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}

	newSalt := strings.TrimSpace(os.Getenv("ADMIN_PASSWORD_SALT"))
	newHash := strings.TrimSpace(strings.ToLower(os.Getenv("ADMIN_PASSWORD_HASH")))
	newSecret := strings.TrimSpace(os.Getenv("ADMIN_SESSION_SECRET"))
	if newSalt == "" {
		newSalt, err = randomHex(16)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
	}
	if newSecret == "" {
		newSecret, err = randomHex(32)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
	}
	if strings.TrimSpace(newPassword) != "" {
		newSalt, err = randomHex(16)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		newHash = adminauth.HashPassword(newPassword, newSalt)
		newSecret, err = randomHex(32) // rotate to invalidate old sessions
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
	}

	nextMgr, err := adminauth.NewManager(adminauth.Config{
		Username:      newUsername,
		PasswordHash:  newHash,
		PasswordSalt:  newSalt,
		SessionSecret: newSecret,
		SessionTTL:    time.Duration(readIntEnv("ADMIN_SESSION_TTL_HOURS", 24*7)) * time.Hour,
		CookieSecure:  readBoolEnv("COOKIE_SECURE", false),
	})
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}

	env["ADMIN_USERNAME"] = newUsername
	env["ADMIN_PASSWORD_HASH"] = newHash
	env["ADMIN_PASSWORD_SALT"] = newSalt
	env["ADMIN_SESSION_SECRET"] = newSecret
	if err := godotenv.Write(env, ".env"); err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}

	_ = os.Setenv("ADMIN_USERNAME", newUsername)
	_ = os.Setenv("ADMIN_PASSWORD_HASH", newHash)
	_ = os.Setenv("ADMIN_PASSWORD_SALT", newSalt)
	_ = os.Setenv("ADMIN_SESSION_SECRET", newSecret)

	s.setAdminAuth(nextMgr)
	nextMgr.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":              true,
		"reloginRequired": true,
		"username":        newUsername,
	})
}

func (s *apiServer) handleFrontend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		http.NotFound(w, r)
		return
	}
	if s.webDir == "" {
		http.NotFound(w, r)
		return
	}

	webAbs, err := filepath.Abs(s.webDir)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	cleanPath := path.Clean("/" + r.URL.Path)
	relPath := strings.TrimPrefix(cleanPath, "/")
	candidate := filepath.Join(webAbs, filepath.FromSlash(relPath))
	candidateClean := filepath.Clean(candidate)

	if candidateClean == webAbs || strings.HasPrefix(candidateClean, webAbs+string(os.PathSeparator)) {
		if info, err := os.Stat(candidateClean); err == nil && !info.IsDir() {
			http.ServeFile(w, r, candidateClean)
			return
		}
	}

	indexPath := filepath.Join(webAbs, "index.html")
	if _, err := os.Stat(indexPath); err != nil {
		http.NotFound(w, r)
		return
	}
	http.ServeFile(w, r, indexPath)
}

func (s *apiServer) handleAuthStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	cfg, err := config.LoadForLogin()
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"authorized": false,
			"configured": false,
			"error":      err.Error(),
		})
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	authorized, err := tgauth.Status(ctx, cfg)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"authorized":   authorized,
		"configured":   true,
		"session_file": cfg.Telegram.SessionFile,
	})
}

type loginRequest struct {
	Phone    string `json:"phone"`
	Code     string `json:"code"`
	Password string `json:"password"`
}

func (s *apiServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	var req loginRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}

	cfg, err := config.LoadForLogin()
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if strings.TrimSpace(req.Code) == "" {
		result, err := tgauth.RequestOTP(ctx, cfg, req.Phone)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if result.AlreadyAuth {
			writeJSON(w, http.StatusOK, map[string]any{
				"ok":            true,
				"alreadyLogged": true,
				"message":       "Already logged in.",
			})
			return
		}
		if strings.TrimSpace(result.PhoneCodeHash) == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("telegram did not return phone_code_hash"))
			return
		}
		s.saveOTP(result.Phone, result.PhoneCodeHash, result.TimeoutSeconds)
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":           true,
			"otpRequested": true,
			"deliveryType": result.CodeType,
			"nextType":     result.NextType,
			"timeoutSec":   result.TimeoutSeconds,
			"message":      "OTP requested. Check Telegram app/SMS and submit code to verify login.",
		})
		return
	}

	phone := strings.TrimSpace(req.Phone)
	if phone == "" {
		phone = strings.TrimSpace(cfg.Telegram.Phone)
	}
	if phone == "" {
		errorJSON(w, http.StatusBadRequest, errors.New("phone is required"))
		return
	}
	hash, ok := s.getOTPHash(phone)
	if !ok {
		errorJSON(w, http.StatusBadRequest, errors.New("OTP session not found or expired. Request OTP again."))
		return
	}
	if err := tgauth.VerifyOTP(ctx, cfg, phone, hash, req.Code, req.Password); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	s.clearOTP(phone)

	restarted := false
	startErr := s.manager.Start()
	if startErr == nil {
		restarted = true
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":         true,
		"started":    restarted,
		"startError": errorText(startErr),
	})
}

func (s *apiServer) handleLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	_ = s.manager.Stop(ctx)

	cfg, err := config.LoadForLogin()
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	ctx, cancel = context.WithTimeout(r.Context(), 25*time.Second)
	defer cancel()
	if err := tgauth.Logout(ctx, cfg); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *apiServer) handleServiceStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *apiServer) handleServiceStart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if err := s.manager.Start(); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *apiServer) handleServiceStop(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()
	if err := s.manager.Stop(ctx); err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

func (s *apiServer) handleServiceRestart(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	if err := s.manager.Restart(ctx); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, s.manager.Status())
}

var allowedSettingKeys = []string{
	"TG_API_ID",
	"TG_API_HASH",
	"OPENAI_BASE_URL",
	"OPENAI_API_KEY",
	"OPENAI_MODEL",
	"AI_CONTEXT_MESSAGE_LIMIT",
	"AUTO_REPLY_ENABLED",
}

func allowedSettingSet() map[string]struct{} {
	set := make(map[string]struct{}, len(allowedSettingKeys))
	for _, key := range allowedSettingKeys {
		set[key] = struct{}{}
	}
	return set
}

type updateSettingsRequest struct {
	Values map[string]string `json:"values"`
}

type upsertVariablesRequest struct {
	Values []store.GlobalVariable `json:"values"`
}

type upsertAgentRequest struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description"`
	Intents     []string `json:"intents"`
	Tools       []string `json:"tools"`
	Variables   []string `json:"variables"`
	Visibility  string   `json:"visibility"`
	AllowUsers  []string `json:"allowUsers"`
	Model       string   `json:"model"`
	Temperature float64  `json:"temperature"`
	Body        string   `json:"body"`
}

func (s *apiServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		env, err := readDotEnv(".env")
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"keys":   allowedSettingKeys,
			"values": pickAllowedValues(env),
		})
	case http.MethodPut:
		var req updateSettingsRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if len(req.Values) == 0 {
			errorJSON(w, http.StatusBadRequest, errors.New("values payload is required"))
			return
		}

		allowed := allowedSettingSet()
		env, err := readDotEnv(".env")
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		for key, value := range req.Values {
			if _, ok := allowed[key]; !ok {
				errorJSON(w, http.StatusBadRequest, fmt.Errorf("setting %s is not editable", key))
				return
			}
			env[key] = strings.TrimSpace(value)
			_ = os.Setenv(key, strings.TrimSpace(value))
		}
		if err := godotenv.Write(env, ".env"); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}

		restarted := false
		if s.manager.Status().Running {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := s.manager.Restart(ctx); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			restarted = true
		}
		if _, changed := req.Values["AUTO_REPLY_ENABLED"]; changed {
			s.convEvents.Publish(conversationstream.Event{
				Type:      "global_auto_reply_changed",
				Mode:      strings.TrimSpace(strings.ToLower(env["AUTO_REPLY_ENABLED"])),
				CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"restarted": restarted,
			"values":    pickAllowedValues(env),
		})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleVariables(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		values, err := s.db.ListGlobalVariables(r.Context())
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		masked := make([]map[string]any, 0, len(values))
		for _, v := range values {
			item := map[string]any{
				"key":       v.Key,
				"type":      v.Type,
				"updatedAt": v.UpdatedAt,
			}
			if strings.ToLower(v.Type) == "secret" {
				item["value"] = "********"
				item["masked"] = true
			} else {
				item["value"] = v.Value
				item["masked"] = false
			}
			masked = append(masked, item)
		}
		writeJSON(w, http.StatusOK, map[string]any{"values": masked})
	case http.MethodPut:
		var req upsertVariablesRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if len(req.Values) == 0 {
			errorJSON(w, http.StatusBadRequest, errors.New("values payload is required"))
			return
		}
		for i := range req.Values {
			req.Values[i].Key = strings.TrimSpace(strings.ToUpper(req.Values[i].Key))
			req.Values[i].Type = strings.TrimSpace(strings.ToLower(req.Values[i].Type))
		}
		if err := s.db.UpsertGlobalVariables(r.Context(), req.Values); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleVariableByKey(w http.ResponseWriter, r *http.Request) {
	key := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/variables/"))
	if key == "" || strings.Contains(key, "/") {
		errorJSON(w, http.StatusNotFound, errors.New("variable not found"))
		return
	}
	if r.Method != http.MethodDelete {
		methodNotAllowed(w)
		return
	}
	deleted, err := s.db.DeleteGlobalVariable(r.Context(), key)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	if !deleted {
		errorJSON(w, http.StatusNotFound, errors.New("variable not found"))
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *apiServer) handleAgents(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		writeJSON(w, http.StatusOK, map[string]any{"agents": s.agentMgr.List()})
	case http.MethodPost:
		var req upsertAgentRequest
		if err := decodeJSON(r, &req); err != nil {
			s.logger.Warn("agent create decode failed", "error", err.Error())
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		agent, err := s.agentMgr.Upsert(agents.UpsertAgentInput{
			ID:          req.ID,
			Name:        req.Name,
			Description: req.Description,
			Intents:     req.Intents,
			Tools:       req.Tools,
			Variables:   req.Variables,
			Visibility:  req.Visibility,
			AllowUsers:  req.AllowUsers,
			Model:       req.Model,
			Temperature: req.Temperature,
			Body:        req.Body,
		})
		if err != nil {
			s.logger.Warn("agent create failed", "id", strings.TrimSpace(req.ID), "error", err.Error())
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		restarted := false
		if s.manager.Status().Running {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := s.manager.Restart(ctx); err != nil {
				s.logger.Warn("agent create restart failed", "id", agent.ID, "error", err.Error())
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			restarted = true
		}
		s.logger.Info("agent created", "id", agent.ID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": agent, "restarted": restarted})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleAgentByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/agents/"))
	if id == "" || strings.Contains(id, "/") {
		errorJSON(w, http.StatusNotFound, errors.New("agent not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		agent, ok := s.agentMgr.Get(id)
		if !ok {
			errorJSON(w, http.StatusNotFound, errors.New("agent not found"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"agent": agent})
	case http.MethodPut:
		var req upsertAgentRequest
		if err := decodeJSON(r, &req); err != nil {
			s.logger.Warn("agent update decode failed", "id", id, "error", err.Error())
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if strings.TrimSpace(req.ID) == "" {
			req.ID = id
		}
		if strings.TrimSpace(req.ID) != id {
			s.logger.Warn("agent update id mismatch", "path_id", id, "body_id", strings.TrimSpace(req.ID))
			errorJSON(w, http.StatusBadRequest, errors.New("id mismatch"))
			return
		}
		agent, err := s.agentMgr.Upsert(agents.UpsertAgentInput{
			ID:          req.ID,
			Name:        req.Name,
			Description: req.Description,
			Intents:     req.Intents,
			Tools:       req.Tools,
			Variables:   req.Variables,
			Visibility:  req.Visibility,
			AllowUsers:  req.AllowUsers,
			Model:       req.Model,
			Temperature: req.Temperature,
			Body:        req.Body,
		})
		if err != nil {
			s.logger.Warn("agent update failed", "id", id, "error", err.Error())
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		restarted := false
		if s.manager.Status().Running {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := s.manager.Restart(ctx); err != nil {
				s.logger.Warn("agent update restart failed", "id", agent.ID, "error", err.Error())
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			restarted = true
		}
		s.logger.Info("agent updated", "id", agent.ID)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "agent": agent, "restarted": restarted})
	case http.MethodDelete:
		if err := s.agentMgr.Delete(id); err != nil {
			if strings.Contains(strings.ToLower(err.Error()), "not found") {
				errorJSON(w, http.StatusNotFound, err)
				return
			}
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		restarted := false
		if s.manager.Status().Running {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := s.manager.Restart(ctx); err != nil {
				s.logger.Warn("agent delete restart failed", "id", id, "error", err.Error())
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			restarted = true
		}
		s.logger.Info("agent deleted", "id", id)
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "restarted": restarted})
	default:
		methodNotAllowed(w)
	}
}

type setConversationModeRequest struct {
	Mode string `json:"mode"`
}

type sendConversationMessageRequest struct {
	Text string `json:"text"`
}

type updateBehaviorPolicyRequest = behavior.Policy

type upsertPersonaGroupRequest struct {
	ID          string  `json:"id"`
	Name        string  `json:"name"`
	Slug        string  `json:"slug"`
	Description string  `json:"description"`
	Path        string  `json:"path"`
	Markdown    *string `json:"markdown"`
}

type upsertPersonaGroupMemberRequest struct {
	UserID   string `json:"userId"`
	Username string `json:"username"`
}

type upsertPersonaUserProfileRequest struct {
	ID       string  `json:"id"`
	Label    string  `json:"label"`
	UserID   string  `json:"userId"`
	Username string  `json:"username"`
	Path     string  `json:"path"`
	Enabled  *bool   `json:"enabled"`
	Markdown *string `json:"markdown"`
}

type updatePersonaContentRequest struct {
	Content string `json:"content"`
}

func readBehaviorPathFromEnv() string {
	path := strings.TrimSpace(os.Getenv("BEHAVIOR_POLICY_PATH"))
	if path == "" {
		path = "./behavior.yaml"
	}
	return filepath.Clean(path)
}

func (s *apiServer) handleBehavior(w http.ResponseWriter, r *http.Request) {
	path := readBehaviorPathFromEnv()

	switch r.Method {
	case http.MethodGet:
		loaded := behavior.Load(path, s.logger)
		writeJSON(w, http.StatusOK, map[string]any{
			"path":     loaded.Path,
			"loadedAt": loaded.LoadedAt.Format(time.RFC3339Nano),
			"policy":   loaded.Policy,
		})
	case http.MethodPut:
		var req updateBehaviorPolicyRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		loaded, err := behavior.Write(path, req)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		restarted := false
		if s.manager.Status().Running {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := s.manager.Restart(ctx); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			restarted = true
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"restarted": restarted,
			"path":      loaded.Path,
			"loadedAt":  loaded.LoadedAt.Format(time.RFC3339Nano),
			"policy":    loaded.Policy,
		})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleBehaviorRuntime(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	states, err := s.db.ListBehaviorRuntimeStates(r.Context(), 500)
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}
	loaded := behavior.Load(readBehaviorPathFromEnv(), s.logger)
	writeJSON(w, http.StatusOK, map[string]any{
		"path":     loaded.Path,
		"loadedAt": loaded.LoadedAt.Format(time.RFC3339Nano),
		"policy":   loaded.Policy,
		"states":   states,
	})
}

func (s *apiServer) handlePersonaGroups(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.db.ListPersonaGroups(r.Context())
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groups": items})
	case http.MethodPost:
		var req upsertPersonaGroupRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		path := strings.TrimSpace(req.Path)
		if path == "" {
			seed := strings.TrimSpace(req.Slug)
			if seed == "" {
				seed = strings.TrimSpace(req.Name)
			}
			path = s.persona.DefaultGroupMarkdownPath(seed)
		}
		group, err := s.db.CreatePersonaGroup(r.Context(), store.PersonaGroupInput{
			ID:           req.ID,
			Name:         req.Name,
			Slug:         req.Slug,
			Description:  req.Description,
			MarkdownPath: path,
		})
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		markdown := ""
		if req.Markdown != nil {
			markdown = *req.Markdown
		}
		if _, err := s.persona.WriteGroupMarkdown(group.MarkdownPath, markdown); err != nil {
			s.logger.Warn("failed to initialize group markdown file", "group_id", group.ID, "error", err.Error())
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "group": group})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handlePersonaGroupByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/persona/groups/"))
	if id == "" || strings.Contains(id, "/") {
		errorJSON(w, http.StatusNotFound, errors.New("persona group not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		group, ok, err := s.db.GetPersonaGroup(r.Context(), id)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !ok {
			errorJSON(w, http.StatusNotFound, errors.New("persona group not found"))
			return
		}
		content, _ := s.persona.ReadGroupMarkdown(group.MarkdownPath)
		writeJSON(w, http.StatusOK, map[string]any{"group": group, "content": content})
	case http.MethodPut:
		existing, ok, err := s.db.GetPersonaGroup(r.Context(), id)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !ok {
			errorJSON(w, http.StatusNotFound, errors.New("persona group not found"))
			return
		}
		var req upsertPersonaGroupRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		path := strings.TrimSpace(req.Path)
		if path == "" {
			path = existing.MarkdownPath
		}
		group, err := s.db.UpdatePersonaGroup(r.Context(), id, store.PersonaGroupInput{
			ID:           id,
			Name:         req.Name,
			Slug:         req.Slug,
			Description:  req.Description,
			MarkdownPath: path,
		})
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if req.Markdown != nil {
			if _, err := s.persona.WriteGroupMarkdown(group.MarkdownPath, *req.Markdown); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "group": group})
	case http.MethodDelete:
		deleted, err := s.db.DeletePersonaGroup(r.Context(), id)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !deleted {
			errorJSON(w, http.StatusNotFound, errors.New("persona group not found"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handlePersonaGroupMembers(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/persona/groups/"))
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		errorJSON(w, http.StatusNotFound, errors.New("persona group member route not found"))
		return
	}
	groupID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if groupID == "" || action != "members" {
		errorJSON(w, http.StatusNotFound, errors.New("persona group member route not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		items, err := s.db.ListPersonaGroupMembers(r.Context(), groupID)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"groupId": groupID, "members": items})
	case http.MethodPost:
		var req upsertPersonaGroupMemberRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		member, err := s.db.CreatePersonaGroupMember(r.Context(), store.PersonaGroupMemberInput{
			GroupID:  groupID,
			UserID:   req.UserID,
			Username: req.Username,
		})
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "member": member})
	case http.MethodDelete:
		if len(parts) < 3 {
			errorJSON(w, http.StatusBadRequest, errors.New("member id is required"))
			return
		}
		memberID, err := strconv.ParseInt(strings.TrimSpace(parts[2]), 10, 64)
		if err != nil || memberID <= 0 {
			errorJSON(w, http.StatusBadRequest, errors.New("invalid member id"))
			return
		}
		deleted, err := s.db.DeletePersonaGroupMember(r.Context(), groupID, memberID)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !deleted {
			errorJSON(w, http.StatusNotFound, errors.New("persona group member not found"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handlePersonaGroupContent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/persona/groups/"))
	id = strings.TrimSpace(strings.TrimSuffix(id, "/content"))
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	group, ok, err := s.db.GetPersonaGroup(r.Context(), id)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		errorJSON(w, http.StatusNotFound, errors.New("persona group not found"))
		return
	}
	var req updatePersonaContentRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	path, err := s.persona.WriteGroupMarkdown(group.MarkdownPath, req.Content)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "groupId": group.ID, "path": path})
}

func (s *apiServer) handlePersonaUsers(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		items, err := s.db.ListPersonaUserProfiles(r.Context())
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"users": items})
	case http.MethodPost:
		var req upsertPersonaUserProfileRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		path := strings.TrimSpace(req.Path)
		if path == "" {
			path = s.persona.DefaultUserMarkdownPath(req.Label, req.ID)
		}
		enabled := true
		if req.Enabled != nil {
			enabled = *req.Enabled
		}
		user, err := s.db.CreatePersonaUserProfile(r.Context(), store.PersonaUserProfileInput{
			ID:           req.ID,
			Label:        req.Label,
			UserID:       req.UserID,
			Username:     req.Username,
			MarkdownPath: path,
			Enabled:      enabled,
		})
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		markdown := ""
		if req.Markdown != nil {
			markdown = *req.Markdown
		}
		if _, err := s.persona.WriteUserMarkdown(user.MarkdownPath, markdown); err != nil {
			s.logger.Warn("failed to initialize user persona markdown file", "profile_id", user.ID, "error", err.Error())
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handlePersonaUserByID(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/persona/users/"))
	if id == "" || strings.Contains(id, "/") {
		errorJSON(w, http.StatusNotFound, errors.New("persona user profile not found"))
		return
	}
	switch r.Method {
	case http.MethodGet:
		user, ok, err := s.db.GetPersonaUserProfile(r.Context(), id)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !ok {
			errorJSON(w, http.StatusNotFound, errors.New("persona user profile not found"))
			return
		}
		content, _ := s.persona.ReadUserMarkdown(user.MarkdownPath)
		writeJSON(w, http.StatusOK, map[string]any{"user": user, "content": content})
	case http.MethodPut:
		existing, ok, err := s.db.GetPersonaUserProfile(r.Context(), id)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !ok {
			errorJSON(w, http.StatusNotFound, errors.New("persona user profile not found"))
			return
		}
		var req upsertPersonaUserProfileRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		path := strings.TrimSpace(req.Path)
		if path == "" {
			path = existing.MarkdownPath
		}
		user, err := s.db.UpdatePersonaUserProfile(r.Context(), id, store.PersonaUserProfileInput{
			ID:           id,
			Label:        req.Label,
			UserID:       req.UserID,
			Username:     req.Username,
			MarkdownPath: path,
			Enabled: func() bool {
				if req.Enabled == nil {
					return existing.Enabled
				}
				return *req.Enabled
			}(),
		})
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if req.Markdown != nil {
			if _, err := s.persona.WriteUserMarkdown(user.MarkdownPath, *req.Markdown); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true, "user": user})
	case http.MethodDelete:
		deleted, err := s.db.DeletePersonaUserProfile(r.Context(), id)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if !deleted {
			errorJSON(w, http.StatusNotFound, errors.New("persona user profile not found"))
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{"ok": true})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handlePersonaUserContent(w http.ResponseWriter, r *http.Request) {
	id := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/persona/users/"))
	id = strings.TrimSpace(strings.TrimSuffix(id, "/content"))
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	user, ok, err := s.db.GetPersonaUserProfile(r.Context(), id)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	if !ok {
		errorJSON(w, http.StatusNotFound, errors.New("persona user profile not found"))
		return
	}
	var req updatePersonaContentRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	path, err := s.persona.WriteUserMarkdown(user.MarkdownPath, req.Content)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "userId": user.ID, "path": path})
}

func (s *apiServer) handlePersonaResolve(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	chatID := strings.TrimSpace(r.URL.Query().Get("chatId"))
	userID := strings.TrimSpace(r.URL.Query().Get("userId"))
	username := strings.TrimSpace(r.URL.Query().Get("username"))
	if userID == "" && strings.HasPrefix(chatID, "user:") {
		userID = strings.TrimSpace(strings.TrimPrefix(chatID, "user:"))
	}
	soulPath := strings.TrimSpace(os.Getenv("SOUL_PROMPT_PATH"))
	if soulPath == "" {
		soulPath = "./SOUL.md"
	}
	soulPath = filepath.Clean(soulPath)
	soulPrompt := ""
	if b, err := os.ReadFile(soulPath); err == nil {
		soulPrompt = string(b)
	}
	resolved, err := s.persona.Resolve(r.Context(), persona.ResolveInput{
		ChatID:   chatID,
		UserID:   userID,
		Username: username,
	}, soulPrompt)
	if err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"chatId":   chatID,
		"userId":   userID,
		"username": username,
		"resolved": resolved,
	})
}

func (s *apiServer) handleConversations(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		limit := 100
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
				limit = parsed
			}
		}

		rows, err := s.db.ListConversationSummaries(r.Context(), limit)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		modes, err := s.db.ListChatModes(r.Context())
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		runtimeStates, err := s.db.ListBehaviorRuntimeStates(r.Context(), 1000)
		if err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		runtimeByChat := make(map[string]store.BehaviorRuntimeState, len(runtimeStates))
		for _, state := range runtimeStates {
			runtimeByChat[state.ChatID] = state
		}
		unresolved := make([]string, 0)
		for _, row := range rows {
			if isPlaceholderName(row.ChatName) || strings.EqualFold(strings.TrimSpace(row.ChatName), strings.TrimSpace(row.ChatID)) {
				unresolved = append(unresolved, row.ChatID)
			}
		}
		resolvedNames := s.resolveConversationNamesViaDialogs(r.Context(), unresolved)
		globalEnabled := readBoolEnv("AUTO_REPLY_ENABLED", true)

		items := make([]map[string]any, 0, len(rows))
		for _, row := range rows {
			if name := strings.TrimSpace(resolvedNames[row.ChatID]); name != "" {
				row.ChatName = name
				_ = s.db.BackfillOtherPersonName(r.Context(), row.ChatID, name)
			}
			if isPlaceholderName(row.ChatName) {
				if resolved := s.resolveConversationName(r.Context(), row.ChatID); resolved != "" {
					row.ChatName = resolved
					_ = s.db.BackfillOtherPersonName(r.Context(), row.ChatID, resolved)
				}
			}
			mode, hasMode := modes[row.ChatID]
			runtime := runtimeByChat[row.ChatID]
			effective := effectiveConversationMode(globalEnabled, mode, hasMode, runtime.EscalatedManual)
			hasManual := hasMode && mode == "manual"
			items = append(items, map[string]any{
				"chatId":            row.ChatID,
				"chatName":          row.ChatName,
				"lastMessage":       row.LastMessage,
				"lastMessageAt":     row.LastMessageAt,
				"unreadIncoming":    row.UnreadIncoming,
				"effectiveMode":     effective,
				"hasManualOverride": hasManual,
				"mode":              mode,
				"escalatedManual":   runtime.EscalatedManual,
				"manualReason":      strings.TrimSpace(runtime.EscalationReason),
			})
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"globalAutoReplyEnabled": globalEnabled,
			"conversations":          items,
		})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleConversationByID(w http.ResponseWriter, r *http.Request) {
	rest := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/api/conversations/"))
	if rest == "" {
		errorJSON(w, http.StatusNotFound, errors.New("conversation route not found"))
		return
	}
	parts := strings.Split(rest, "/")
	if len(parts) < 2 {
		errorJSON(w, http.StatusNotFound, errors.New("conversation route not found"))
		return
	}
	chatID := strings.TrimSpace(parts[0])
	action := strings.TrimSpace(parts[1])
	if chatID == "" || action == "" {
		errorJSON(w, http.StatusBadRequest, errors.New("chat id and action are required"))
		return
	}

	switch action {
	case "messages":
		s.handleConversationMessages(w, r, chatID)
	case "mode":
		s.handleConversationMode(w, r, chatID)
	default:
		errorJSON(w, http.StatusNotFound, errors.New("conversation route not found"))
	}
}

func (s *apiServer) handleConversationMessages(w http.ResponseWriter, r *http.Request, chatID string) {
	switch r.Method {
	case http.MethodGet:
		limit := 50
		if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
			if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 200 {
				limit = parsed
			}
		}
		var before int64
		if raw := strings.TrimSpace(r.URL.Query().Get("before")); raw != "" {
			parsed, err := strconv.ParseInt(raw, 10, 64)
			if err != nil || parsed < 0 {
				errorJSON(w, http.StatusBadRequest, errors.New("invalid before value"))
				return
			}
			before = parsed
		}

		rows, err := s.db.ListConversationMessages(r.Context(), chatID, limit, before)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		resolvedOtherName := ""
		for i := range rows {
			if rows[i].Direction == "other_person" && isPlaceholderName(rows[i].SenderName) {
				if resolvedOtherName == "" {
					resolvedMap := s.resolveConversationNamesViaDialogs(r.Context(), []string{chatID})
					resolvedOtherName = strings.TrimSpace(resolvedMap[chatID])
					if resolvedOtherName == "" {
						resolvedOtherName = s.resolveConversationName(r.Context(), chatID)
					}
					if resolvedOtherName != "" {
						_ = s.db.BackfillOtherPersonName(r.Context(), chatID, resolvedOtherName)
					}
				}
				if resolvedOtherName != "" {
					rows[i].SenderName = resolvedOtherName
				}
			}
		}
		nextBefore := int64(0)
		if len(rows) > 0 {
			nextBefore = rows[0].ID
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"chatId":       chatID,
			"messages":     rows,
			"nextBefore":   nextBefore,
			"hasMore":      len(rows) == limit,
			"messageCount": len(rows),
		})
	case http.MethodPost:
		var req sendConversationMessageRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		text := strings.TrimSpace(req.Text)
		if text == "" {
			errorJSON(w, http.StatusBadRequest, errors.New("text is required"))
			return
		}
		ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
		defer cancel()
		record, err := s.manager.SendConversationMessage(ctx, chatID, text)
		if err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":      true,
			"message": record,
		})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleConversationMode(w http.ResponseWriter, r *http.Request, chatID string) {
	if r.Method != http.MethodPut {
		methodNotAllowed(w)
		return
	}
	var req setConversationModeRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	mode := strings.TrimSpace(strings.ToLower(req.Mode))
	if mode != "auto" && mode != "manual" {
		errorJSON(w, http.StatusBadRequest, errors.New("mode must be auto or manual"))
		return
	}
	if err := s.db.UpsertChatMode(r.Context(), chatID, mode); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	if mode == "auto" {
		if err := s.db.ClearBehaviorEscalation(r.Context(), chatID); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
	}
	globalEnabled := readBoolEnv("AUTO_REPLY_ENABLED", true)
	runtimeState, _, _ := s.db.GetBehaviorRuntimeState(r.Context(), chatID)
	effective := effectiveConversationMode(globalEnabled, mode, true, runtimeState.EscalatedManual)
	s.convEvents.Publish(conversationstream.Event{
		Type:      "mode_changed",
		ChatID:    chatID,
		Mode:      mode,
		Reason:    strings.TrimSpace(runtimeState.EscalationReason),
		CreatedAt: time.Now().UTC().Format(time.RFC3339Nano),
	})
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":                true,
		"chatId":            chatID,
		"mode":              mode,
		"effectiveMode":     effective,
		"globalAutoEnabled": globalEnabled,
		"escalatedManual":   runtimeState.EscalatedManual,
		"manualReason":      strings.TrimSpace(runtimeState.EscalationReason),
	})
}

type updateSoulRequest struct {
	Content string `json:"content"`
}

func (s *apiServer) handleSoul(w http.ResponseWriter, r *http.Request) {
	env, err := readDotEnv(".env")
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}
	soulPath := strings.TrimSpace(env["SOUL_PROMPT_PATH"])
	if soulPath == "" {
		soulPath = "./SOUL.md"
	}
	soulPath = filepath.Clean(soulPath)

	switch r.Method {
	case http.MethodGet:
		content := ""
		if b, err := os.ReadFile(soulPath); err == nil {
			content = string(b)
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"path":    soulPath,
			"content": content,
		})
	case http.MethodPut:
		var req updateSoulRequest
		if err := decodeJSON(r, &req); err != nil {
			errorJSON(w, http.StatusBadRequest, err)
			return
		}
		if err := os.MkdirAll(filepath.Dir(soulPath), 0o755); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		if err := os.WriteFile(soulPath, []byte(req.Content), 0o644); err != nil {
			errorJSON(w, http.StatusInternalServerError, err)
			return
		}
		restarted := false
		if s.manager.Status().Running {
			ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
			defer cancel()
			if err := s.manager.Restart(ctx); err != nil {
				errorJSON(w, http.StatusBadRequest, err)
				return
			}
			restarted = true
		}
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"restarted": restarted,
			"path":      soulPath,
		})
	default:
		methodNotAllowed(w)
	}
}

func (s *apiServer) handleLogs(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	limit := 200
	if raw := strings.TrimSpace(r.URL.Query().Get("limit")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 && parsed <= 500 {
			limit = parsed
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"logs": s.logs.Snapshot(limit),
	})
}

func (s *apiServer) handleLogStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		errorJSON(w, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for _, entry := range s.logs.Snapshot(200) {
		if err := writeSSE(w, "log", entry); err != nil {
			return
		}
	}
	flusher.Flush()

	ch, unsub := s.logs.Subscribe()
	defer unsub()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, "log", entry); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *apiServer) handleConversationStream(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		methodNotAllowed(w)
		return
	}
	flusher, ok := w.(http.Flusher)
	if !ok {
		errorJSON(w, http.StatusInternalServerError, errors.New("streaming is not supported"))
		return
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	for _, entry := range s.convEvents.Snapshot(200) {
		if err := writeSSE(w, "conversation", entry); err != nil {
			return
		}
	}
	flusher.Flush()

	ch, unsub := s.convEvents.Subscribe()
	defer unsub()

	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-r.Context().Done():
			return
		case <-ticker.C:
			if _, err := fmt.Fprint(w, ": keepalive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case entry, ok := <-ch:
			if !ok {
				return
			}
			if err := writeSSE(w, "conversation", entry); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

func (s *apiServer) requireAdminGin() gin.HandlerFunc {
	return func(c *gin.Context) {
		if s.isAdminAuthenticated(c.Request) {
			c.Next()
			return
		}
		c.AbortWithStatusJSON(http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
	}
}

func (s *apiServer) isAdminAuthenticated(r *http.Request) bool {
	admin := s.getAdminAuth()
	if !admin.Enabled() {
		return false
	}
	token, err := admin.TokenFromRequest(r)
	if err != nil {
		return false
	}
	return admin.ValidateToken(token)
}

func (s *apiServer) getAdminAuth() *adminauth.Manager {
	s.adminMu.RLock()
	defer s.adminMu.RUnlock()
	return s.adminAuth
}

func (s *apiServer) setAdminAuth(next *adminauth.Manager) {
	s.adminMu.Lock()
	defer s.adminMu.Unlock()
	s.adminAuth = next
}

func randomHex(numBytes int) (string, error) {
	if numBytes <= 0 {
		return "", errors.New("numBytes must be positive")
	}
	buf := make([]byte, numBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func writeSSE(w http.ResponseWriter, event string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "event: %s\n", event); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", string(body)); err != nil {
		return err
	}
	return nil
}

func corsMiddleware(frontendBase string) gin.HandlerFunc {
	allowedOrigin := "http://localhost:5173"
	if frontendBase != "" {
		allowedOrigin = frontendBase
	}
	return func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", allowedOrigin)
		c.Header("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type")
		c.Header("Access-Control-Allow-Credentials", "true")
		if c.Request.Method == http.MethodOptions {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	}
}

func readServerPort() int {
	if raw := strings.TrimSpace(os.Getenv("CONTROL_PORT")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	if raw := strings.TrimSpace(os.Getenv("PORT")); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v > 0 {
			return v
		}
	}
	return 3000
}

func readWebDir() string {
	if raw := strings.TrimSpace(os.Getenv("WEB_DIR")); raw != "" {
		return raw
	}
	return "./web"
}

func readPersonaGroupsDir() string {
	if raw := strings.TrimSpace(os.Getenv("PERSONA_GROUPS_DIR")); raw != "" {
		return raw
	}
	return "./personality/groups"
}

func readPersonaUsersDir() string {
	if raw := strings.TrimSpace(os.Getenv("PERSONA_USERS_DIR")); raw != "" {
		return raw
	}
	return "./personality/users"
}

func effectiveConversationMode(globalEnabled bool, mode string, hasMode bool, escalatedManual bool) string {
	if !globalEnabled {
		return "manual"
	}
	if escalatedManual {
		return "manual"
	}
	if hasMode && strings.TrimSpace(strings.ToLower(mode)) == "manual" {
		return "manual"
	}
	return "auto"
}

func isPlaceholderName(name string) bool {
	n := strings.TrimSpace(strings.ToLower(name))
	if n == "" {
		return true
	}
	return n == "other_person" || n == "unknown"
}

func (s *apiServer) resolveConversationName(ctx context.Context, chatID string) string {
	cctx, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	name, err := s.manager.ResolveConversationName(cctx, chatID)
	if err != nil {
		return ""
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return ""
	}
	return name
}

func (s *apiServer) resolveConversationNamesViaDialogs(ctx context.Context, chatIDs []string) map[string]string {
	out := map[string]string{}
	if len(chatIDs) == 0 {
		return out
	}
	userIDs := map[int64]string{}
	for _, chatID := range chatIDs {
		chatID = strings.TrimSpace(chatID)
		if !strings.HasPrefix(chatID, "user:") {
			continue
		}
		raw := strings.TrimSpace(strings.TrimPrefix(chatID, "user:"))
		uid, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || uid <= 0 {
			continue
		}
		userIDs[uid] = chatID
	}
	if len(userIDs) == 0 {
		return out
	}

	cfg, err := config.LoadForLogin()
	if err != nil {
		return out
	}
	client := telegram.NewClient(cfg.Telegram.APIID, cfg.Telegram.APIHash, telegram.Options{
		SessionStorage: &session.FileStorage{Path: cfg.Telegram.SessionFile},
	})
	runCtx, cancel := context.WithTimeout(ctx, 6*time.Second)
	defer cancel()

	_ = client.Run(runCtx, func(ctx context.Context) error {
		status, err := client.Auth().Status(ctx)
		if err != nil || !status.Authorized {
			return err
		}
		raw := tg.NewClient(client)
		resp, err := raw.MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
			OffsetDate: 0,
			OffsetID:   0,
			OffsetPeer: &tg.InputPeerEmpty{},
			Limit:      100,
			Hash:       0,
		})
		if err != nil {
			return err
		}
		modified, ok := resp.AsModified()
		if !ok {
			return nil
		}
		for _, u := range modified.GetUsers() {
			usr, ok := u.AsNotEmpty()
			if !ok {
				continue
			}
			chatID, wanted := userIDs[usr.ID]
			if !wanted {
				continue
			}
			first, _ := usr.GetFirstName()
			last, _ := usr.GetLastName()
			name := strings.TrimSpace(strings.TrimSpace(first + " " + last))
			if name == "" {
				if username := strings.TrimSpace(usr.Username); username != "" {
					name = "@" + username
				}
			}
			if name == "" {
				if phone := strings.TrimSpace(usr.Phone); phone != "" {
					name = "+" + phone
				}
			}
			if name != "" {
				out[chatID] = name
			}
		}
		return nil
	})
	return out
}

func readBoolEnv(name string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if raw == "" {
		return fallback
	}
	return raw == "true" || raw == "1" || raw == "yes" || raw == "on"
}

func readIntEnv(name string, fallback int) int {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return fallback
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		return fallback
	}
	return v
}

func readDotEnv(path string) (map[string]string, error) {
	env, err := godotenv.Read(path)
	if err == nil {
		return env, nil
	}
	if errors.Is(err, os.ErrNotExist) || strings.Contains(strings.ToLower(err.Error()), "no such file") {
		return map[string]string{}, nil
	}
	return nil, err
}

func pickAllowedValues(env map[string]string) map[string]string {
	out := make(map[string]string, len(allowedSettingKeys))
	keys := append([]string(nil), allowedSettingKeys...)
	sort.Strings(keys)
	for _, key := range keys {
		out[key] = strings.TrimSpace(env[key])
	}
	return out
}

func decodeJSON(r *http.Request, out any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(out); err != nil {
		return err
	}
	return nil
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func errorJSON(w http.ResponseWriter, code int, err error) {
	if err == nil {
		err = errors.New("unknown error")
	}
	writeJSON(w, code, map[string]string{"error": err.Error()})
}

func methodNotAllowed(w http.ResponseWriter) {
	errorJSON(w, http.StatusMethodNotAllowed, errors.New("method not allowed"))
}

func errorText(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}

func normalizePhoneKey(phone string) string {
	return strings.TrimSpace(phone)
}

func (s *apiServer) saveOTP(phone, hash string, timeoutSec int) {
	s.otpMu.Lock()
	s.otpByPhone[normalizePhoneKey(phone)] = otpState{
		phoneCodeHash: strings.TrimSpace(hash),
		createdAt:     time.Now().UTC(),
		timeoutSec:    timeoutSec,
	}
	s.otpMu.Unlock()
}

func (s *apiServer) getOTPHash(phone string) (string, bool) {
	s.otpMu.Lock()
	defer s.otpMu.Unlock()

	key := normalizePhoneKey(phone)
	state, ok := s.otpByPhone[key]
	if !ok {
		return "", false
	}

	ttl := 10 * time.Minute
	if state.timeoutSec > 0 {
		ttl = time.Duration(state.timeoutSec+120) * time.Second
	}
	if time.Since(state.createdAt) > ttl {
		delete(s.otpByPhone, key)
		return "", false
	}
	return state.phoneCodeHash, true
}

func (s *apiServer) clearOTP(phone string) {
	s.otpMu.Lock()
	delete(s.otpByPhone, normalizePhoneKey(phone))
	s.otpMu.Unlock()
}
