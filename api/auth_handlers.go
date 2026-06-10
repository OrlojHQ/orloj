package api

import (
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

type authConfigResponse struct {
	Mode               string   `json:"mode"`
	SetupRequired      bool     `json:"setup_required"`
	SetupTokenRequired bool     `json:"setup_token_required"`
	LoginMethods       []string `json:"login_methods"`
}

type authRequest struct {
	Username   string `json:"username"`
	Password   string `json:"password"`
	SetupToken string `json:"setup_token,omitempty"`
}

type authMeResponse struct {
	Authenticated bool   `json:"authenticated"`
	Username      string `json:"username,omitempty"`
	Name          string `json:"name,omitempty"`
	Role          string `json:"role,omitempty"`
	Method        string `json:"method,omitempty"`
}

type authResetPasswordRequest struct {
	Username    string `json:"username"`
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
	case AuthModeNative:
		resp.LoginMethods = []string{"password"}
		resp.SetupTokenRequired = strings.TrimSpace(os.Getenv("ORLOJ_SETUP_TOKEN")) != ""
		userCount, err := s.stores.LocalAdmins.CountUsers()
		if err != nil {
			http.Error(w, "auth store error", http.StatusInternalServerError)
			return
		}
		resp.SetupRequired = userCount == 0
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
	if !s.authRateLimiter.allow(r) {
		http.Error(w, "too many authentication attempts", http.StatusTooManyRequests)
		return
	}
	if s.authMode != AuthModeNative {
		http.Error(w, "auth setup is only available in native mode", http.StatusBadRequest)
		return
	}
	var req authRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	if requiredToken := strings.TrimSpace(os.Getenv("ORLOJ_SETUP_TOKEN")); requiredToken != "" {
		provided := strings.TrimSpace(req.SetupToken)
		if subtle.ConstantTimeCompare([]byte(provided), []byte(requiredToken)) != 1 {
			http.Error(w, "invalid or missing setup_token", http.StatusForbidden)
			return
		}
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
	hash, err := store.GeneratePasswordHash(req.Password)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	// CreateFirstAdmin atomically checks that no users exist and inserts the
	// admin in one operation, preventing the TOCTOU race where concurrent
	// setup requests both observe zero users.
	user, err := s.stores.LocalAdmins.CreateFirstAdmin(req.Username, hash)
	if err != nil {
		if errors.Is(err, store.ErrAuthUserExists) {
			http.Error(w, "admin account is already configured", http.StatusConflict)
			return
		}
		http.Error(w, "failed to store admin account", http.StatusInternalServerError)
		return
	}
	session, err := s.stores.AuthSessions.Create(user.Username, s.sessionTTL, time.Now().UTC())
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, r, session.ID, s.sessionTTL)
	writeJSON(w, http.StatusCreated, authMeResponse{
		Authenticated: true,
		Username:      user.Username,
		Name:          user.Username,
		Role:          user.Role,
		Method:        "session",
	})
}

func (s *Server) handleAuthLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authRateLimiter.allow(r) {
		http.Error(w, "too many authentication attempts", http.StatusTooManyRequests)
		return
	}
	if s.authMode != AuthModeNative {
		http.Error(w, "auth login is only available in native mode", http.StatusBadRequest)
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
	user, ok, err := s.stores.LocalAdmins.GetByUsername(req.Username)
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !ok {
		userCount, countErr := s.stores.LocalAdmins.CountUsers()
		if countErr != nil {
			http.Error(w, "auth store error", http.StatusInternalServerError)
			return
		}
		if userCount == 0 {
			http.Error(w, "admin setup required", http.StatusConflict)
			return
		}
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	ok, err = store.VerifyPasswordHash(user.PasswordHash, req.Password)
	if err != nil || !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	_ = s.stores.AuthSessions.DeleteExpired(time.Now().UTC())
	session, err := s.stores.AuthSessions.Create(user.Username, s.sessionTTL, time.Now().UTC())
	if err != nil {
		http.Error(w, "failed to create session", http.StatusInternalServerError)
		return
	}
	s.setSessionCookie(w, r, session.ID, s.sessionTTL)
	writeJSON(w, http.StatusOK, authMeResponse{
		Authenticated: true,
		Username:      user.Username,
		Name:          user.Username,
		Role:          user.Role,
		Method:        "session",
	})
}

func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if s.authMode != AuthModeNative {
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
	if s.authMode == AuthModeNative {
		user, ok, err := s.authenticatedSessionUser(r, true)
		if err != nil {
			http.Error(w, "auth store error", http.StatusInternalServerError)
			return
		}
		if ok {
			writeJSON(w, http.StatusOK, authMeResponse{
				Authenticated: true,
				Username:      user.Username,
				Name:          user.Username,
				Role:          user.Role,
				Method:        "session",
			})
			return
		}
		if bearerToken(r.Header.Get("Authorization")) != "" {
			allowed, status, _, identity := authorizeWithIdentity(s.authorizer, r, "reader")
			if status >= 500 {
				http.Error(w, "authorization lookup failed", http.StatusInternalServerError)
				return
			}
			if allowed {
				writeJSON(w, http.StatusOK, authMeFromIdentity(identity, true))
				return
			}
		}
		writeJSON(w, http.StatusOK, authMeResponse{Authenticated: false})
		return
	}

	allowed, status, _, identity := authorizeWithIdentity(s.authorizer, r, "reader")
	if status >= 500 {
		http.Error(w, "authorization lookup failed", http.StatusInternalServerError)
		return
	}
	if allowed {
		writeJSON(w, http.StatusOK, authMeFromIdentity(identity, true))
		return
	}
	writeJSON(w, http.StatusOK, authMeResponse{Authenticated: false})
}

func (s *Server) handleAuthChangePassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authRateLimiter.allow(r) {
		http.Error(w, "too many authentication attempts", http.StatusTooManyRequests)
		return
	}
	if s.authMode != AuthModeNative {
		http.Error(w, "password change is only available in native mode", http.StatusBadRequest)
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

	user, ok, err := s.authenticatedSessionUser(r, true)
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "authentication required", http.StatusUnauthorized)
		return
	}

	passwordOK, verifyErr := store.VerifyPasswordHash(user.PasswordHash, req.CurrentPassword)
	if verifyErr != nil || !passwordOK {
		http.Error(w, "invalid current password", http.StatusUnauthorized)
		return
	}

	hash, hashErr := store.GeneratePasswordHash(req.NewPassword)
	if hashErr != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	if setErr := s.stores.LocalAdmins.SetPassword(user.Username, hash); setErr != nil {
		http.Error(w, "failed to update password", http.StatusInternalServerError)
		return
	}
	if delErr := s.stores.AuthSessions.DeleteByUsername(user.Username); delErr != nil && s.logger != nil {
		s.logger.Printf("WARNING: failed to invalidate sessions after password change for %s: %v", user.Username, delErr)
	}
	s.clearSessionCookie(w, r)
	writeJSON(w, http.StatusOK, map[string]string{"status": "password changed"})
}

