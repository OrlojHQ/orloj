package api

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

// RequestAuthorizer evaluates API authorization for one request+required role.
type RequestAuthorizer interface {
	Authorize(r *http.Request, requiredRole string) (allowed bool, statusCode int, message string)
}

// IdentityAuthorizer is an optional extension implemented by authorizers that
// can also return the authenticated principal identity.
type IdentityAuthorizer interface {
	RequestAuthorizer
	AuthorizeWithIdentity(r *http.Request, requiredRole string) (allowed bool, statusCode int, message string, identity AuthIdentity)
}

type tokenPrincipal struct {
	Name string
	Role string
}

type authConfig struct {
	envTokens map[string]tokenPrincipal // SHA-256(token) -> principal
	store     *store.APITokenStore
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

func normalizeAPIRole(role, fallback string) (string, bool) {
	r := strings.ToLower(strings.TrimSpace(role))
	if r == "" {
		r = strings.ToLower(strings.TrimSpace(fallback))
	}
	switch r {
	case "admin", "writer", "reader", "controller":
		return r, true
	default:
		return "", false
	}
}

func parseTokenEnvConfig() map[string]tokenPrincipal {
	tokens := make(map[string]tokenPrincipal)
	rawList := strings.TrimSpace(os.Getenv("ORLOJ_API_TOKENS"))
	if rawList != "" {
		pairs := strings.Split(rawList, ",")
		skipped := 0
		for _, pair := range pairs {
			raw := strings.TrimSpace(pair)
			if raw == "" {
				skipped++
				continue
			}
			parts := strings.Split(raw, ":")
			var (
				name  string
				token string
				role  string
			)
			switch len(parts) {
			case 2:
				token = strings.TrimSpace(parts[0])
				role = strings.TrimSpace(parts[1])
			case 3:
				name = strings.TrimSpace(parts[0])
				token = strings.TrimSpace(parts[1])
				role = strings.TrimSpace(parts[2])
				if name == "" {
					skipped++
					continue
				}
			default:
				skipped++
				continue
			}
			if token == "" {
				skipped++
				continue
			}
			normalizedRole, ok := normalizeAPIRole(role, "reader")
			if !ok {
				skipped++
				continue
			}
			tokens[hashToken(token)] = tokenPrincipal{Name: name, Role: normalizedRole}
		}
		if skipped > 0 {
			log.Printf("WARNING: ORLOJ_API_TOKENS: %d malformed entries skipped (expected token:role or name:token:role)", skipped)
		}
		if len(tokens) == 0 && len(pairs) > 0 {
			log.Fatalf("ORLOJ_API_TOKENS is set but all %d entries are malformed — refusing to start with auth disabled", len(pairs))
		}
	}
	if len(tokens) == 0 {
		if single := strings.TrimSpace(os.Getenv("ORLOJ_API_TOKEN")); single != "" {
			tokens[hashToken(single)] = tokenPrincipal{Role: "admin"}
		}
	}
	return tokens
}

func loadAuthConfig(tokenStore *store.APITokenStore) authConfig {
	return authConfig{
		envTokens: parseTokenEnvConfig(),
		store:     tokenStore,
	}
}

func newTokenAuthorizerFromEnv() RequestAuthorizer {
	return newTokenAuthorizerWithStoreFromEnv(nil)
}

func newTokenAuthorizerWithStoreFromEnv(tokenStore *store.APITokenStore) RequestAuthorizer {
	return tokenAuthorizer{cfg: loadAuthConfig(tokenStore)}
}

func (a tokenAuthorizer) authEnabled() (bool, int, string) {
	if len(a.cfg.envTokens) > 0 {
		return true, 0, ""
	}
	if a.cfg.store == nil {
		return false, 0, ""
	}
	hasAny, err := a.cfg.store.HasAny()
	if err != nil {
		return false, http.StatusInternalServerError, "auth store error"
	}
	return hasAny, 0, ""
}

func (a tokenAuthorizer) resolveTokenPrincipal(token string) (tokenPrincipal, bool, int, string) {
	hashed := hashToken(token)
	if principal, ok := a.cfg.envTokens[hashed]; ok {
		return principal, true, 0, ""
	}
	if a.cfg.store == nil {
		return tokenPrincipal{}, false, 0, ""
	}
	record, ok, err := a.cfg.store.GetByHash(hashed)
	if err != nil {
		return tokenPrincipal{}, false, http.StatusInternalServerError, "auth store error"
	}
	if !ok {
		return tokenPrincipal{}, false, 0, ""
	}
	role, valid := normalizeAPIRole(record.Role, "")
	if !valid {
		return tokenPrincipal{}, false, http.StatusInternalServerError, "auth store role invalid"
	}
	return tokenPrincipal{Name: strings.TrimSpace(record.Name), Role: role}, true, 0, ""
}

// NewAPIKeyAuthorizer returns an authorizer that validates a single API key
// as an admin bearer token. When key is empty, auth is disabled (all requests
// pass). This is intended for the --api-key CLI flag.
func NewAPIKeyAuthorizer(key string) RequestAuthorizer {
	key = strings.TrimSpace(key)
	if key == "" {
		return tokenAuthorizer{cfg: authConfig{envTokens: map[string]tokenPrincipal{}}}
	}
	return tokenAuthorizer{cfg: authConfig{
		envTokens: map[string]tokenPrincipal{hashToken(key): {Role: "admin"}},
	}}
}

func (a tokenAuthorizer) Authorize(r *http.Request, requiredRole string) (bool, int, string) {
	allowed, statusCode, message, _ := a.AuthorizeWithIdentity(r, requiredRole)
	return allowed, statusCode, message
}

func (a tokenAuthorizer) AuthorizeWithIdentity(r *http.Request, requiredRole string) (bool, int, string, AuthIdentity) {
	if strings.TrimSpace(requiredRole) == "" {
		return true, 0, "", AuthIdentity{Method: "none"}
	}
	enabled, statusCode, message := a.authEnabled()
	if statusCode > 0 {
		return false, statusCode, message, AuthIdentity{}
	}
	if !enabled {
		return true, 0, "", AuthIdentity{Method: "none"}
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return false, http.StatusUnauthorized, "missing bearer token", AuthIdentity{}
	}
	principal, ok, pStatus, pMessage := a.resolveTokenPrincipal(token)
	if pStatus > 0 {
		return false, pStatus, pMessage, AuthIdentity{}
	}
	if !ok {
		return false, http.StatusUnauthorized, "invalid token", AuthIdentity{}
	}
	if !roleAllows(principal.Role, requiredRole) {
		return false, http.StatusForbidden, "forbidden", AuthIdentity{}
	}
	return true, 0, "", AuthIdentity{
		Name:   principal.Name,
		Role:   principal.Role,
		Method: "bearer",
	}
}

type statusCaptureResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

func (w *statusCaptureResponseWriter) WriteHeader(code int) {
	w.statusCode = code
	w.ResponseWriter.WriteHeader(code)
}

func (w *statusCaptureResponseWriter) Write(b []byte) (int, error) {
	if w.statusCode == 0 {
		w.statusCode = http.StatusOK
	}
	return w.ResponseWriter.Write(b)
}

func (w *statusCaptureResponseWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) withAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		required := requiredRoleForRequest(r, s.uiBasePath)
		if required == "" {
			next.ServeHTTP(w, r)
			return
		}
		authorizer := s.authorizer
		if authorizer == nil {
			authorizer = newTokenAuthorizerWithStoreFromEnv(s.stores.APITokens)
		}

		var (
			allowed    bool
			statusCode int
			message    string
			identity   AuthIdentity
		)
		if withIdentity, ok := authorizer.(IdentityAuthorizer); ok {
			allowed, statusCode, message, identity = withIdentity.AuthorizeWithIdentity(r, required)
		} else {
			allowed, statusCode, message = authorizer.Authorize(r, required)
		}
		if !allowed {
			if statusCode <= 0 {
				statusCode = http.StatusForbidden
			}
			http.Error(w, strings.TrimSpace(message), statusCode)
			return
		}
		if strings.TrimSpace(identity.Role) == "" {
			identity.Role = strings.TrimSpace(required)
		}
		if strings.TrimSpace(identity.Method) == "" {
			identity.Method = "bearer"
		}

		ctx := withAuthIdentity(r.Context(), identity)
		reqWithIdentity := r.WithContext(ctx)
		// Extension point: a custom authorizer can enforce per-namespace,
		// per-resource, or per-user policies here. Nil by default.
		if s.resourceAuthorizer != nil {
			ns := requestNamespace(reqWithIdentity)
			resType, resName := resourceInfoFromPath(reqWithIdentity.URL.Path)
			raAllowed, raStatus, raMsg := s.resourceAuthorizer.AuthorizeResource(reqWithIdentity, reqWithIdentity.Method, resType, ns, resName)
			if !raAllowed {
				if raStatus <= 0 {
					raStatus = http.StatusForbidden
				}
				http.Error(w, strings.TrimSpace(raMsg), raStatus)
				return
			}
		}

		rw := &statusCaptureResponseWriter{ResponseWriter: w}
		next.ServeHTTP(rw, reqWithIdentity)
		if rw.statusCode == 0 {
			rw.statusCode = http.StatusOK
		}
		s.emitBearerRequestAudit(reqWithIdentity, rw.statusCode, identity)
	})
}

