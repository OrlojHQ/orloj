package resources

import "testing"

func TestToolNormalizeAcceptsA2AType(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "a2a-tool"},
		Spec: ToolSpec{
			Type: "a2a",
			A2A: ToolA2ASpec{
				AgentURL: "https://remote.example.com/a2a",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("expected a2a type to normalize, got %v", err)
	}
	if tool.Spec.Type != "a2a" {
		t.Fatalf("expected type=a2a, got %q", tool.Spec.Type)
	}
}

func TestToolNormalizeA2ARequiresAgentURL(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "a2a-tool"},
		Spec: ToolSpec{
			Type: "a2a",
			A2A:  ToolA2ASpec{},
		},
	}
	if err := tool.Normalize(); err == nil {
		t.Fatal("expected error when a2a.agent_url is missing")
	}
}

func TestToolNormalizeA2ATrimsAgentURL(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "a2a-tool"},
		Spec: ToolSpec{
			Type: "a2a",
			A2A: ToolA2ASpec{
				AgentURL: "  https://remote.example.com/a2a  ",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.A2A.AgentURL != "https://remote.example.com/a2a" {
		t.Fatalf("expected trimmed URL, got %q", tool.Spec.A2A.AgentURL)
	}
}

func TestToolNormalizeA2ATrimsProtocolVersion(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "a2a-tool"},
		Spec: ToolSpec{
			Type: "a2a",
			A2A: ToolA2ASpec{
				AgentURL:        "https://remote.example.com/a2a",
				ProtocolVersion: "  1.0 ",
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if tool.Spec.A2A.ProtocolVersion != "1.0" {
		t.Fatalf("expected trimmed protocol version, got %q", tool.Spec.A2A.ProtocolVersion)
	}
}

func TestToolNormalizeA2AOptionalFields(t *testing.T) {
	tool := Tool{
		Metadata: ObjectMeta{Name: "a2a-tool"},
		Spec: ToolSpec{
			Type: "a2a",
			A2A: ToolA2ASpec{
				AgentURL:        "https://remote.example.com/a2a",
				ProtocolVersion: "1.0",
				PreferStreaming:  true,
			},
		},
	}
	if err := tool.Normalize(); err != nil {
		t.Fatalf("normalize failed: %v", err)
	}
	if !tool.Spec.A2A.PreferStreaming {
		t.Fatal("expected prefer_streaming to be true")
	}
}

func TestToolNormalizeA2ACaseInsensitiveType(t *testing.T) {
	for _, typeName := range []string{"A2A", "a2a", "A2a"} {
		tool := Tool{
			Metadata: ObjectMeta{Name: "a2a-tool"},
			Spec: ToolSpec{
				Type: typeName,
				A2A:  ToolA2ASpec{AgentURL: "https://example.com/a2a"},
			},
		}
		if err := tool.Normalize(); err != nil {
			t.Fatalf("expected type %q to normalize, got %v", typeName, err)
		}
		if tool.Spec.Type != "a2a" {
			t.Fatalf("expected normalized type=a2a, got %q", tool.Spec.Type)
		}
	}
}

func TestParseToolManifestA2A(t *testing.T) {
	yaml := `apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: remote-summarizer
spec:
  type: a2a
  description: Summarize documents via remote agent
  a2a:
    agent_url: https://remote.example.com/v1/agents/summarizer/a2a
    protocol_version: "1.0"
    prefer_streaming: true
`
	tool, err := ParseToolManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tool.Spec.Type != "a2a" {
		t.Fatalf("expected type=a2a, got %q", tool.Spec.Type)
	}
	if tool.Spec.A2A.AgentURL != "https://remote.example.com/v1/agents/summarizer/a2a" {
		t.Fatalf("expected agent_url, got %q", tool.Spec.A2A.AgentURL)
	}
	if tool.Spec.A2A.ProtocolVersion != "1.0" {
		t.Fatalf("expected protocol_version=1.0, got %q", tool.Spec.A2A.ProtocolVersion)
	}
	if !tool.Spec.A2A.PreferStreaming {
		t.Fatal("expected prefer_streaming=true")
	}
}

func TestParseToolManifestA2AMinimal(t *testing.T) {
	yaml := `apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: remote-tool
spec:
  type: a2a
  a2a:
    agent_url: https://remote.example.com/a2a
`
	tool, err := ParseToolManifest([]byte(yaml))
	if err != nil {
		t.Fatalf("parse failed: %v", err)
	}
	if tool.Spec.Type != "a2a" {
		t.Fatalf("expected type=a2a, got %q", tool.Spec.Type)
	}
	if tool.Spec.A2A.AgentURL != "https://remote.example.com/a2a" {
		t.Fatalf("expected agent_url, got %q", tool.Spec.A2A.AgentURL)
	}
}
