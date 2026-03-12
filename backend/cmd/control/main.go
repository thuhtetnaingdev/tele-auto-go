package main

import (
	"context"
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

	"github.com/joho/godotenv"

	"tele-auto-go/internal/adminauth"
	"tele-auto-go/internal/config"
	"tele-auto-go/internal/control"
	"tele-auto-go/internal/logging"
	"tele-auto-go/internal/logstream"
	"tele-auto-go/internal/tgauth"
)

type apiServer struct {
	logger       *slog.Logger
	manager      *control.Manager
	logs         *logstream.Hub
	frontendBase string
	webDir       string
	adminAuth    *adminauth.Manager

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
	logger := logging.NewWithHub(level, logs)
	manager := control.NewManager(logger)
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
		frontendBase: strings.TrimSpace(os.Getenv("FRONTEND_ORIGIN")),
		webDir:       readWebDir(),
		adminAuth:    admin,
		otpByPhone:   make(map[string]otpState),
	}
	if info, err := os.Stat(server.webDir); err == nil && info.IsDir() {
		logger.Info("frontend web dir detected", "web_dir", server.webDir)
	} else {
		logger.Warn("frontend web dir not found; API-only mode", "web_dir", server.webDir)
	}
	if server.adminAuth.Enabled() {
		logger.Info("admin auth enabled", "username", server.adminAuth.Username())
	} else {
		logger.Warn("admin auth is disabled; dashboard is open without login")
	}

	if err := manager.Start(); err != nil {
		logger.Warn("telegram worker did not auto-start", "error", err.Error())
	}

	port := readServerPort()
	mux := http.NewServeMux()
	mux.HandleFunc("/health", server.handleHealth)
	mux.HandleFunc("/api/admin/me", server.handleAdminMe)
	mux.HandleFunc("/api/admin/login", server.handleAdminLogin)
	mux.HandleFunc("/api/admin/logout", server.handleAdminLogout)
	mux.HandleFunc("/api/auth/status", server.requireAdmin(server.handleAuthStatus))
	mux.HandleFunc("/api/auth/login", server.requireAdmin(server.handleLogin))
	mux.HandleFunc("/api/auth/logout", server.requireAdmin(server.handleLogout))
	mux.HandleFunc("/api/service/status", server.requireAdmin(server.handleServiceStatus))
	mux.HandleFunc("/api/service/start", server.requireAdmin(server.handleServiceStart))
	mux.HandleFunc("/api/service/stop", server.requireAdmin(server.handleServiceStop))
	mux.HandleFunc("/api/service/restart", server.requireAdmin(server.handleServiceRestart))
	mux.HandleFunc("/api/settings", server.requireAdmin(server.handleSettings))
	mux.HandleFunc("/api/soul", server.requireAdmin(server.handleSoul))
	mux.HandleFunc("/api/logs", server.requireAdmin(server.handleLogs))
	mux.HandleFunc("/api/logs/stream", server.requireAdmin(server.handleLogStream))
	mux.HandleFunc("/", server.handleFrontend)

	root := withCORS(server.frontendBase, mux)
	httpServer := &http.Server{
		Addr:              ":" + strconv.Itoa(port),
		Handler:           root,
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
	if !s.adminAuth.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"configured":    false,
			"authenticated": true,
		})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"configured":    true,
		"authenticated": s.isAdminAuthenticated(r),
		"username":      s.adminAuth.Username(),
	})
}

func (s *apiServer) handleAdminLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	if !s.adminAuth.Enabled() {
		writeJSON(w, http.StatusOK, map[string]any{
			"ok":            true,
			"configured":    false,
			"authenticated": true,
		})
		return
	}

	var req adminLoginRequest
	if err := decodeJSON(r, &req); err != nil {
		errorJSON(w, http.StatusBadRequest, err)
		return
	}
	if !s.adminAuth.VerifyCredentials(req.Username, req.Password) {
		errorJSON(w, http.StatusUnauthorized, errors.New("invalid username or password"))
		return
	}

	token, err := s.adminAuth.IssueToken()
	if err != nil {
		errorJSON(w, http.StatusInternalServerError, err)
		return
	}
	s.adminAuth.SetCookie(w, token)
	writeJSON(w, http.StatusOK, map[string]any{
		"ok":            true,
		"configured":    true,
		"authenticated": true,
		"username":      s.adminAuth.Username(),
	})
}

func (s *apiServer) handleAdminLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		methodNotAllowed(w)
		return
	}
	s.adminAuth.ClearCookie(w)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
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
	"TG_PHONE",
	"OPENAI_BASE_URL",
	"OPENAI_API_KEY",
	"OPENAI_MODEL",
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

		writeJSON(w, http.StatusOK, map[string]any{
			"ok":        true,
			"restarted": restarted,
			"values":    pickAllowedValues(env),
		})
	default:
		methodNotAllowed(w)
	}
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

func (s *apiServer) requireAdmin(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if s.isAdminAuthenticated(r) {
			next(w, r)
			return
		}
		errorJSON(w, http.StatusUnauthorized, errors.New("unauthorized"))
	}
}

func (s *apiServer) isAdminAuthenticated(r *http.Request) bool {
	if !s.adminAuth.Enabled() {
		return true
	}
	token, err := s.adminAuth.TokenFromRequest(r)
	if err != nil {
		return false
	}
	return s.adminAuth.ValidateToken(token)
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

func withCORS(frontendBase string, next http.Handler) http.Handler {
	allowedOrigin := "http://localhost:5173"
	if frontendBase != "" {
		allowedOrigin = frontendBase
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")
		w.Header().Set("Access-Control-Allow-Credentials", "true")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
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
