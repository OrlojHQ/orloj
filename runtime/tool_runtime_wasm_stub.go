package agentruntime

import (
	"context"
	"fmt"
	"strings"
)

// UnsupportedWASMToolRuntime is an explicit fail-closed placeholder until a real WASM executor is wired.
type UnsupportedWASMToolRuntime struct{}

func NewUnsupportedWASMToolRuntime() ToolRuntime {
	return &UnsupportedWASMToolRuntime{}
}

func (r *UnsupportedWASMToolRuntime) Call(_ context.Context, tool string, _ string) (string, error) {
	tool = strings.TrimSpace(tool)
	return "", NewToolError(
		ToolStatusError,
		ToolCodeIsolationUnavailable,
		ToolReasonIsolationUnavailable,
		false,
		fmt.Sprintf("wasm isolation runtime unavailable for tool=%s", tool),
		ErrToolIsolationUnavailable,
		map[string]string{
			"tool":           tool,
			"isolation_mode": "wasm",
		},
	)
}