func (s *Server) handleAuthAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authRateLimiter.allow(r) {
		http.Error(w, "too many authentication attempts", http.StatusTooManyRequests)
		return
	}
	if s.authMode != AuthModeNative {
		http.Error(w, "password reset is only available in native mode", http.StatusBadRequest)
		return
	}
	if !s.authorizeAdminRequest(w, r) {
		return
	}

	var req authResetPasswordRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	req.Username = strings.TrimSpace(req.Username)
	if req.Username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	if err := store.ValidatePasswordPolicy(req.NewPassword, 12); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	_, hasUser, err := s.stores.LocalAdmins.GetByUsername(req.Username)
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !hasUser {
		http.Error(w, "user not found", http.StatusNotFound)
		return
	}

	hash, err := store.GeneratePasswordHash(req.NewPassword)
	if err != nil {
		http.Error(w, "failed to hash password", http.StatusInternalServerError)
		return
	}
	if err := s.stores.LocalAdmins.SetPassword(req.Username, hash); err != nil {
		if errors.Is(err, store.ErrAuthUserNotFound) {
			http.Error(w, "user not found", http.StatusNotFound)
			return
		}
		http.Error(w, "failed to reset password", http.StatusInternalServerError)
		return
	}
	currentUser, hasCurrentSession, currentSessionErr := s.authenticatedSessionUser(r, false)
	if currentSessionErr != nil && s.logger != nil {
		s.logger.Printf("WARNING: failed to resolve current session during password reset for %s: %v", req.Username, currentSessionErr)
	}
	if delErr := s.stores.AuthSessions.DeleteByUsername(req.Username); delErr != nil && s.logger != nil {
		s.logger.Printf("WARNING: failed to invalidate sessions after password reset for %s: %v", req.Username, delErr)
	}
	if hasCurrentSession && strings.EqualFold(currentUser.Username, req.Username) {
		s.clearSessionCookie(w, r)
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "password reset"})
}

