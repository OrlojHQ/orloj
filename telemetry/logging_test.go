package telemetry

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want slog.Level
	}{
		{name: "default", raw: "", want: slog.LevelInfo},
		{name: "debug", raw: "debug", want: slog.LevelDebug},
		{name: "info", raw: "INFO", want: slog.LevelInfo},
		{name: "warn", raw: "warn", want: slog.LevelWarn},
		{name: "warning", raw: "warning", want: slog.LevelWarn},
		{name: "error", raw: "error", want: slog.LevelError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseLogLevel(tt.raw)
			if err != nil {
				t.Fatalf("ParseLogLevel(%q): %v", tt.raw, err)
			}
			if got != tt.want {
				t.Fatalf("ParseLogLevel(%q)=%v, want %v", tt.raw, got, tt.want)
			}
		})
	}
}

func TestParseLogLevelInvalid(t *testing.T) {
	if _, err := ParseLogLevel("trace"); err == nil {
		t.Fatal("expected invalid log level error")
	}
}

func TestResolveLogLevelDebugPrecedence(t *testing.T) {
	got, err := ResolveLogLevel("error", true)
	if err != nil {
		t.Fatalf("ResolveLogLevel: %v", err)
	}
	if got != slog.LevelDebug {
		t.Fatalf("ResolveLogLevel with debug flag=%v, want %v", got, slog.LevelDebug)
	}
}

func TestDebugBridgeLoggerFilteredAtInfo(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelInfo}))

	NewDebugBridgeLogger(logger).Printf("hidden")
	NewBridgeLogger(logger).Printf("visible")

	out := buf.String()
	if strings.Contains(out, "hidden") {
		t.Fatalf("debug bridge log was not filtered: %s", out)
	}
	if !strings.Contains(out, "visible") {
		t.Fatalf("info bridge log missing: %s", out)
	}
}

func TestDebugBridgeLoggerEmitsAtDebug(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	NewDebugBridgeLogger(logger).Printf("visible debug")

	out := buf.String()
	if !strings.Contains(out, `"level":"DEBUG"`) {
		t.Fatalf("debug level missing: %s", out)
	}
	if !strings.Contains(out, "visible debug") {
		t.Fatalf("debug message missing: %s", out)
	}
}

func TestErrorBridgeLoggerEmitsAtError(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelError}))

	NewErrorBridgeLogger(logger).Printf("visible error")

	out := buf.String()
	if !strings.Contains(out, `"level":"ERROR"`) {
		t.Fatalf("error level missing: %s", out)
	}
	if !strings.Contains(out, "visible error") {
		t.Fatalf("error message missing: %s", out)
	}
}
