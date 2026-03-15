package store

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
)

type ConflictError struct {
	Resource string
	Expected string
	Current  string
}

func (e *ConflictError) Error() string {
	return fmt.Sprintf("%s resourceVersion conflict: expected=%q current=%q", e.Resource, e.Expected, e.Current)
}

func IsConflict(err error) bool {
	_, ok := err.(*ConflictError)
	return ok
}

func initializeCreateMetadata(resource string, meta *crds.ObjectMeta) error {
	expected := strings.TrimSpace(meta.ResourceVersion)
	if expected != "" && expected != "0" {
		return &ConflictError{Resource: resource, Expected: expected, Current: ""}
	}
	meta.ResourceVersion = "1"
	if meta.Generation <= 0 {
		meta.Generation = 1
	}
	if strings.TrimSpace(meta.CreatedAt) == "" {
		meta.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return nil
}

func initializeUpdateMetadata(resource string, meta *crds.ObjectMeta, current crds.ObjectMeta, specChanged bool) error {
	expected := strings.TrimSpace(meta.ResourceVersion)
	if expected != "" && expected != current.ResourceVersion {
		return &ConflictError{Resource: resource, Expected: expected, Current: current.ResourceVersion}
	}
	nextVersion, err := nextResourceVersion(current.ResourceVersion)
	if err != nil {
		return fmt.Errorf("invalid %s current resourceVersion %q: %w", resource, current.ResourceVersion, err)
	}
	meta.ResourceVersion = nextVersion

	currentGeneration := current.Generation
	if currentGeneration <= 0 {
		currentGeneration = 1
	}
	if specChanged {
		meta.Generation = currentGeneration + 1
	} else {
		meta.Generation = currentGeneration
	}

	meta.CreatedAt = current.CreatedAt
	if strings.TrimSpace(meta.CreatedAt) == "" {
		meta.CreatedAt = time.Now().UTC().Format(time.RFC3339)
	}
	return nil
}

func nextResourceVersion(current string) (string, error) {
	cur := strings.TrimSpace(current)
	if cur == "" {
		return "1", nil
	}
	n, err := strconv.ParseInt(cur, 10, 64)
	if err != nil {
		return "", err
	}
	if n < 0 {
		return "", fmt.Errorf("resourceVersion cannot be negative")
	}
	return strconv.FormatInt(n+1, 10), nil
}
