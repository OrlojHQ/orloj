package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

const (
	sessionCookieName     = "orloj_session"
	sessionCookieNameHost = "__Host-orloj_session"
)

type noAuthAuthorizer struct{}

func (noAuthAuthorizer) Authorize(_ *http.Request, _ string) (bool, int, string) {
	return true, 0, ""
}

func (noAuthAuthorizer) AuthorizeWithIdentity(_ *http.Request, _ string) (bool, int, string, AuthIdentity) {
	return true, 0, "", AuthIdentity{Method: "none"}
}

type nativeModeAuthorizer struct {
	tokenAuthorizer RequestAuthorizer
	admins          *store.LocalAdminStore
	sessions        *store.AuthSessionStore
	sessionTTL      time.Duration
}

func newNativeModeAuthorizer(tokenAuthorizer RequestAuthorizer, admins *store.LocalAdminStore, sessions *store.AuthSessionStore, sessionTTL time.Duration) RequestAuthorizer {
	if tokenAuthorizer == nil {
		tokenAuthorizer = newTokenAuthorizerFromEnv()
	}
	if admins == nil {
		admins = store.NewLocalAdminStore()
	}
	if sessions == nil {
		sessions = store.NewAuthSessionStore()
	}
	if sessionTTL <= 0 {
		sessionTTL = 24 * time.Hour
	}
	return nativeModeAuthorizer{
		tokenAuthorizer: tokenAuthorizer,
		admins:          admins,
		sessions:        sessions,
		sessionTTL:      sessionTTL,
	}
}

func (a nativeModeAuthorizer) Authorize(r *http.Request, requiredRole string) (bool, int, string) {
	allowed, statusCode, message, _ := a.AuthorizeWithIdentity(r, requiredRole)
	return allowed, statusCode, message
}

func (a nativeModeAuthorizer) AuthorizeWithIdentity(r *http.Request, requiredRole string) (bool, int, string, AuthIdentity) {
	if strings.TrimSpace(requiredRole) == "" {
		return true, 0, "", AuthIdentity{Method: "none"}
	}

	hasAdmin, err := a.admins.HasAdmin()
	if err != nil {
		return false, http.StatusInternalServerError, "auth store error", AuthIdentity{}
	}
	if !hasAdmin {
		return false, http.StatusUnauthorized, "admin setup required", AuthIdentity{}
	}

	sessionID := readSessionID(r)
	if sessionID != "" {
		session, ok, err := a.sessions.Get(sessionID)
		if err != nil {
			return false, http.StatusInternalServerError, "session lookup failed", AuthIdentity{}
		}
		if ok {
			expiresAt, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
			if err != nil || !expiresAt.After(time.Now().UTC()) {
				_ = a.sessions.Delete(sessionID)
				return false, http.StatusUnauthorized, "session expired", AuthIdentity{}
			}
			user, found, err := a.admins.GetByUsername(session.Username)
			if err != nil {
				return false, http.StatusInternalServerError, "auth store error", AuthIdentity{}
			}
			if !found {
				_ = a.sessions.Delete(sessionID)
				return false, http.StatusUnauthorized, "session user not found", AuthIdentity{}
			}
			if !roleAllows(user.Role, requiredRole) {
				return false, http.StatusForbidden, "forbidden", AuthIdentity{}
			}
			_ = a.sessions.Touch(sessionID, a.sessionTTL, time.Now().UTC())
			return true, 0, "", AuthIdentity{
				Name:   user.Username,
				Role:   strings.ToLower(strings.TrimSpace(user.Role)),
				Method: "session",
			}
		}
	}

	if a.tokenAuthorizer != nil {
		if bearerToken(r.Header.Get("Authorization")) == "" {
			return false, http.StatusUnauthorized, "missing credentials", AuthIdentity{}
		}
		if withIdentity, ok := a.tokenAuthorizer.(IdentityAuthorizer); ok {
			return withIdentity.AuthorizeWithIdentity(r, requiredRole)
		}
		allowed, status, message := a.tokenAuthorizer.Authorize(r, requiredRole)
		return allowed, status, message, AuthIdentity{Method: "bearer", Role: strings.TrimSpace(requiredRole)}
	}
	return false, http.StatusUnauthorized, "missing credentials", AuthIdentity{}
}

func readSessionID(r *http.Request) string {
	if r == nil {
		return ""
	}
	// Check the __Host- prefixed cookie first (HTTPS), then fall back to the
	// unprefixed variant (HTTP / development).
	for _, name := range []string{sessionCookieNameHost, sessionCookieName} {
		cookie, err := r.Cookie(name)
		if err == nil && strings.TrimSpace(cookie.Value) != "" {
			return strings.TrimSpace(cookie.Value)
		}
	}
	return ""
}
