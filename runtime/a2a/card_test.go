package a2a

import (
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestGenerateAgentCard_BasicFields(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{
			Name: "test-agent",
			Annotations: map[string]string{
				"orloj.dev/description": "A test agent",
			},
		},
		Spec: resources.AgentSpec{
			Prompt: "You are a helpful assistant",
			Tools:  []string{"search-tool"},
		},
	}

	tools := []resources.Tool{{
		Metadata: resources.ObjectMeta{Name: "search-tool"},
		Spec: resources.ToolSpec{
			Description: "Searches the web",
			InputSchema: map[string]any{"type": "object", "properties": map[string]any{"query": map[string]any{"type": "string"}}},
		},
	}}

	config := CardGeneratorConfig{
		PublicBaseURL:    "https://orloj.example.com",
		ProtocolVersion: "1.0",
		StreamingEnabled: true,
		WebhooksEnabled:  true,
		AuthSchemes:      []string{"bearer"},
	}

	card := GenerateAgentCard(agent, tools, config)

	if card.Name != "test-agent" {
		t.Errorf("expected name test-agent, got %s", card.Name)
	}
	if card.Description != "A test agent" {
		t.Errorf("expected description 'A test agent', got %q", card.Description)
	}
	if card.URL != "https://orloj.example.com/v1/agents/test-agent/a2a" {
		t.Errorf("unexpected URL: %s", card.URL)
	}
	if card.ProtocolVersion != "1.0" {
		t.Errorf("expected protocol version 1.0, got %s", card.ProtocolVersion)
	}
	if !card.Capabilities.Streaming {
		t.Error("expected streaming=true")
	}
	if !card.Capabilities.PushNotifications {
		t.Error("expected pushNotifications=true")
	}
	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	if card.Skills[0].Name != "search-tool" {
		t.Errorf("expected skill name search-tool, got %s", card.Skills[0].Name)
	}
	if card.Skills[0].Description != "Searches the web" {
		t.Errorf("expected skill description 'Searches the web', got %q", card.Skills[0].Description)
	}
	if card.Skills[0].InputSchema == nil {
		t.Error("expected skill input schema to be set")
	}
	if card.Authentication == nil || len(card.Authentication.Schemes) != 1 || card.Authentication.Schemes[0] != "bearer" {
		t.Errorf("unexpected auth: %+v", card.Authentication)
	}
}

func TestGenerateAgentCard_FallbackDescription(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "no-desc"},
		Spec:     resources.AgentSpec{Prompt: "Short prompt"},
	}

	card := GenerateAgentCard(agent, nil, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if card.Description != "Short prompt" {
		t.Errorf("expected prompt fallback, got %q", card.Description)
	}
}

func TestGenerateAgentCard_NoToolsNoSkills(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "bare"},
		Spec:     resources.AgentSpec{Prompt: "Hello"},
	}

	card := GenerateAgentCard(agent, nil, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if len(card.Skills) != 0 {
		t.Errorf("expected 0 skills, got %d", len(card.Skills))
	}
}

func TestGenerateAgentCard_NoInputSchemaOmitted(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "agent"},
		Spec:     resources.AgentSpec{Prompt: "test", Tools: []string{"tool1"}},
	}
	tools := []resources.Tool{{
		Metadata: resources.ObjectMeta{Name: "tool1"},
		Spec:     resources.ToolSpec{Description: "A tool"},
	}}

	card := GenerateAgentCard(agent, tools, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill, got %d", len(card.Skills))
	}
	if card.Skills[0].InputSchema != nil {
		t.Error("expected nil input schema for tool without schema")
	}
}

