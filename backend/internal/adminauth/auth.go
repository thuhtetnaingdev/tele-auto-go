package adminauth

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"
)

const CookieName = "tele_auto_session"

type Config struct {
	Username      string
	PasswordHash  string
	PasswordSalt  string
	SessionSecret string
	SessionTTL    time.Duration
	CookieSecure  bool
}

type Manager struct {
	cfg Config
}

type sessionClaims struct {
	Username string `json:"u"`
	ExpUnix  int64  `json:"e"`
	Nonce    string `json:"n"`
}

func NewManager(cfg Config) (*Manager, error) {
	cfg.Username = strings.TrimSpace(cfg.Username)
	cfg.PasswordHash = strings.TrimSpace(strings.ToLower(cfg.PasswordHash))
	cfg.PasswordSalt = strings.TrimSpace(cfg.PasswordSalt)
	cfg.SessionSecret = strings.TrimSpace(cfg.SessionSecret)

	enabled := cfg.Username != "" || cfg.PasswordHash != "" || cfg.PasswordSalt != "" || cfg.SessionSecret != ""
	if !enabled {
		return &Manager{cfg: cfg}, nil
	}

	if cfg.Username == "" || cfg.PasswordHash == "" || cfg.PasswordSalt == "" || cfg.SessionSecret == "" {
		return nil, errors.New("admin auth enabled but ADMIN_* values are incomplete")
	}
	if len(cfg.SessionSecret) < 16 {
		return nil, errors.New("ADMIN_SESSION_SECRET must be at least 16 characters")
	}
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 24 * time.Hour
	}

	return &Manager{cfg: cfg}, nil
}

func (m *Manager) Enabled() bool {
	return m != nil && m.cfg.Username != "" && m.cfg.PasswordHash != "" && m.cfg.PasswordSalt != "" && m.cfg.SessionSecret != ""
}

func (m *Manager) Username() string {
	if m == nil {
		return ""
	}
	return m.cfg.Username
}

func HashPassword(password, salt string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(salt) + ":" + password))
	return hex.EncodeToString(sum[:])
}

func secureEqual(a, b string) bool {
	ab := []byte(a)
	bb := []byte(b)
	if len(ab) != len(bb) {
		return false
	}
	return subtle.ConstantTimeCompare(ab, bb) == 1
}

func (m *Manager) VerifyCredentials(username, password string) bool {
	if !m.Enabled() {
		return true
	}
	if !secureEqual(strings.TrimSpace(username), m.cfg.Username) {
		return false
	}
	calc := HashPassword(password, m.cfg.PasswordSalt)
	return secureEqual(calc, strings.ToLower(m.cfg.PasswordHash))
}

func (m *Manager) IssueToken() (string, error) {
	if !m.Enabled() {
		return "", errors.New("admin auth is disabled")
	}
	nonce := make([]byte, 16)
	if _, err := rand.Read(nonce); err != nil {
		return "", err
	}

	claims := sessionClaims{
		Username: m.cfg.Username,
		ExpUnix:  time.Now().UTC().Add(m.cfg.SessionTTL).Unix(),
		Nonce:    base64.RawURLEncoding.EncodeToString(nonce),
	}
	payload, err := json.Marshal(claims)
	if err != nil {
		return "", err
	}
	sig := m.sign(payload)
	return base64.RawURLEncoding.EncodeToString(payload) + "." + base64.RawURLEncoding.EncodeToString(sig), nil
}

func (m *Manager) ValidateToken(token string) bool {
	if !m.Enabled() {
		return true
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 {
		return false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return false
	}
	sig, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return false
	}
	expected := m.sign(payload)
	if subtle.ConstantTimeCompare(sig, expected) != 1 {
		return false
	}

	var claims sessionClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return false
	}
	if !secureEqual(claims.Username, m.cfg.Username) {
		return false
	}
	if claims.ExpUnix <= time.Now().UTC().Unix() {
		return false
	}
	if strings.TrimSpace(claims.Nonce) == "" {
		return false
	}
	return true
}

func (m *Manager) sign(payload []byte) []byte {
	mac := hmac.New(sha256.New, []byte(m.cfg.SessionSecret))
	_, _ = mac.Write(payload)
	return mac.Sum(nil)
}

func (m *Manager) SetCookie(w http.ResponseWriter, token string) {
	if !m.Enabled() {
		return
	}
	maxAge := int(m.cfg.SessionTTL.Seconds())
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    token,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   m.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *Manager) ClearCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     CookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   m.cfg.CookieSecure,
		SameSite: http.SameSiteLaxMode,
	})
}

func (m *Manager) TokenFromRequest(r *http.Request) (string, error) {
	if !m.Enabled() {
		return "", nil
	}
	c, err := r.Cookie(CookieName)
	if err != nil {
		return "", err
	}
	value := strings.TrimSpace(c.Value)
	if value == "" {
		return "", fmt.Errorf("empty auth cookie")
	}
	return value, nil
}
