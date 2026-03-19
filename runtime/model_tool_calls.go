package agentruntime

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var toolDirectiveRegex = regexp.MustCompile(`(?i)\btool\s*[:=]\s*([a-z0-9._/\-]+)(?:\s+input\s*[:=]\s*(.+))?$`) //nolint:gochecknoglobals

func selectAuthorizedToolCalls(response ModelResponse, availableTools []string) ([]ModelToolCall, error) {
	allowed := normalizeAllowedTools(availableTools)
	if len(allowed) == 0 {
		return nil, nil
	}

	requested := normalizeModelToolCalls(response.ToolCalls)
	if len(requested) == 0 {
		requested = normalizeModelToolCalls(parseToolCallsFromModelContent(response.Content))
	}
	if len(requested) == 0 {
		return nil, nil
	}

	out := make([]ModelToolCall, 0, len(requested))
	for _, call := range requested {
		key := normalizeToolKey(call.Name)
		if _, ok := allowed[key]; !ok {
			allowedList := make([]string, 0, len(allowed))
			for _, name := range allowed {
				allowedList = append(allowedList, name)
			}
			sort.Strings(allowedList)
			return nil, NewToolDeniedError(
				fmt.Sprintf("model requested unauthorized tool=%s allowed=%s", call.Name, strings.Join(allowedList, ",")),
				map[string]string{
					"tool":    strings.TrimSpace(call.Name),
					"allowed": strings.Join(allowedList, ","),
				},
				ErrToolPermissionDenied,
			)
		}
		call.Name = allowed[key]
		out = append(out, call)
	}
	return out, nil
}

func normalizeAllowedTools(raw []string) map[string]string {
	out := make(map[string]string, len(raw))
	for _, value := range raw {
		trimmed := strings.TrimSpace(value)
		key := normalizeToolKey(trimmed)
		if key == "" {
			continue
		}
		if _, exists := out[key]; exists {
			continue
		}
		out[key] = trimmed
	}
	return out
}

func normalizeModelToolCalls(raw []ModelToolCall) []ModelToolCall {
	out := make([]ModelToolCall, 0, len(raw))
	for _, call := range raw {
		name := strings.TrimSpace(call.Name)
		if name == "" {
			continue
		}
		out = append(out, ModelToolCall{
			ID:    strings.TrimSpace(call.ID),
			Name:  name,
			Input: strings.TrimSpace(call.Input),
		})
	}
	return out
}

func parseToolCallsFromModelContent(content string) []ModelToolCall {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}

	candidates := []string{content}
	if unwrapped := unwrapFencedCodeBlock(content); unwrapped != "" && unwrapped != content {
		candidates = append(candidates, unwrapped)
	}

	for _, candidate := range candidates {
		if calls := parseToolCallsFromJSON(candidate); len(calls) > 0 {
			return calls
		}
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		matches := toolDirectiveRegex.FindStringSubmatch(line)
		if len(matches) < 2 {
			continue
		}
		call := ModelToolCall{Name: strings.TrimSpace(matches[1])}
		if len(matches) > 2 {
			call.Input = strings.TrimSpace(matches[2])
		}
		if call.Name != "" {
			return []ModelToolCall{call}
		}
	}
	return nil
}

func parseToolCallsFromJSON(raw string) []ModelToolCall {
	type toolItem struct {
		Name  string `json:"name"`
		Tool  string `json:"tool"`
		Input string `json:"input"`
	}
	type envelope struct {
		Name      string     `json:"name"`
		Tool      string     `json:"tool"`
		Input     string     `json:"input"`
		Tools     []toolItem `json:"tools"`
		ToolCalls []toolItem `json:"tool_calls"`
	}

	parsed := envelope{}
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		return nil
	}

	out := make([]ModelToolCall, 0, 4)
	appendItem := func(name string, input string) {
		name = strings.TrimSpace(name)
		if name == "" {
			return
		}
		out = append(out, ModelToolCall{
			Name:  name,
			Input: strings.TrimSpace(input),
		})
	}

	appendItem(firstNonEmptyToolCall(parsed.Name, parsed.Tool), parsed.Input)
	for _, item := range parsed.Tools {
		appendItem(firstNonEmptyToolCall(item.Name, item.Tool), item.Input)
	}
	for _, item := range parsed.ToolCalls {
		appendItem(firstNonEmptyToolCall(item.Name, item.Tool), item.Input)
	}
	return out
}

func unwrapFencedCodeBlock(content string) string {
	content = strings.TrimSpace(content)
	if !strings.HasPrefix(content, "```") {
		return content
	}
	lines := strings.Split(content, "\n")
	if len(lines) < 3 {
		return content
	}
	if !strings.HasPrefix(strings.TrimSpace(lines[0]), "```") {
		return content
	}
	last := strings.TrimSpace(lines[len(lines)-1])
	if !strings.HasPrefix(last, "```") {
		return content
	}
	body := strings.Join(lines[1:len(lines)-1], "\n")
	return strings.TrimSpace(body)
}

func firstNonEmptyToolCall(values ...string) string {
	for _, value := range values {
		trimmed := strings.TrimSpace(value)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