func authMeFromIdentity(identity AuthIdentity, authenticated bool) authMeResponse {
	name := strings.TrimSpace(identity.Name)
	method := strings.ToLower(strings.TrimSpace(identity.Method))
	role := strings.ToLower(strings.TrimSpace(identity.Role))
	resp := authMeResponse{
		Authenticated: authenticated,
		Name:          name,
		Role:          role,
		Method:        method,
	}
	if method == "session" {
		resp.Username = name
	}
	return resp
}

func authorizeWithIdentity(authorizer RequestAuthorizer, r *http.Request, requiredRole string) (bool, int, string, AuthIdentity) {
	if authorizer == nil {
		return true, 0, "", AuthIdentity{Method: "none"}
	}
	if withIdentity, ok := authorizer.(IdentityAuthorizer); ok {
		return withIdentity.AuthorizeWithIdentity(r, requiredRole)
	}
	allowed, statusCode, message := authorizer.Authorize(r, requiredRole)
	identity := AuthIdentity{}
	if allowed {
		identity = AuthIdentity{
			Role:   strings.TrimSpace(requiredRole),
			Method: "bearer",
		}
	}
	return allowed, statusCode, message, identity
}

// handleAuthCLIToken authenticates with username+password and returns a new
// API bearer token for CLI use. This avoids the session/cookie dance and lets
// `orlojctl auth login` obtain a token in one step.
func (s *Server) handleAuthCLIToken(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !s.authRateLimiter.allow(r) {
		http.Error(w, "too many authentication attempts", http.StatusTooManyRequests)
		return
	}
	if s.authMode != AuthModeNative {
		http.Error(w, "cli-token endpoint is only available in native auth mode", http.StatusBadRequest)
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
	if strings.TrimSpace(req.Password) == "" {
		http.Error(w, "password is required", http.StatusBadRequest)
		return
	}

	user, ok, err := s.stores.LocalAdmins.GetByUsername(req.Username)
	if err != nil {
		http.Error(w, "auth store error", http.StatusInternalServerError)
		return
	}
	if !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	ok, err = store.VerifyPasswordHash(user.PasswordHash, req.Password)
	if err != nil || !ok {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	tokenName := fmt.Sprintf("cli-%s-%d", user.Username, time.Now().UnixMilli())
	rawToken, err := store.GenerateOpaqueCredential(32)
	if err != nil {
		http.Error(w, "failed to generate token", http.StatusInternalServerError)
		return
	}
	record, err := s.stores.APITokens.Create(tokenName, hashToken(rawToken), user.Role, time.Now().UTC())
	if err != nil {
		http.Error(w, "failed to create token", http.StatusInternalServerError)
		return
	}

	writeJSON(w, http.StatusCreated, map[string]string{
		"name":       record.Name,
		"role":       record.Role,
		"token":      rawToken,
		"username":   user.Username,
		"created_at": record.CreatedAt,
	})
}

func (s *Server) authorizeAdminRequest(w http.ResponseWriter, r *http.Request) bool {
	_, ok := s.authorizeAdminRequestWithIdentity(w, r)
	return ok
}

// authorizeAdminRequestWithIdentity performs admin authorization and returns
// the caller's identity for audit logging.  On failure it writes an HTTP error
// and returns (zero, false).
func (s *Server) authorizeAdminRequestWithIdentity(w http.ResponseWriter, r *http.Request) (AuthIdentity, bool) {
	allowed, status, message, identity := authorizeWithIdentity(s.authorizer, r, "admin")
	if allowed {
		return identity, true
	}
	if status <= 0 {
		status = http.StatusForbidden
	}
	http.Error(w, strings.TrimSpace(message), status)
	return AuthIdentity{}, false
}

func (s *Server) authenticatedSessionUser(r *http.Request, touch bool) (store.LocalAdminAccount, bool, error) {
	sessionID := readSessionID(r)
	if sessionID == "" {
		return store.LocalAdminAccount{}, false, nil
	}
	session, ok, err := s.stores.AuthSessions.Get(sessionID)
	if err != nil {
		return store.LocalAdminAccount{}, false, err
	}
	if !ok {
		return store.LocalAdminAccount{}, false, nil
	}
	expiresAt, parseErr := time.Parse(time.RFC3339Nano, session.ExpiresAt)
	if parseErr != nil || !expiresAt.After(time.Now().UTC()) {
		_ = s.stores.AuthSessions.Delete(sessionID)
		return store.LocalAdminAccount{}, false, nil
	}
	user, found, err := s.stores.LocalAdmins.GetByUsername(session.Username)
	if err != nil {
		return store.LocalAdminAccount{}, false, err
	}
	if !found {
		_ = s.stores.AuthSessions.Delete(sessionID)
		return store.LocalAdminAccount{}, false, nil
	}
	if touch {
		_ = s.stores.AuthSessions.Touch(sessionID, s.sessionTTL, time.Now().UTC())
	}
	return user, true, nil
}

func (s *Server) setSessionCookie(w http.ResponseWriter, r *http.Request, sessionID string, ttl time.Duration) {
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	secure := isSecureRequest(r, s.trustedProxies)
	name := sessionCookieName
	if secure {
		name = sessionCookieNameHost
	}
	cookie := &http.Cookie{
		Name:     name,
		Value:    strings.TrimSpace(sessionID),
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   secure,
		Expires:  time.Now().UTC().Add(ttl),
		MaxAge:   int(ttl.Seconds()),
	}
	http.SetCookie(w, cookie)
}

func (s *Server) clearSessionCookie(w http.ResponseWriter, r *http.Request) {
	secure := isSecureRequest(r, s.trustedProxies)
	// Clear both cookie names in case the client has either variant.
	for _, name := range []string{sessionCookieName, sessionCookieNameHost} {
		http.SetCookie(w, &http.Cookie{
			Name:     name,
			Value:    "",
			Path:     "/",
			HttpOnly: true,
			SameSite: http.SameSiteLaxMode,
			Secure:   secure,
			Expires:  time.Unix(0, 0).UTC(),
			MaxAge:   -1,
		})
	}
}

func isSecureRequest(r *http.Request, trustedProxies []*net.IPNet) bool {
	if r != nil && r.TLS != nil {
		return true
	}
	if r == nil {
		return false
	}
	if !isTrustedPeer(r, trustedProxies) {
		return false
	}
	proto := strings.TrimSpace(r.Header.Get("X-Forwarded-Proto"))
	if strings.Contains(proto, ",") {
		proto = strings.TrimSpace(strings.Split(proto, ",")[0])
	}
	return strings.EqualFold(proto, "https")
}
