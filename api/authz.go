package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

// maxToolMutationProbeBytes caps how much of a /v1/tools mutation body we read
// when deciding whether the request requires admin. Real tool specs are well
// under this size; anything larger is rejected as suspicious.
const maxToolMutationProbeBytes = 1 << 20 // 1 MiB

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
	Name            string
	Role            string
	A2AAgentSystems []string
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
	case "admin", "writer", "reader", "controller", "a2a":
		return r, true
	default:
		return "", false
	}
}

func normalizeA2AAgentSystemRefs(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		v := strings.TrimSpace(value)
		if v == "" {
			continue
		}
		if strings.Contains(v, "/") {
			parts := strings.SplitN(v, "/", 2)
			v = store.ScopedName(parts[0], parts[1])
		} else {
			v = store.ScopedName(resources.DefaultNamespace, v)
		}
		key := strings.ToLower(v)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

func parseA2AAgentSystemRefs(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	fields := strings.FieldsFunc(raw, func(r rune) bool {
		return r == '|' || r == ';'
	})
	return normalizeA2AAgentSystemRefs(fields)
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
				name            string
				token           string
				role            string
				a2aAgentSystems []string
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
			case 4:
				name = strings.TrimSpace(parts[0])
				token = strings.TrimSpace(parts[1])
				role = strings.TrimSpace(parts[2])
				a2aAgentSystems = parseA2AAgentSystemRefs(parts[3])
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
			if normalizedRole == "a2a" && len(a2aAgentSystems) == 0 {
				skipped++
				continue
			}
			tokens[hashToken(token)] = tokenPrincipal{Name: name, Role: normalizedRole, A2AAgentSystems: a2aAgentSystems}
		}
		if skipped > 0 {
			log.Printf("WARNING: ORLOJ_API_TOKENS: %d malformed entries skipped (expected token:role, name:token:role, or name:token:a2a:namespace/name|other)", skipped)
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
	return tokenPrincipal{
		Name:            strings.TrimSpace(record.Name),
		Role:            role,
		A2AAgentSystems: normalizeA2AAgentSystemRefs(record.A2AAgentSystems),
	}, true, 0, ""
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

// TryResolveIdentity attempts to extract and validate a bearer token from the
// request without rejecting missing tokens. Returns the resolved identity
// (Method:"bearer") when a valid token is present, a "none" identity when no
// token is sent or auth is globally disabled, and an error status when a token
// is present but invalid. Used by A2A invoke/registry paths where the handler
// decides per-system policy.
func (a tokenAuthorizer) TryResolveIdentity(r *http.Request) (AuthIdentity, int, string) {
	enabled, statusCode, message := a.authEnabled()
	if statusCode > 0 {
		return AuthIdentity{}, statusCode, message
	}
	if !enabled {
		return AuthIdentity{Method: "none", AuthDisabled: true}, 0, ""
	}
	token := bearerToken(r.Header.Get("Authorization"))
	if token == "" {
		return AuthIdentity{Method: "none"}, 0, ""
	}
	principal, ok, pStatus, pMessage := a.resolveTokenPrincipal(token)
	if pStatus > 0 {
		return AuthIdentity{}, pStatus, pMessage
	}
	if !ok {
		return AuthIdentity{}, http.StatusUnauthorized, "invalid token"
	}
	return AuthIdentity{
		Name:            principal.Name,
		Role:            principal.Role,
		Method:          "bearer",
		A2AAgentSystems: principal.A2AAgentSystems,
	}, 0, ""
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
		Name:            principal.Name,
		Role:            principal.Role,
		Method:          "bearer",
		A2AAgentSystems: principal.A2AAgentSystems,
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

		// A2A invoke and registry paths use optional auth: resolve identity
		// if a token is present, but don't reject missing tokens. The handler
		// enforces per-system policy (public vs bearer).
		if required == "a2a_invoke" {
			authorizer := s.authorizer
			if authorizer == nil {
				authorizer = newTokenAuthorizerWithStoreFromEnv(s.stores.APITokens)
			}
			if ta, ok := authorizer.(tokenAuthorizer); ok {
				identity, statusCode, message := ta.TryResolveIdentity(r)
				if statusCode > 0 {
					http.Error(w, strings.TrimSpace(message), statusCode)
					return
				}
				if identity.Method != "bearer" && identity.Method != "none" {
					http.Error(w, "A2A requires bearer token authentication", http.StatusUnauthorized)
					return
				}
				ctx := withAuthIdentity(r.Context(), identity)
				rw := &statusCaptureResponseWriter{ResponseWriter: w}
				next.ServeHTTP(rw, r.WithContext(ctx))
				if rw.statusCode == 0 {
					rw.statusCode = http.StatusOK
				}
				s.emitBearerRequestAudit(r.WithContext(ctx), rw.statusCode, identity)
				return
			}
			// Fallback for non-tokenAuthorizer: use standard "a2a" role check.
			required = "a2a"
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
		if isA2AInvokeRequest(r.URL.Path, r.Method) && !strings.EqualFold(identity.Method, "bearer") && !strings.EqualFold(identity.Method, "none") {
			http.Error(w, "A2A requires bearer token authentication", http.StatusUnauthorized)
			return
		}

		ctx := withAuthIdentity(r.Context(), identity)
		reqWithIdentity := r.WithContext(ctx)
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
	if isA2ACardPath(path, method) {
		return ""
	}
	if isA2ARegistryPath(path, method) || isA2AInvokeRequest(path, method) {
		return "a2a_invoke"
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
		return ""
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
	// Tool specs of type "cli" with isolation_mode "none" execute commands
	// directly on the worker host. Treat that combination as privileged and
	// require admin, mirroring the existing /v1/mcp-servers gate above. Other
	// tool types (http, grpc, container/wasm CLI, etc.) remain writer-creatable.
	if (path == "/v1/tools" || strings.HasPrefix(path, "/v1/tools/")) &&
		!strings.HasSuffix(path, "/status") &&
		(method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch) {
		if toolMutationRequiresAdmin(r) {
			return "admin"
		}
	}
	return "writer"
}

func isA2ARegistryPath(path, method string) bool {
	return strings.EqualFold(method, http.MethodGet) && strings.TrimSpace(path) == "/v1/a2a/agents"
}

func isA2ACardPath(path, method string) bool {
	if !strings.EqualFold(method, http.MethodGet) && !strings.EqualFold(method, http.MethodHead) {
		return false
	}
	path = strings.TrimSpace(path)
	if path == "/.well-known/agent-card.json" || path == "/.well-known/agent.json" {
		return true
	}
	if strings.HasPrefix(path, "/v1/agents/") || strings.HasPrefix(path, "/v1/agent-systems/") {
		return strings.Contains(path, "/.well-known/agent-card.json") || strings.HasSuffix(path, "/.well-known/agent.json")
	}
	return false
}

func isA2AInvokeRequest(path, method string) bool {
	if !strings.EqualFold(method, http.MethodPost) {
		return false
	}
	path = strings.TrimSpace(path)
	if path == "/a2a" {
		return true
	}
	if strings.HasPrefix(path, "/v1/agents/") || strings.HasPrefix(path, "/v1/agent-systems/") {
		return strings.HasSuffix(path, "/a2a")
	}
	return false
}

// toolMutationRequiresAdmin inspects an in-flight /v1/tools mutation body and
// reports true when the spec requests host CLI execution
// (spec.type == "cli" && spec.runtime.isolation_mode == "none").
//
// The body is read into memory, then re-attached to the request so downstream
// handlers can decode it normally. The probe runs the same parser the handler
// will run (resources.ParseToolManifest) so JSON and YAML manifests are
// classified identically.
//
// Failure modes:
//   - Body read error: fail closed (return true). The downstream handler will
//     also error, but we never let an unreadable body sneak past.
//   - Body exceeds the size cap: fail closed.
//   - Manifest parse error: fall through to writer (return false). The handler
//     will reject the request with 400, so writers can't exploit this to write
//     a malformed cli+none object.
func toolMutationRequiresAdmin(r *http.Request) bool {
	if r == nil || r.Body == nil {
		return false
	}
	limited := io.LimitReader(r.Body, maxToolMutationProbeBytes+1)
	body, err := io.ReadAll(limited)
	if err != nil {
		// Restore an empty body so the downstream handler can produce its own
		// error rather than panicking, but still fail closed on the auth check.
		r.Body = io.NopCloser(bytes.NewReader(nil))
		return true
	}
	// Re-attach the body for the actual handler.
	r.Body = io.NopCloser(bytes.NewReader(body))
	if int64(len(body)) > maxToolMutationProbeBytes {
		return true
	}
	if len(bytes.TrimSpace(body)) == 0 {
		// Empty body — let the regular handler reject it; no privilege upgrade.
		return false
	}
	// Use the same manifest parser as the handler so JSON and YAML inputs are
	// classified identically. Note that ParseToolManifest also runs Normalize,
	// which defaults isolation_mode for cli to "container" when omitted; that
	// means a cli manifest without an explicit isolation_mode will not be
	// classified as host execution here, matching the runtime's behavior.
	tool, err := resources.ParseToolManifest(body)
	if err != nil {
		// Malformed manifest: let the handler surface a 400. Writer-only.
		return false
	}
	toolType := strings.ToLower(strings.TrimSpace(tool.Spec.Type))
	mode := strings.ToLower(strings.TrimSpace(tool.Spec.Runtime.IsolationMode))
	return toolType == "cli" && mode == "none"
}

// roleAllows checks whether the actual role satisfies the required role.
//
// Role hierarchy (highest to lowest):
//   - admin:      full access to all endpoints
//   - writer:     read + write + A2A invoke
//   - a2a:        A2A invoke only
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
	case "a2a":
		return actual == "a2a" || actual == "writer"
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
