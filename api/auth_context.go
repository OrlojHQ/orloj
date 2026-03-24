package api

import (
	"context"
	"net/http"
)

type authContextKey struct{}

// AuthIdentity carries the authenticated caller's identity through the
// request context for audit logging and downstream authorization.
type AuthIdentity struct {
	Role   string
	Method string // "bearer", "session", "none"
}

func withAuthIdentity(ctx context.Context, id AuthIdentity) context.Context {
	return context.WithValue(ctx, authContextKey{}, id)
}

// AuthIdentityFromRequest extracts the authenticated identity from the
// request context, if present.
func AuthIdentityFromRequest(r *http.Request) (AuthIdentity, bool) {
	id, ok := r.Context().Value(authContextKey{}).(AuthIdentity)
	return id, ok
}

// ResourceAuthorizer is an optional extension point for fine-grained access
// control beyond the built-in role check. A custom authorization layer can
// implement this interface to enforce per-namespace, per-resource-type, or
// per-user policies. Nil by default (all access permitted after the role
// check passes).
//
// The method, resourceType, namespace, and name describe the operation.
// resourceType is the API resource kind (e.g. "Agent", "Secret", "Task").
// The namespace and name may be empty for list/create operations.
//
// Returning (true, 0, "") allows the request. Returning (false, statusCode,
// message) rejects it.
type ResourceAuthorizer interface {
	AuthorizeResource(r *http.Request, method, resourceType, namespace, name string) (allowed bool, statusCode int, message string)
}
