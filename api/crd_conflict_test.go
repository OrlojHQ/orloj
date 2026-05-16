package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestCheckCRDConflict_Off(t *testing.T) {
	s := &Server{crdConflictPolicy: CRDConflictOff}
	w := httptest.NewRecorder()
	meta := resources.ObjectMeta{
		Annotations: map[string]string{"orloj.dev/managed-by": "crd-sync"},
	}
	if s.checkCRDConflict(w, meta, "Agent") {
		t.Error("expected no conflict when policy is off")
	}
}

func TestCheckCRDConflict_Warn(t *testing.T) {
	s := &Server{crdConflictPolicy: CRDConflictWarn}
	w := httptest.NewRecorder()
	meta := resources.ObjectMeta{
		Annotations: map[string]string{"orloj.dev/managed-by": "crd-sync"},
	}
	if s.checkCRDConflict(w, meta, "Agent") {
		t.Error("expected no blocking when policy is warn")
	}
	if w.Header().Get("X-Orloj-CRD-Managed") != "true" {
		t.Error("expected X-Orloj-CRD-Managed header")
	}
}

func TestCheckCRDConflict_Reject(t *testing.T) {
	s := &Server{crdConflictPolicy: CRDConflictReject}
	w := httptest.NewRecorder()
	meta := resources.ObjectMeta{
		Annotations: map[string]string{"orloj.dev/managed-by": "crd-sync"},
	}
	if !s.checkCRDConflict(w, meta, "Agent") {
		t.Error("expected conflict rejection")
	}
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestCheckCRDConflict_NotManaged(t *testing.T) {
	s := &Server{crdConflictPolicy: CRDConflictReject}
	w := httptest.NewRecorder()
	meta := resources.ObjectMeta{}
	if s.checkCRDConflict(w, meta, "Agent") {
		t.Error("expected no conflict for non-CRD-managed resource")
	}
}

func TestNormalizeCRDConflictPolicy(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{"", CRDConflictWarn},
		{"warn", CRDConflictWarn},
		{"WARN", CRDConflictWarn},
		{"reject", CRDConflictReject},
		{"REJECT", CRDConflictReject},
		{"off", CRDConflictOff},
		{"OFF", CRDConflictOff},
		{"invalid", CRDConflictWarn},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := normalizeCRDConflictPolicy(tt.input); got != tt.expected {
				t.Errorf("normalizeCRDConflictPolicy(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}