func TestGenerateSystemCard(t *testing.T) {
	system := resources.AgentSystem{
		Metadata: resources.ObjectMeta{
			Name: "pipeline",
			Annotations: map[string]string{
				"orloj.dev/description": "A pipeline system",
			},
		},
		Spec: resources.AgentSystemSpec{
			Agents: []string{"agent-a", "agent-b"},
		},
	}

	agents := []resources.Agent{
		{Metadata: resources.ObjectMeta{Name: "agent-a"}, Spec: resources.AgentSpec{Tools: []string{"tool1"}}},
		{Metadata: resources.ObjectMeta{Name: "agent-b"}, Spec: resources.AgentSpec{Tools: []string{"tool1", "tool2"}}},
	}

	tools := []resources.Tool{
		{Metadata: resources.ObjectMeta{Name: "tool1"}, Spec: resources.ToolSpec{Description: "Tool 1"}},
		{Metadata: resources.ObjectMeta{Name: "tool2"}, Spec: resources.ToolSpec{Description: "Tool 2"}},
	}

	config := CardGeneratorConfig{PublicBaseURL: "https://example.com", ProtocolVersion: "1.0"}
	card := GenerateSystemCard(system, agents, tools, config)

	if card.Name != "pipeline" {
		t.Errorf("expected name pipeline, got %s", card.Name)
	}
	if card.Description != "A pipeline system" {
		t.Errorf("unexpected description: %q", card.Description)
	}
	if len(card.Skills) != 2 {
		t.Errorf("expected 2 deduplicated skills, got %d", len(card.Skills))
	}
}

func TestGenerateAgentCard_NoAuth(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "no-auth-agent"},
		Spec:     resources.AgentSpec{Prompt: "Test"},
	}

	card := GenerateAgentCard(agent, nil, CardGeneratorConfig{
		PublicBaseURL: "https://example.com",
		AuthSchemes:   nil,
	})

	if card.Authentication != nil {
		t.Errorf("expected nil auth when no schemes, got %+v", card.Authentication)
	}
}

func TestGenerateAgentCard_StreamingDisabled(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "no-stream"},
		Spec:     resources.AgentSpec{Prompt: "Test"},
	}

	card := GenerateAgentCard(agent, nil, CardGeneratorConfig{
		PublicBaseURL:    "https://example.com",
		StreamingEnabled: false,
		WebhooksEnabled:  false,
	})

	if card.Capabilities.Streaming {
		t.Error("expected streaming=false")
	}
	if card.Capabilities.PushNotifications {
		t.Error("expected pushNotifications=false")
	}
	if !card.Capabilities.StateTransitions {
		t.Error("expected stateTransitions=true always")
	}
}

func TestGenerateAgentCard_MultipleTools(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "multi-tool"},
		Spec:     resources.AgentSpec{Prompt: "Agent with tools", Tools: []string{"tool1", "tool2", "tool3"}},
	}

	tools := []resources.Tool{
		{Metadata: resources.ObjectMeta{Name: "tool1"}, Spec: resources.ToolSpec{Description: "Tool 1"}},
		{Metadata: resources.ObjectMeta{Name: "tool2"}, Spec: resources.ToolSpec{Description: "Tool 2"}},
		{Metadata: resources.ObjectMeta{Name: "tool3"}, Spec: resources.ToolSpec{Description: "Tool 3"}},
	}

	card := GenerateAgentCard(agent, tools, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if len(card.Skills) != 3 {
		t.Fatalf("expected 3 skills, got %d", len(card.Skills))
	}
	for i, skill := range card.Skills {
		expected := tools[i].Metadata.Name
		if skill.ID != expected {
			t.Errorf("skill %d: expected ID %q, got %q", i, expected, skill.ID)
		}
	}
}

func TestGenerateAgentCard_TrailingSlashBaseURL(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "agent"},
		Spec:     resources.AgentSpec{Prompt: "Test"},
	}

	card := GenerateAgentCard(agent, nil, CardGeneratorConfig{PublicBaseURL: "https://example.com/"})

	expected := "https://example.com/v1/agents/agent/a2a"
	if card.URL != expected {
		t.Errorf("expected %q, got %q", expected, card.URL)
	}
}

