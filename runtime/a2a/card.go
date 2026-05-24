package a2a

import (
	"net/url"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// CardGeneratorConfig controls Agent Card generation.
type CardGeneratorConfig struct {
	PublicBaseURL    string
	ProtocolVersion  string
	StreamingEnabled bool
	WebhooksEnabled  bool
	AuthSchemes      []string
	Namespace        string
}

// GenerateAgentCard builds an A2A Agent Card from an Orloj Agent and its tools.
func GenerateAgentCard(agent resources.Agent, tools []resources.Tool, config CardGeneratorConfig) AgentCard {
	name := agent.Metadata.Name
	desc := ""
	if agent.Metadata.Annotations != nil {
		desc = strings.TrimSpace(agent.Metadata.Annotations["orloj.dev/description"])
	}
	if desc == "" {
		desc = truncatePrompt(agent.Spec.Prompt, 200)
	}

	agentURL := appendNamespaceQuery(strings.TrimSuffix(config.PublicBaseURL, "/")+"/v1/agents/"+url.PathEscape(name)+"/a2a", config.Namespace)

	card := AgentCard{
		Name:            name,
		Description:     desc,
		URL:             agentURL,
		ProtocolVersion: config.ProtocolVersion,
		Capabilities: CardCapabilities{
			Streaming:         config.StreamingEnabled,
			PushNotifications: config.WebhooksEnabled,
			StateTransitions:  true,
		},
	}

	if len(config.AuthSchemes) > 0 {
		card.Authentication = &CardAuth{Schemes: config.AuthSchemes}
	}

	for _, tool := range tools {
		skill := CardSkill{
			ID:          tool.Metadata.Name,
			Name:        tool.Metadata.Name,
			Description: tool.Spec.Description,
		}
		if len(tool.Spec.InputSchema) > 0 {
			skill.InputSchema = tool.Spec.InputSchema
		}
		card.Skills = append(card.Skills, skill)
	}

	return card
}

// GenerateSystemCard builds a card from an AgentSystem. It aggregates
// skills from all agents in the system.
func GenerateSystemCard(system resources.AgentSystem, agents []resources.Agent, tools []resources.Tool, config CardGeneratorConfig) AgentCard {
	name := system.Metadata.Name
	desc := ""
	if system.Metadata.Annotations != nil {
		desc = strings.TrimSpace(system.Metadata.Annotations["orloj.dev/description"])
	}
	if desc == "" && len(agents) > 0 {
		desc = "Multi-agent system: " + name
	}

	agentURL := appendNamespaceQuery(strings.TrimSuffix(config.PublicBaseURL, "/")+"/v1/agent-systems/"+url.PathEscape(name)+"/a2a", config.Namespace)

	card := AgentCard{
		Name:            name,
		Description:     desc,
		URL:             agentURL,
		ProtocolVersion: config.ProtocolVersion,
		Capabilities: CardCapabilities{
			Streaming:         config.StreamingEnabled,
			PushNotifications: config.WebhooksEnabled,
			StateTransitions:  true,
		},
	}

	if len(config.AuthSchemes) > 0 {
		card.Authentication = &CardAuth{Schemes: config.AuthSchemes}
	}

	toolsByName := make(map[string]resources.Tool, len(tools))
	for _, t := range tools {
		toolsByName[t.Metadata.Name] = t
	}

	seen := make(map[string]struct{})
	for _, agent := range agents {
		for _, toolName := range agent.Spec.Tools {
			if _, ok := seen[toolName]; ok {
				continue
			}
			seen[toolName] = struct{}{}
			tool, ok := toolsByName[toolName]
			if !ok {
				continue
			}
			skill := CardSkill{
				ID:          tool.Metadata.Name,
				Name:        tool.Metadata.Name,
				Description: tool.Spec.Description,
			}
			if len(tool.Spec.InputSchema) > 0 {
				skill.InputSchema = tool.Spec.InputSchema
			}
			card.Skills = append(card.Skills, skill)
		}
	}

	return card
}

func appendNamespaceQuery(rawURL, namespace string) string {
	namespace = resources.NormalizeNamespace(namespace)
	if namespace == "" || namespace == resources.DefaultNamespace {
		return rawURL
	}
	sep := "?"
	if strings.Contains(rawURL, "?") {
		sep = "&"
	}
	return rawURL + sep + "namespace=" + url.QueryEscape(namespace)
}

func truncatePrompt(prompt string, maxLen int) string {
	prompt = strings.TrimSpace(prompt)
	if len(prompt) <= maxLen {
		return prompt
	}
	return prompt[:maxLen-3] + "..."
}
