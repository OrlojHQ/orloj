package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type createTokenRequest struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	A2AAgentSystems []string `json:"a2a_agent_systems,omitempty"`
}

type tokenMetadataResponse struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	CreatedAt       string   `json:"created_at"`
	A2AAgentSystems []string `json:"a2a_agent_systems,omitempty"`
}

type tokenCreateResponse struct {
	Name            string   `json:"name"`
	Role            string   `json:"role"`
	CreatedAt       string   `json:"created_at"`
	Token           string   `json:"token"`
	A2AAgentSystems []string `json:"a2a_agent_systems,omitempty"`
}

type tokenListResponse struct {
	Items []tokenMetadataResponse `json:"items"`
}

type createAuthUserRequest struct {
	Username string `json:"username"`
	Role     string `json:"role"`
}

type authUserResponse struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type authUserCreateResponse struct {
	Username  string `json:"username"`
	Role      string `json:"role"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
	Password  string `json:"password"`
}

type authUserListResponse struct {
	Items []authUserResponse `json:"items"`
}

func (s *Server) handleTokens(w http.ResponseWriter, r *http.Request) {
	switch r.Method {
	case http.MethodGet:
		records, err := s.stores.APITokens.List()
		if err != nil {
			http.Error(w, "token store error", http.StatusInternalServerError)
			return
		}
		resp := tokenListResponse{Items: make([]tokenMetadataResponse, 0, len(records))}
		for _, record := range records {
			resp.Items = append(resp.Items, tokenMetadataResponse{
				Name:            record.Name,
				Role:            record.Role,
				CreatedAt:       record.CreatedAt,
				A2AAgentSystems: record.A2AAgentSystems,
			})
		}
		writeJSON(w, http.StatusOK, resp)
	case http.MethodPost:
		// Re-extract identity so audit is explicit, not dependent on middleware ordering.
		identity, _ := AuthIdentityFromRequest(r)
		auditReq := r.WithContext(withAuthIdentity(r.Context(), identity))

		var req createTokenRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			http.Error(w, "name is required", http.StatusBadRequest)
			return
		}
		rawToken, err := store.GenerateOpaqueCredential(32)
		if err != nil {
			http.Error(w, "failed to generate token", http.StatusInternalServerError)
			return
		}
		a2aAgentSystems := normalizeA2AAgentSystemRefs(req.A2AAgentSystems)
		record, err := s.stores.APITokens.CreateWithA2AAgentSystems(req.Name, hashToken(rawToken), req.Role, a2aAgentSystems, time.Now().UTC())
		if err != nil {
			switch {
			case errors.Is(err, store.ErrAPITokenExists):
				http.Error(w, "token already exists", http.StatusConflict)
			case errors.Is(err, store.ErrInvalidAuthRole):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "failed to create token", http.StatusInternalServerError)
			}
			return
		}
		s.emitAdminAudit(auditReq, "token.create", "api-token", record.Name, fmt.Sprintf("created API token %q with role %s", record.Name, record.Role))
		writeJSON(w, http.StatusCreated, tokenCreateResponse{
			Name:            record.Name,
			Role:            record.Role,
			CreatedAt:       record.CreatedAt,
			Token:           rawToken,
			A2AAgentSystems: record.A2AAgentSystems,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleTokenByName(w http.ResponseWriter, r *http.Request) {
	name := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/tokens/"), "/")
	if name == "" {
		http.Error(w, "token name is required", http.StatusBadRequest)
		return
	}
	decoded, err := url.PathUnescape(name)
	if err != nil {
		http.Error(w, "invalid token name encoding", http.StatusBadRequest)
		return
	}
	name = strings.TrimSpace(decoded)
	if name == "" || strings.Contains(name, "/") {
		http.Error(w, "invalid token name", http.StatusBadRequest)
		return
	}

	// Re-extract identity so audit is explicit, not dependent on middleware ordering.
	identity, _ := AuthIdentityFromRequest(r)
	auditReq := r.WithContext(withAuthIdentity(r.Context(), identity))

	switch r.Method {
	case http.MethodDelete:
		err := s.stores.APITokens.Delete(name)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrAPITokenNotFound):
				http.Error(w, "token not found", http.StatusNotFound)
			default:
				http.Error(w, "failed to delete token", http.StatusInternalServerError)
			}
			return
		}
		s.emitAdminAudit(auditReq, "token.delete", "api-token", name, fmt.Sprintf("deleted API token %q", name))
		writeJSON(w, http.StatusOK, map[string]string{"status": "token deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAuthUsers(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthModeNative {
		http.Error(w, "user management is only available in native mode", http.StatusBadRequest)
		return
	}
	identity, ok := s.authorizeAdminRequestWithIdentity(w, r)
	if !ok {
		return
	}
	auditReq := r.WithContext(withAuthIdentity(r.Context(), identity))

	switch r.Method {
	case http.MethodGet:
		users, err := s.stores.LocalAdmins.ListUsers()
		if err != nil {
			http.Error(w, "auth store error", http.StatusInternalServerError)
			return
		}
		items := make([]authUserResponse, 0, len(users))
		for _, user := range users {
			items = append(items, authUserResponse{
				Username:  user.Username,
				Role:      user.Role,
				CreatedAt: user.CreatedAt,
				UpdatedAt: user.UpdatedAt,
			})
		}
		writeJSON(w, http.StatusOK, authUserListResponse{Items: items})
	case http.MethodPost:
		var req createAuthUserRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "invalid request body", http.StatusBadRequest)
			return
		}
		req.Username = strings.TrimSpace(req.Username)
		if req.Username == "" {
			http.Error(w, "username is required", http.StatusBadRequest)
			return
		}
		password, err := store.GenerateOpaqueCredential(24)
		if err != nil {
			http.Error(w, "failed to generate password", http.StatusInternalServerError)
			return
		}
		hash, err := store.GeneratePasswordHash(password)
		if err != nil {
			http.Error(w, "failed to hash password", http.StatusInternalServerError)
			return
		}
		user, err := s.stores.LocalAdmins.CreateUser(req.Username, hash, req.Role)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrAuthUserExists):
				http.Error(w, "user already exists", http.StatusConflict)
			case errors.Is(err, store.ErrInvalidAuthRole):
				http.Error(w, err.Error(), http.StatusBadRequest)
			default:
				http.Error(w, "failed to create user", http.StatusInternalServerError)
			}
			return
		}
		s.emitAdminAudit(auditReq, "user.create", "auth-user", user.Username, fmt.Sprintf("created user %q with role %s", user.Username, user.Role))
		writeJSON(w, http.StatusCreated, authUserCreateResponse{
			Username:  user.Username,
			Role:      user.Role,
			CreatedAt: user.CreatedAt,
			UpdatedAt: user.UpdatedAt,
			Password:  password,
		})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (s *Server) handleAuthUserByName(w http.ResponseWriter, r *http.Request) {
	if s.authMode != AuthModeNative {
		http.Error(w, "user management is only available in native mode", http.StatusBadRequest)
		return
	}
	identity, ok := s.authorizeAdminRequestWithIdentity(w, r)
	if !ok {
		return
	}
	auditReq := r.WithContext(withAuthIdentity(r.Context(), identity))

	username := strings.Trim(strings.TrimPrefix(r.URL.Path, "/v1/auth/users/"), "/")
	if username == "" {
		http.Error(w, "username is required", http.StatusBadRequest)
		return
	}
	decoded, err := url.PathUnescape(username)
	if err != nil {
		http.Error(w, "invalid username encoding", http.StatusBadRequest)
		return
	}
	username = strings.TrimSpace(decoded)
	if username == "" || strings.Contains(username, "/") {
		http.Error(w, "invalid username", http.StatusBadRequest)
		return
	}

	switch r.Method {
	case http.MethodDelete:
		err := s.stores.LocalAdmins.DeleteUser(username)
		if err != nil {
			switch {
			case errors.Is(err, store.ErrAuthUserNotFound):
				http.Error(w, "user not found", http.StatusNotFound)
			case errors.Is(err, store.ErrAuthLastAdmin):
				http.Error(w, err.Error(), http.StatusConflict)
			default:
				http.Error(w, "failed to delete user", http.StatusInternalServerError)
			}
			return
		}
		if delErr := s.stores.AuthSessions.DeleteByUsername(username); delErr != nil && s.logger != nil {
			s.logger.Printf("WARNING: failed to invalidate sessions after deleting user %s: %v", username, delErr)
		}
		s.emitAdminAudit(auditReq, "user.delete", "auth-user", username, fmt.Sprintf("deleted user %q", username))
		writeJSON(w, http.StatusOK, map[string]string{"status": "user deleted"})
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

// emitAdminAudit records an audit event for admin-only auth operations
// (token and user CRUD).  The caller's identity is extracted from the
// request context (set by the authorization middleware or session layer).
func (s *Server) emitAdminAudit(r *http.Request, action, resourceKind, resourceName, message string) {
	identity, _ := AuthIdentityFromRequest(r)
	principal := strings.TrimSpace(identity.Name)
	if principal == "" {
		principal = "unknown"
	}
	s.extensions.Audit.RecordAudit(r.Context(), agentruntime.AuditEvent{
		Timestamp:    time.Now().UTC().Format(time.RFC3339Nano),
		Component:    "apiserver",
		Action:       action,
		Outcome:      "success",
		ResourceKind: resourceKind,
		ResourceName: resourceName,
		Principal:    principal,
		Message:      message,
	})
}
