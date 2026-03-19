package api

import (
	"net/http"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

const sessionCookieName = "orloj_session"

type noAuthAuthorizer struct{}

func (noAuthAuthorizer) Authorize(_ *http.Request, _ string) (bool, int, string) {
	return true, 0, ""
}

type localModeAuthorizer struct {
	tokenAuthorizer RequestAuthorizer
	admins          *store.LocalAdminStore
	sessions        *store.AuthSessionStore
	sessionTTL      time.Duration
}

func newLocalModeAuthorizer(tokenAuthorizer RequestAuthorizer, admins *store.LocalAdminStore, sessions *store.AuthSessionStore, sessionTTL time.Duration) RequestAuthorizer {
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
	return localModeAuthorizer{
		tokenAuthorizer: tokenAuthorizer,
		admins:          admins,
		sessions:        sessions,
		sessionTTL:      sessionTTL,
	}
}

func (a localModeAuthorizer) Authorize(r *http.Request, requiredRole string) (bool, int, string) {
	if strings.TrimSpace(requiredRole) == "" {
		return true, 0, ""
	}

	hasAdmin, err := a.admins.HasAdmin()
	if err != nil {
		return false, http.StatusInternalServerError, "auth store error"
	}
	if !hasAdmin {
		return false, http.StatusUnauthorized, "admin setup required"
	}

	sessionID := readSessionID(r)
	if sessionID != "" {
		session, ok, err := a.sessions.Get(sessionID)
		if err != nil {
			return false, http.StatusInternalServerError, "session lookup failed"
		}
		if ok {
			expiresAt, err := time.Parse(time.RFC3339Nano, session.ExpiresAt)
			if err != nil || !expiresAt.After(time.Now().UTC()) {
				_ = a.sessions.Delete(sessionID)
				return false, http.StatusUnauthorized, "session expired"
			}
			_ = a.sessions.Touch(sessionID, a.sessionTTL, time.Now().UTC())
			return true, 0, ""
		}
	}

	if a.tokenAuthorizer != nil {
		if bearerToken(r.Header.Get("Authorization")) == "" {
			return false, http.StatusUnauthorized, "missing credentials"
		}
		return a.tokenAuthorizer.Authorize(r, requiredRole)
	}
	return false, http.StatusUnauthorized, "missing credentials"
}

func readSessionID(r *http.Request) string {
	if r == nil {
		return ""
	}
	cookie, err := r.Cookie(sessionCookieName)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(cookie.Value)
}
