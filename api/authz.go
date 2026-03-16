package api

import (
	"net/http"
	"os"
	"strings"
)

// RequestAuthorizer evaluates API authorization for one request+required role.
type RequestAuthorizer interface {
	Authorize(r *http.Request, requiredRole string) (allowed bool, statusCode int, message string)
}

type authConfig struct {
	enabled bool
	tokens  map[string]string
}

type tokenAuthorizer struct {
	cfg authConfig
}

func loadAuthConfig() authConfig {
	cfg := authConfig{
		enabled: false,
		tokens:  make(map[string]string),
	}

	rawList := strings.TrimSpace(os.Getenv("ORLOJ_API_TOKENS"))
	if rawList != "" {
		pairs := strings.Split(rawList, ",")
		for _, pair := range pairs {
			parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(parts) != 2 {
				continue
			}
			token := strings.TrimSpace(parts[0])
			role := strings.ToLower(strings.TrimSpace(parts[1]))
			if token == "" {
				continue
			}
			if role == "" {
				role = "reader"
			}
			cfg.tokens[token] = role
		}
	}

	if len(cfg.tokens) == 0 {
		if single := strings.TrimSpace(os.Getenv("ORLOJ_API_TOKEN")); single != "" {
			cfg.tokens[single] = "admin"
		}
	}

	cfg.enabled = len(cfg.tokens) > 0
	return cfg
}

func newTokenAuthorizerFromEnv() RequestAuthorizer {
	return tokenAuthorizer{cfg: loadAuthConfig()}
}

// NewAPIKeyAuthorizer returns an authorizer that validates a single API key
// as an admin bearer token. When key is empty, auth is disabled (all requests
// pass). This is intended for the --api-key CLI flag.
func NewAPIKeyAuthorizer(key string) RequestAuthorizer {
	key = strings.TrimSpace(key)
	if key == "" {
		return tokenAuthorizer{cfg: authConfig{enabled: false}}
	}
	return tokenAuthorizer{cfg: authConfig{
		enabled: true,
		tokens:  map[string]string{key: "admin"},
	}}
}

func (a tokenAuthorizer) Authorize(r *http.Request, requiredRole string) (bool, int, string) {
	if strings.TrimSpace(requiredRole) == "" {
		return true, 0, ""
	}
	if !a.cfg.enabled {
		return true, 0, ""
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return false, http.StatusUnauthorized, "missing bearer token"
	}
	role, ok := a.cfg.tokens[token]
	if !ok {
		return false, http.StatusUnauthorized, "invalid token"
	}
	if !roleAllows(strings.ToLower(strings.TrimSpace(role)), requiredRole) {
		return false, http.StatusForbidden, "forbidden"
	}
	return true, 0, ""
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && strings.HasPrefix(strings.TrimSpace(r.URL.Path), "/v1/webhook-deliveries/") {
			next.ServeHTTP(w, r)
			return
		}
		required := requiredRoleForRequest(r)
		if required == "" {
			next.ServeHTTP(w, r)
			return
		}
		authorizer := s.authorizer
		if authorizer == nil {
			authorizer = newTokenAuthorizerFromEnv()
		}
		allowed, statusCode, message := authorizer.Authorize(r, required)
		if !allowed {
			if statusCode <= 0 {
				statusCode = http.StatusForbidden
			}
			http.Error(w, strings.TrimSpace(message), statusCode)
			return
		}
		next.ServeHTTP(w, r)
	})
}

func requiredRoleForRequest(r *http.Request) string {
	path := strings.TrimSpace(r.URL.Path)
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if path == "/healthz" {
		return ""
	}
	if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
		return "reader"
	}
	if strings.HasSuffix(path, "/status") {
		return "controller"
	}
	return "writer"
}

func roleAllows(actual, required string) bool {
	if actual == "admin" {
		return true
	}
	switch required {
	case "reader":
		return actual == "reader" || actual == "writer" || actual == "controller"
	case "writer":
		return actual == "writer"
	case "controller":
		return actual == "controller"
	default:
		return false
	}
}

func bearerToken(authz string) string {
	authz = strings.TrimSpace(authz)
	if authz == "" {
		return ""
	}
	parts := strings.SplitN(authz, " ", 2)
	if len(parts) != 2 {
		return ""
	}
	if !strings.EqualFold(strings.TrimSpace(parts[0]), "Bearer") {
		return ""
	}
	return strings.TrimSpace(parts[1])
}
