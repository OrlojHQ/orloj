package agentruntime

import "testing"

func TestBuiltinToolSchemaForMemoryWrite(t *testing.T) {
	schema, ok := builtinToolSchemaForName(ToolMemoryWrite)
	if !ok {
		t.Fatal("expected builtin schema for memory.write")
	}
	if schema.Description == "" {
		t.Fatal("expected non-empty description")
	}
	required, ok := schema.Parameters["required"].([]string)
	if !ok {
		t.Fatalf("expected required []string, got %#v", schema.Parameters["required"])
	}
	if len(required) != 2 || required[0] != "key" || required[1] != "value" {
		t.Fatalf("unexpected required fields: %#v", required)
	}
}
