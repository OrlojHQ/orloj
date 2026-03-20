package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

type authConfigResponse struct {
	Mode          string   `json:"mode"`
	SetupRequired bool     `json:"setup_required"`
	LoginMethods  []string `json:"login_methods"`
}

type authRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type authMeResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	Method        string `json:"method,omitempty"`
}

type authResetPasswordRequest struct {
	Username    string `json:"username,omitempty"`
	NewPassword string `json:"new_password"`
}

type authChangePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

func (s *Server) handleAuthConfig(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	resp := authConfigResponse{Mode: string(s.authMode)}
	switch s.authMode {
	case AuthModeLocal:
		resp.LoginMethods = []string{"password"}
		hasAdmin, err := s.stores.LocalAdmins.HasAdmin()
		if err != nil {
			http.Error(w, "auth store error", http.StatusInternalServerError)
			return
		}
		resp.SetupRequired = !hasAdmin
	case AuthModeSSO:
		resp.LoginMethods = []string{"sso"}
	default:
		resp.Mode = string(AuthModeOff)
		resp.LoginMethods = []string{}
	}
	writeJSON(w, http.StatusOK, resp)
}

func (s *Server) handleAuthSetup(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeLocal {
		http.Error(w, "auth setup is only available in local mode", http.StatusBadRequest)
		return
	}
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if err := store.ValidatePasswordPolicy(req.Password, 12); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	hasAdmin, err := s.stores.LocalAdmins.HasAdmin()
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if hasAdmin {
		http.Error(w, "admin account is already configured", http.StatusConflict)
		return
	}
	hash, err := store.GeneratePasswordHash(req.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	if err := s.stores.LocalAdmins.Upsert(req.Username, hash); err != nil {
		http.Error(w, "failed to store admin account", http.StatusInternalServerError)
		return
	}
	session, err := s.stores.AuthSessions.Create(req.Username, s.sessionTTL, time.Now().UTC())
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, r, session.ID, s.sessionTTL)
	writeJSON(w, http.StatusCreated, authMeResponse{Authenticated: true, Username: req.Username, Method: "session"})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeLocal {
		http.Error(w, "auth login is only available in local mode", http.StatusBadRequest)
		return
	}
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	admin, hasAdmin, err := s.stores.LocalAdmins.Get()
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !hasAdmin {
		http.Error(w, "admin setup required", http.StatusConflict)
		return
	}
	if !strings.EqualFold(admin.Username, req.Username) {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	ok, err := store.VerifyPasswordHash(admin.PasswordHash, req.Password)
	if err != nil || !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	_ = s.stores.AuthSessions.DeleteExpired(time.Now().UTC())
	session, err := s.stores.AuthSessions.Create(admin.Username, s.sessionTTL, time.Now().UTC())
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, r, session.ID, s.sessionTTL)
	writeJSON(w, http.StatusOK, authMeResponse{Authenticated: true, Username: admin.Username, Method: "session"})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeLocal {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
		return
	}
	sessionID := readSessionID(r)
	if sessionID != "" {
		_ = s.stores.AuthSessions.Delete(sessionID)
	}
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) handleAuthMe(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeLocal {
		writeJSON(w, http.StatusOK, authMeResponse{Authenticated: true, Method: "none"})
		return
	}

	if sessionID := readSessionID(r); sessionID != "" {
		session, ok, err := s.stores.AuthSessions.Get(sessionID)
		if err == nil && ok {
			expiresAt, parseErr := time.Parse(time.RFC3339Nano, session.ExpiresAt)
			if parseErr == nil && expiresAt.After(time.Now().UTC()) {
				_ = s.stores.AuthSessions.Touch(sessionID, s.sessionTTL, time.Now().UTC())
				writeJSON(w, http.StatusOK, authMeResponse{Authenticated: true, Username: session.Username, Method: "session"})
				return
			}
			_ = s.stores.AuthSessions.Delete(sessionID)
		}
	}

	if s.authorizer != nil {
		if allowed, _, _ := s.authorizer.Authorize(r, "reader"); allowed {
			writeJSON(w, http.StatusOK, authMeResponse{Authenticated: true, Method: "bearer"})
			return
		}
	}
	writeJSON(w, http.StatusOK, authMeResponse{Authenticated: false})
}

