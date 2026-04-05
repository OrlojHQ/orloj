package agentruntime

import "strings"

func builtinOrlojToolSchemaForName(name string) (builtinToolSchema, bool) {
	switch strings.TrimSpace(name) {
	case ToolOrlojTaskCreate:
		return builtinToolSchema{
			Description: "Create a new task from a template. The task runs independently (fire-and-forget). Returns the created task name and initial phase.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"template": map[string]any{
						"type":        "string",
						"description": "Name of an existing task template (mode=template) to instantiate.",
					},
					"input": map[string]any{
						"type":        "object",
						"description": "Key-value input overrides for the new task. Merged with template defaults.",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
					"labels": map[string]any{
						"type":        "object",
						"description": "Additional labels to attach to the new task.",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
				},
				"required":             []string{"template"},
				"additionalProperties": false,
			},
		}, true
	case ToolOrlojTaskList:
		return builtinToolSchema{
			Description: "List tasks in the current namespace, optionally filtered by labels.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"labels": map[string]any{
						"type":        "object",
						"description": "Label key-value pairs to filter tasks. Only tasks matching all labels are returned.",
						"additionalProperties": map[string]any{
							"type": "string",
						},
					},
					"limit": map[string]any{
						"type":        "integer",
						"description": "Maximum number of tasks to return. Defaults to 20.",
					},
				},
				"additionalProperties": false,
			},
		}, true
	default:
		return builtinToolSchema{}, false
	}
}