func (s *Server) emitBearerRequestAudit(r *http.Request, statusCode int, identity AuthIdentity) {
	if s == nil {
		return
	}
	if !strings.EqualFold(strings.TrimSpace(identity.Method), "bearer") {
		return
	}
	if !strings.HasPrefix(strings.TrimSpace(r.URL.Path), "/v1/") {
		return
	}

	principal := strings.TrimSpace(identity.Name)
	if principal == "" {
		role := strings.TrimSpace(identity.Role)
		if role == "" {
			role = "unknown"
		}
		principal = "bearer:" + role
	}
	outcome := "success"
	if statusCode >= 400 {
		outcome = "error"
	}
	resType, resName := resourceInfoFromPath(r.URL.Path)
	namespace := ""
	if !strings.HasPrefix(strings.TrimSpace(r.URL.Path), "/v1/auth") && !strings.HasPrefix(strings.TrimSpace(r.URL.Path), "/v1/tokens") {
		namespace = requestNamespace(r)
	}
	s.extensions.Audit.RecordAudit(r.Context(), agentruntime.AuditEvent{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Component:    "apiserver",
		Action:       "api.request",
		Outcome:      outcome,
		Namespace:    namespace,
		ResourceKind: resType,
		ResourceName: resName,
		Principal:    principal,
		Message:      fmt.Sprintf("%s %s", strings.ToUpper(strings.TrimSpace(r.Method)), strings.TrimSpace(r.URL.Path)),
		Metadata: map[string]string{
			"method": strings.ToUpper(strings.TrimSpace(r.Method)),
			"path":   strings.TrimSpace(r.URL.Path),
			"status": strconv.Itoa(statusCode),
			"role":   strings.TrimSpace(identity.Role),
		},
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

func requiredRoleForRequest(r *http.Request, uiBasePath string) string {
	path := strings.TrimSpace(r.URL.Path)
	method := strings.ToUpper(strings.TrimSpace(r.Method))
	if path == "/healthz" {
		return ""
	}
	if path == "/metrics" {
		return "reader"
	}
	if isUIPath(path, uiBasePath) {
		return ""
	}
	if path == "/v1/auth" || strings.HasPrefix(path, "/v1/auth/") {
		return ""
	}
	if path == "/v1/tokens" || strings.HasPrefix(path, "/v1/tokens/") {
		return "admin"
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

// roleAllows checks whether the actual role satisfies the required role.
//
// Role hierarchy (highest to lowest):
//   - admin:      full access to all endpoints
//   - writer:     read + write (satisfies both "reader" and "writer" requirements)
//   - controller: read + status-patch (satisfies "reader" and "controller" requirements)
//   - reader:     read-only (satisfies only "reader" requirements)
//
// Writer and controller are orthogonal write-path capabilities; neither
// implies the other.  Both include implicit read access.
func roleAllows(actual, required string) bool {
	actual = strings.ToLower(strings.TrimSpace(actual))
	required = strings.ToLower(strings.TrimSpace(required))
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

// isUIPath returns true when the request path falls under the configured
// web console base path (e.g. "/" or "/console/").
// When the console is at "/", every path outside the /v1 API prefix is a
// console path (/healthz and /metrics are checked before this is called).
func isUIPath(reqPath, uiBasePath string) bool {
	if uiBasePath == "/" {
		return !strings.HasPrefix(reqPath, "/v1")
	}
	base := strings.TrimSuffix(uiBasePath, "/")
	return reqPath == base || strings.HasPrefix(reqPath, uiBasePath)
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
