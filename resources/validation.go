package resources

// Public validation API for external consumers (CLI tools, CRD webhooks,
// admission controllers). The store layer calls Normalize() internally on
// Upsert, so these are not used by the core server — they exist so that
// external packages can validate resources without importing store internals.

// ValidateAgent normalizes and validates an Agent resource.
func ValidateAgent(a *Agent) error { return a.Normalize() }

// ValidateAgentSystem normalizes and validates an AgentSystem resource.
func ValidateAgentSystem(a *AgentSystem) error { return a.Normalize() }

// ValidateTool normalizes and validates a Tool resource.
func ValidateTool(t *Tool) error { return t.Normalize() }

// ValidateMcpServer normalizes and validates an McpServer resource.
func ValidateMcpServer(m *McpServer) error { return m.Normalize() }

// ValidateModelEndpoint normalizes and validates a ModelEndpoint resource.
func ValidateModelEndpoint(m *ModelEndpoint) error { return m.Normalize() }

// ValidateMemory normalizes and validates a Memory resource.
func ValidateMemory(m *Memory) error { return m.Normalize() }

// ValidateAgentPolicy normalizes and validates an AgentPolicy resource.
func ValidateAgentPolicy(p *AgentPolicy) error { return p.Normalize() }

// ValidateSecret normalizes and validates a Secret resource.
func ValidateSecret(s *Secret) error { return s.Normalize() }
