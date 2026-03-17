package telemetry

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"strings"

	"go.opentelemetry.io/otel/trace"
)

type contextKey string

const requestIDKey contextKey = "request_id"

// NewLogger creates a *slog.Logger with a JSON handler for production
// or a text handler for development. The environment variable ORLOJ_LOG_FORMAT
// controls the output: "json" (default) or "text".
func NewLogger(serviceName string) *slog.Logger {
	format := strings.ToLower(strings.TrimSpace(os.Getenv("ORLOJ_LOG_FORMAT")))
	var handler slog.Handler
	switch format {
	case "text":
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	default:
		handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})
	}
	return slog.New(handler).With("service", serviceName)
}

// NewBridgeLogger creates a *log.Logger that writes through slog.
// All output from the returned logger will be formatted as structured
// log entries. Use this to maintain backward compatibility with code
// that expects *log.Logger while gaining structured output.
func NewBridgeLogger(sl *slog.Logger) *log.Logger {
	return log.New(&slogWriter{sl: sl}, "", 0)
}

type slogWriter struct {
	sl *slog.Logger
}

func (w *slogWriter) Write(p []byte) (int, error) {
	msg := strings.TrimRight(string(p), "\n")
	w.sl.Info(msg)
	return len(p), nil
}

// RequestIDMiddleware injects a request ID into the context and response headers.
// If the incoming request has an X-Request-ID header, it is reused; otherwise
// a new random ID is generated.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if rid == "" {
			rid = generateRequestID()
		}
		w.Header().Set("X-Request-ID", rid)
		ctx := context.WithValue(r.Context(), requestIDKey, rid)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestID extracts the request ID from the context.
func RequestID(ctx context.Context) string {
	if v, ok := ctx.Value(requestIDKey).(string); ok {
		return v
	}
	return ""
}

// ContextLogger returns a slog.Logger enriched with request ID and
// OTel trace/span IDs from the context.
func ContextLogger(ctx context.Context, base *slog.Logger) *slog.Logger {
	attrs := make([]slog.Attr, 0, 3)
	if rid := RequestID(ctx); rid != "" {
		attrs = append(attrs, slog.String("request_id", rid))
	}
	sc := trace.SpanFromContext(ctx).SpanContext()
	if sc.HasTraceID() {
		attrs = append(attrs, slog.String("trace_id", sc.TraceID().String()))
	}
	if sc.HasSpanID() {
		attrs = append(attrs, slog.String("span_id", sc.SpanID().String()))
	}
	if len(attrs) == 0 {
		return base
	}
	args := make([]any, len(attrs))
	for i, a := range attrs {
		args[i] = a
	}
	return base.With(args...)
}

func generateRequestID() string {
	b := make([]byte, 8)
	_, _ = io.ReadFull(rand.Reader, b)
	return hex.EncodeToString(b)
}