func TestGenerateAgentCard_MultipleAuthSchemes(t *testing.T) {
	agent := resources.Agent{
		Metadata: resources.ObjectMeta{Name: "agent"},
		Spec:     resources.AgentSpec{Prompt: "Test"},
	}

	card := GenerateAgentCard(agent, nil, CardGeneratorConfig{
		PublicBaseURL: "https://example.com",
		AuthSchemes:   []string{"bearer", "apikey"},
	})

	if card.Authentication == nil {
		t.Fatal("expected authentication to be set")
	}
	if len(card.Authentication.Schemes) != 2 {
		t.Errorf("expected 2 auth schemes, got %d", len(card.Authentication.Schemes))
	}
}

func TestGenerateSystemCard_DeduplicatesAcrossAgents(t *testing.T) {
	system := resources.AgentSystem{
		Metadata: resources.ObjectMeta{Name: "system"},
		Spec:     resources.AgentSystemSpec{Agents: []string{"a", "b"}},
	}

	agents := []resources.Agent{
		{Metadata: resources.ObjectMeta{Name: "a"}, Spec: resources.AgentSpec{Tools: []string{"shared-tool", "a-only"}}},
		{Metadata: resources.ObjectMeta{Name: "b"}, Spec: resources.AgentSpec{Tools: []string{"shared-tool", "b-only"}}},
	}

	tools := []resources.Tool{
		{Metadata: resources.ObjectMeta{Name: "shared-tool"}, Spec: resources.ToolSpec{Description: "Shared"}},
		{Metadata: resources.ObjectMeta{Name: "a-only"}, Spec: resources.ToolSpec{Description: "A only"}},
		{Metadata: resources.ObjectMeta{Name: "b-only"}, Spec: resources.ToolSpec{Description: "B only"}},
	}

	card := GenerateSystemCard(system, agents, tools, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if len(card.Skills) != 3 {
		t.Fatalf("expected 3 deduplicated skills, got %d", len(card.Skills))
	}
}

func TestGenerateSystemCard_SkipsMissingTools(t *testing.T) {
	system := resources.AgentSystem{
		Metadata: resources.ObjectMeta{Name: "system"},
		Spec:     resources.AgentSystemSpec{Agents: []string{"a"}},
	}

	agents := []resources.Agent{
		{Metadata: resources.ObjectMeta{Name: "a"}, Spec: resources.AgentSpec{Tools: []string{"existing", "missing"}}},
	}

	tools := []resources.Tool{
		{Metadata: resources.ObjectMeta{Name: "existing"}, Spec: resources.ToolSpec{Description: "Exists"}},
	}

	card := GenerateSystemCard(system, agents, tools, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if len(card.Skills) != 1 {
		t.Fatalf("expected 1 skill (skipping missing), got %d", len(card.Skills))
	}
}

func TestGenerateSystemCard_FallbackDescription(t *testing.T) {
	system := resources.AgentSystem{
		Metadata: resources.ObjectMeta{Name: "my-system"},
		Spec:     resources.AgentSystemSpec{Agents: []string{"a"}},
	}
	agents := []resources.Agent{
		{Metadata: resources.ObjectMeta{Name: "a"}, Spec: resources.AgentSpec{Prompt: "Test"}},
	}

	card := GenerateSystemCard(system, agents, nil, CardGeneratorConfig{PublicBaseURL: "https://example.com"})

	if card.Description != "Multi-agent system: my-system" {
		t.Errorf("expected fallback description, got %q", card.Description)
	}
}

func TestTruncatePrompt(t *testing.T) {
	short := "Hello"
	if got := truncatePrompt(short, 200); got != "Hello" {
		t.Errorf("expected %q, got %q", short, got)
	}

	long := ""
	for i := 0; i < 250; i++ {
		long += "x"
	}
	result := truncatePrompt(long, 200)
	if len(result) != 200 {
		t.Errorf("expected length 200, got %d", len(result))
	}
	if result[197:] != "..." {
		t.Errorf("expected trailing ..., got %q", result[197:])
	}
}
