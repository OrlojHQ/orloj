package store

import (
	"errors"
	"strings"
)

var (
	ErrSessionNotFound = errors.New("session resource not found")
	ErrSessionInvalid  = errors.New("invalid session operation")
	ErrSessionConflict = errors.New("session operation conflict")
)

type sessionDomainError struct {
	kind    error
	message string
}

func (e *sessionDomainError) Error() string { return e.message }
func (e *sessionDomainError) Unwrap() error { return e.kind }

// ClassifySessionError converts known, user-actionable Session store failures
// into typed errors. Unknown failures are returned unchanged so callers never
// mistake SQL or driver text for a safe client-facing message.
func ClassifySessionError(err error) error {
	if err == nil ||
		errors.Is(err, ErrSessionNotFound) ||
		errors.Is(err, ErrSessionInvalid) ||
		errors.Is(err, ErrSessionConflict) {
		return err
	}
	message := err.Error()
	lower := strings.ToLower(message)
	switch {
	case (strings.HasPrefix(lower, "session ") ||
		strings.HasPrefix(lower, "checkpoint ") ||
		strings.HasPrefix(lower, "turn ")) &&
		strings.HasSuffix(lower, " not found"):
		return &sessionDomainError{kind: ErrSessionNotFound, message: message}
	case strings.HasPrefix(lower, "checkpoint id is required"),
		strings.HasPrefix(lower, "checkpoint state must "),
		strings.HasPrefix(lower, "checkpoint state exceeds "),
		strings.HasPrefix(lower, "checkpoint safe point is required"),
		strings.HasPrefix(lower, "invalid "),
		strings.HasPrefix(lower, "unsupported "),
		strings.HasSuffix(lower, " is required"):
		return &sessionDomainError{kind: ErrSessionInvalid, message: message}
	case strings.Contains(lower, "active turn"),
		strings.HasPrefix(lower, "session ") &&
			(strings.Contains(lower, " is ") ||
				strings.Contains(lower, " has expired") ||
				strings.Contains(lower, " reached max_turns")),
		strings.Contains(lower, "already exists"),
		strings.Contains(lower, "fence"),
		strings.Contains(lower, "hash mismatch"),
		strings.Contains(lower, "lineage"),
		strings.Contains(lower, "cannot rewind"),
		strings.Contains(lower, "cannot fork"),
		strings.Contains(lower, "cannot enqueue"),
		strings.Contains(lower, "cannot pause"),
		strings.Contains(lower, "cannot resume"),
		strings.Contains(lower, "cannot cancel"),
		strings.Contains(lower, "cannot complete"):
		return &sessionDomainError{kind: ErrSessionConflict, message: message}
	default:
		return err
	}
}
