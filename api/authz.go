package api

import (
	"crypto/sha256"
	"encoding/hex"
	"log"
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
	tokens  map[string]string // SHA-256(token) -> role
}

// hashToken produces a hex-encoded SHA-256 digest of a raw token. Storing
// and comparing hashes instead of raw tokens eliminates timing side-channels
// inherent in Go map lookups.
func hashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
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
		skipped := 0
		for _, pair := range pairs {
			parts := strings.SplitN(strings.TrimSpace(pair), ":", 2)
			if len(parts) != 2 {
				skipped++
				continue
			}
			token := strings.TrimSpace(parts[0])
			role := strings.ToLower(strings.TrimSpace(parts[1]))
			if token == "" {
				skipped++
				continue
			}
			if role == "" {
				role = "reader"
			}
			cfg.tokens[hashToken(token)] = role
		}
		if skipped > 0 {
			log.Printf("WARNING: ORLOJ_API_TOKENS: %d malformed token entries skipped (expected token:role pairs)", skipped)
		}
		if len(cfg.tokens) == 0 && len(pairs) > 0 {
			log.Fatalf("ORLOJ_API_TOKENS is set but all %d entries are malformed — refusing to start with auth disabled", len(pairs))
		}
	}

	if len(cfg.tokens) == 0 {
		if single := strings.TrimSpace(os.Getenv("ORLOJ_API_TOKEN")); single != "" {
			cfg.tokens[hashToken(single)] = "admin"
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
		tokens:  map[string]string{hashToken(key): "admin"},
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
	role, ok := a.cfg.tokens[hashToken(token)]
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
		// Extension point: a custom authorizer can enforce per-namespace,
		// per-resource, or per-user policies here. Nil by default.
		if s.resourceAuthorizer != nil {
			ns := requestNamespace(r)
			resType, resName := resourceInfoFromPath(r.URL.Path)
			raAllowed, raStatus, raMsg := s.resourceAuthorizer.AuthorizeResource(r, r.Method, resType, ns, resName)
			if !raAllowed {
				if raStatus <= 0 {
					raStatus = http.StatusForbidden
				}
				http.Error(w, strings.TrimSpace(raMsg), raStatus)
				return
			}
		}
		ctx := withAuthIdentity(r.Context(), AuthIdentity{
			Role:   required,
			Method: "bearer",
		})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// resourceInfoFromPath extracts the resource type and optional resource name
// from an API path. Used by the ResourceAuthorizer extension point.
func resourceInfoFromPath(path string) (resourceType, name string) {
	path = strings.TrimPrefix(path, "/v1/")
	path = strings.TrimRight(path, "/")
	parts := strings.SplitN(path, "/", 2)
	if len(parts) == 0 {
		return "", ""
	}
	resourceType = parts[0]
	if len(parts) > 1 {
		name = parts[1]
	}
	return resourceType, name
}

func requiredRoleForRequest(r *http.Request) string {
	path := strings.TrimSpace(r.URL.Path)
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if path == "/healthz" {
		return ""
	}
	if path == "/metrics" {
		return "reader"
	}
	if path == "/ui" || strings.HasPrefix(path, "/ui/") {
		return ""
	}
	if path == "/v1/auth" || strings.HasPrefix(path, "/v1/auth/") {
		return ""
	}
	if strings.HasPrefix(path, "/v1/webhook-deliveries/") {
		return "writer"
	}
	// MCP server manifests control host command execution; restrict mutations
	// to admin to prevent writer-role tokens from achieving code execution.
	if (path == "/v1/mcp-servers" || strings.HasPrefix(path, "/v1/mcp-servers/")) &&
		method != http.MethodGet && method != http.MethodHead && method != http.MethodOptions {
		return "admin"
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
