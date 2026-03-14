package agentruntime

import (
	"context"
	"fmt"
	"strings"
)

// MockModelGateway is an in-process placeholder model adapter.
type MockModelGateway struct{}

func (m *MockModelGateway) Complete(_ context.Context, req ModelRequest) (ModelResponse, error) {
	response := ModelResponse{
		Content: fmt.Sprintf("model=%s step=%d", req.Model, req.Step),
		Done:    false,
	}
	call, ok := mockSelectToolCall(req)
	if ok {
		response.ToolCalls = []ModelToolCall{call}
	}
	return response, nil
}

func mockSelectToolCall(req ModelRequest) (ModelToolCall, bool) {
	if len(req.Tools) == 0 {
		return ModelToolCall{}, false
	}
	signalParts := []string{
		strings.TrimSpace(req.Prompt),
		strings.TrimSpace(req.Context["topic"]),
		strings.TrimSpace(req.Context["inbox.content"]),
		strings.TrimSpace(req.Context["previous_agent_last_event"]),
	}
	signal := strings.ToLower(strings.Join(signalParts, " "))
	pick := func(pattern string) (string, bool) {
		for _, tool := range req.Tools {
			toolName := strings.TrimSpace(tool)
			if strings.Contains(strings.ToLower(toolName), pattern) {
				return toolName, true
			}
		}
		return "", false
	}
	for _, pattern := range []string{"vector", "search", "web", "db", "echo"} {
		if !strings.Contains(signal, pattern) {
			continue
		}
		if tool, ok := pick(pattern); ok {
			return ModelToolCall{Name: tool, Input: buildMockToolInput(req)}, true
		}
	}
	if req.Step == 1 {
		return ModelToolCall{
			Name:  strings.TrimSpace(req.Tools[0]),
			Input: buildMockToolInput(req),
		}, true
	}
	return ModelToolCall{}, false
}

func buildMockToolInput(req ModelRequest) string {
	candidates := []string{
		strings.TrimSpace(req.Context["inbox.content"]),
		strings.TrimSpace(req.Context["topic"]),
	}
	for _, value := range candidates {
		if value != "" {
			return value
		}
	}
	return fmt.Sprintf("agent=%s step=%d", strings.TrimSpace(req.Agent), req.Step)
}
