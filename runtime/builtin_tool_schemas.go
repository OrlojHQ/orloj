package agentruntime

import "strings"

type builtinToolSchema struct {
	Description string
	Parameters  map[string]any
}

func builtinToolSchemaForName(name string) (builtinToolSchema, bool) {
	switch strings.TrimSpace(name) {
	case ToolMemoryRead:
		return builtinToolSchema{
			Description: "Read a value from memory by key.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "The exact memory key to read.",
					},
				},
				"required":             []string{"key"},
				"additionalProperties": false,
			},
		}, true
	case ToolMemoryWrite:
		return builtinToolSchema{
			Description: "Write a key-value pair into memory.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"key": map[string]any{
						"type":        "string",
						"description": "The memory key to write.",
					},
					"value": map[string]any{
						"type":        "string",
						"description": "The value to store at the key.",
					},
				},
				"required":             []string{"key", "value"},
				"additionalProperties": false,
			},
		}, true
	case ToolMemorySearch:
		return builtinToolSchema{
			Description: "Search memory entries by query text.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{
						"type":        "string",
						"description": "The search query.",
					},
					"top_k": map[string]any{
						"type":        "integer",
						"description": "Maximum number of results to return.",
					},
				},
				"required":             []string{"query"},
				"additionalProperties": false,
			},
		}, true
	case ToolMemoryList:
		return builtinToolSchema{
			Description: "List memory entries, optionally filtered by key prefix.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"prefix": map[string]any{
						"type":        "string",
						"description": "Optional key prefix filter.",
					},
				},
				"additionalProperties": false,
			},
		}, true
	case ToolMemoryIngest:
		return builtinToolSchema{
			Description: "Ingest a source document into memory by chunking and storing it.",
			Parameters: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"source": map[string]any{
						"type":        "string",
						"description": "Short source label for the ingested content.",
					},
					"content": map[string]any{
						"type":        "string",
						"description": "Full text content to ingest into memory.",
					},
				},
				"required":             []string{"source", "content"},
				"additionalProperties": false,
			},
		}, true
	default:
		return builtinToolSchema{}, false
	}
}
