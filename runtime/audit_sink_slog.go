package agentruntime

import (
	"context"
	"log/slog"
	"os"
)

// SlogAuditSink is a reference AuditSink implementation that writes audit
// events as structured records via log/slog. It is intentionally simple and
// dependency-free so operators can adopt it directly or use it as a template
// for a SIEM/forwarder integration.
//
// Audit logging is OFF by default in Orloj (the runtime uses a no-op sink).
// Wire this sink explicitly when you need a durable audit trail:
//
//	ext := agentruntime.Extensions{
//	    Audit: agentruntime.NewSlogAuditSink(nil), // JSON to stdout
//	}
//	server := api.NewServer(api.ServerOptions{Extensions: ext, /* ... */})
//
// For production, point the underlying slog.Logger at a file or collector and
// ship those records to durable, access-controlled, retention-bound storage
// (see docs/pages/operations/security.md, "Audit Logging").
type SlogAuditSink struct {
	logger *slog.Logger
}

// NewSlogAuditSink returns a SlogAuditSink. If logger is nil, it defaults to a
// JSON handler writing to stdout at Info level, which is convenient for
// container log collection.
func NewSlogAuditSink(logger *slog.Logger) *SlogAuditSink {
	if logger == nil {
		logger = slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	}
	return &SlogAuditSink{logger: logger}
}

// RecordAudit implements AuditSink. It never panics and never blocks on the
// caller beyond the underlying handler write, satisfying the AuditSink
// contract that implementations be resilient.
func (s *SlogAuditSink) RecordAudit(ctx context.Context, event AuditEvent) {
	if s == nil || s.logger == nil {
		return
	}

	attrs := []any{
		slog.String("timestamp", event.Timestamp),
		slog.String("action", event.Action),
		slog.String("outcome", event.Outcome),
	}
	if event.Component != "" {
		attrs = append(attrs, slog.String("component", event.Component))
	}
	if event.Namespace != "" {
		attrs = append(attrs, slog.String("namespace", event.Namespace))
	}
	if event.ResourceKind != "" {
		attrs = append(attrs, slog.String("resource_kind", event.ResourceKind))
	}
	if event.ResourceName != "" {
		attrs = append(attrs, slog.String("resource_name", event.ResourceName))
	}
	if event.Principal != "" {
		attrs = append(attrs, slog.String("principal", event.Principal))
	}
	if event.Message != "" {
		attrs = append(attrs, slog.String("message", event.Message))
	}
	for k, v := range event.Metadata {
		attrs = append(attrs, slog.String("meta."+k, v))
	}

	s.logger.LogAttrs(ctx, slog.LevelInfo, "audit", attrsToSlog(attrs)...)
}

// attrsToSlog converts the []any built above into []slog.Attr for LogAttrs.
func attrsToSlog(attrs []any) []slog.Attr {
	out := make([]slog.Attr, 0, len(attrs))
	for _, a := range attrs {
		if sa, ok := a.(slog.Attr); ok {
			out = append(out, sa)
		}
	}
	return out
}
