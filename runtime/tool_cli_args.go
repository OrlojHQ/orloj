package agentruntime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
)

// evaluateCLIArgs evaluates Go text/template strings against parsed JSON input,
// producing a flat argv slice. Each template produces exactly one argv entry;
// entries that evaluate to empty strings are dropped.
//
// When no arg contains "{{", input is ignored for templating (so tools that only
// consume stdin_from_input / have static argv keep working when the model passes
// free-form text). Missing map keys evaluate to the zero value so optional
// fields like `{{ if .name }}{{ .name }}{{ end }}` work without requiring every
// schema property to be present.
func evaluateCLIArgs(templates []string, input string) ([]string, error) {
	needsTemplate := false
	for _, tmplStr := range templates {
		if strings.Contains(tmplStr, "{{") {
			needsTemplate = true
			break
		}
	}
	if !needsTemplate {
		out := make([]string, 0, len(templates))
		for _, tmplStr := range templates {
			if tmplStr != "" {
				out = append(out, tmplStr)
			}
		}
		return out, nil
	}

	var data map[string]any
	input = strings.TrimSpace(input)
	if input != "" {
		if err := json.Unmarshal([]byte(input), &data); err != nil {
			return nil, NewToolError(
				ToolStatusError,
				ToolCodeInvalidInput,
				ToolReasonInvalidInput,
				false,
				fmt.Sprintf("failed to parse tool input as JSON for CLI arg templating: %v", err),
				err,
				map[string]string{"field": "input"},
			)
		}
	}
	if data == nil {
		data = make(map[string]any)
	}

	out := make([]string, 0, len(templates))
	for i, tmplStr := range templates {
		if !strings.Contains(tmplStr, "{{") {
			out = append(out, tmplStr)
			continue
		}
		// missingkey=zero: optional fields ({{ if .name }}) must not fail when absent.
		tmpl, err := template.New(fmt.Sprintf("arg[%d]", i)).Option("missingkey=zero").Parse(tmplStr)
		if err != nil {
			return nil, NewToolError(
				ToolStatusError,
				ToolCodeInvalidInput,
				ToolReasonInvalidInput,
				false,
				fmt.Sprintf("invalid CLI arg template at index %d: %v", i, err),
				err,
				map[string]string{"index": fmt.Sprintf("%d", i), "template": tmplStr},
			)
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, data); err != nil {
			return nil, NewToolError(
				ToolStatusError,
				ToolCodeInvalidInput,
				ToolReasonInvalidInput,
				false,
				fmt.Sprintf("failed to evaluate CLI arg template at index %d: %v", i, err),
				err,
				map[string]string{"index": fmt.Sprintf("%d", i), "template": tmplStr},
			)
		}
		value := strings.TrimSpace(buf.String())
		// text/template prints "<no value>" for nil interface{} map entries even
		// with missingkey=zero; treat that as empty so optional fields drop cleanly.
		if value == "<no value>" {
			value = ""
		}
		if value != "" {
			out = append(out, value)
		}
	}
	return out, nil
}

// totalArgvLength returns the combined byte length of all argv entries.
func totalArgvLength(args []string) int {
	total := 0
	for _, arg := range args {
		total += len(arg)
	}
	return total
}
