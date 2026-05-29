package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"
)

func TestSlogAuditSinkRecordsStructuredEvent(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))
	sink := NewSlogAuditSink(logger)

	sink.RecordAudit(context.Background(), AuditEvent{
		Timestamp:    "2026-05-29T00:00:00Z",
		Component:    "apiserver",
		Action:       "token.create",
		Outcome:      "success",
		Namespace:    "default",
		ResourceKind: "Token",
		ResourceName: "ci-bot",
		Principal:    "admin",
		Message:      "created",
		Metadata:     map[string]string{"role": "admin"},
	})

	var rec map[string]any
	if err := json.Unmarshal(bytes.TrimSpace(buf.Bytes()), &rec); err != nil {
		t.Fatalf("audit record is not valid JSON: %v\noutput: %s", err, buf.String())
	}

	for key, want := range map[string]string{
		"action":        "token.create",
		"outcome":       "success",
		"principal":     "admin",
		"resource_kind": "Token",
		"namespace":     "default",
		"meta.role":     "admin",
	} {
		if got, _ := rec[key].(string); got != want {
			t.Errorf("field %q = %q, want %q", key, got, want)
		}
	}
}

func TestSlogAuditSinkNilSafe(t *testing.T) {
	var sink *SlogAuditSink
	// Must not panic on a nil receiver or nil logger.
	sink.RecordAudit(context.Background(), AuditEvent{Action: "noop"})

	NewSlogAuditSink(nil) // default handler path must not panic
}