func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeLocal {
		http.Error(w, "password change is only available in local mode", http.StatusBadRequest)
		return
	}

	var req authChangePasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if strings.TrimSpace(req.CurrentPassword) == "" {
		http.Error(w, "current_password is required", http.StatusBadRequest)
		return
	}
	if err := store.ValidatePasswordPolicy(req.NewPassword, 12); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	admin, hasAdmin, err := s.stores.LocalAdmins.Get()
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !hasAdmin {
		http.Error(w, "admin setup required", http.StatusConflict)
		return
	}

	authenticated := false
	if sessionID := readSessionID(r); sessionID != "" {
		session, ok, sessionErr := s.stores.AuthSessions.Get(sessionID)
		if sessionErr != nil {
			http.Error(w, "session lookup failed", http.StatusInternalServerError)
			return
		}
		if ok {
			expiresAt, parseErr := time.Parse(time.RFC3339Nano, session.ExpiresAt)
			if parseErr == nil && expiresAt.After(time.Now().UTC()) && strings.EqualFold(session.Username, admin.Username) {
				authenticated = true
			} else if parseErr != nil || !expiresAt.After(time.Now().UTC()) {
				_ = s.stores.AuthSessions.Delete(sessionID)
			}
		}
	}
	if !authenticated && s.authorizer != nil {
		allowed, _, _ := s.authorizer.Authorize(r, "admin")
		authenticated = allowed
	}
	if !authenticated {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	ok, verifyErr := store.VerifyPasswordHash(admin.PasswordHash, req.CurrentPassword)
	if verifyErr != nil || !ok {
		http.Error(w, "invalid current password", http.StatusUnauthorized)
		return
	}

	hash, hashErr := store.GeneratePasswordHash(req.NewPassword)
	if hashErr != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	if upsertErr := s.stores.LocalAdmins.Upsert(admin.Username, hash); upsertErr != nil {
		http.Error(w, "failed to update password", http.StatusInternalServerError)
		return
	}
	_ = s.stores.AuthSessions.DeleteByUsername(admin.Username)
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

func (s *Server) handleAuthAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeLocal {
		http.Error(w, "password reset is only available in local mode", http.StatusBadRequest)
		return
	}
	if s.authorizer != nil {
		allowed, status, message := s.authorizer.Authorize(r, "admin")
		if !allowed {
			if status <= 0 {
				status = http.StatusForbidden
			}
			http.Error(w, strings.TrimSpace(message), status)
			return
		}
	}

	var req authResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if err := store.ValidatePasswordPolicy(req.NewPassword, 12); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	admin, hasAdmin, err := s.stores.LocalAdmins.Get()
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !hasAdmin {
		http.Error(w, "admin setup required", http.StatusConflict)
		return
	}
	if req.Username != "" && !strings.EqualFold(req.Username, admin.Username) {
		http.Error(w, "only the existing admin username may be reset", http.StatusBadRequest)
		return
	}

	hash, err := store.GeneratePasswordHash(req.NewPassword)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	if err := s.stores.LocalAdmins.Upsert(admin.Username, hash); err != nil {
		http.Error(w, "failed to reset password", http.StatusInternalServerError)
		return
	}
	_ = s.stores.AuthSessions.DeleteByUsername(admin.Username)
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password reset"})
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	cookie := &http.Cookie{
		Name:     sessionCookieName,
		Value:    strings.TrimSpace(sessionID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  time.Now().UTC().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	}
	http.SetCookie(w, cookie)
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookieName,
		Value:    "",
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isSecureRequest(r),
		Expires:  time.Unix(0, 0).UTC(),
		MaxAge:   -1,
	})
}

func isSecureRequest(r *http.Request) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if r == nil {
		return false
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if strings.Contains(proto, ",") {
		proto = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	return strings.EqualFold(proto, "https")
}
