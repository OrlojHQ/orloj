package agentruntime

import (
	"context"
	"time"
)

// MeteringEvent captures one normalized usage event for optional billing/usage sinks.
type MeteringEvent struct {
	Timestamp       string            `json:"timestamp"`
	Component       string            `json:"component,omitempty"`
	Type            string            `json:"type"`
	Namespace       string            `json:"namespace,omitempty"`
	Task            string            `json:"task,omitempty"`
	System          string            `json:"system,omitempty"`
	Agent           string            `json:"agent,omitempty"`
	Worker          string            `json:"worker,omitempty"`
	Attempt         int               `json:"attempt,omitempty"`
	MessageID       string            `json:"message_id,omitempty"`
	Status          string            `json:"status,omitempty"`
	TokensUsed      int               `json:"tokens_used,omitempty"`
	TokensEstimated int               `json:"tokens_estimated,omitempty"`
	ToolCalls       int               `json:"tool_calls,omitempty"`
	Metadata        map[string]string `json:"metadata,omitempty"`
}

// AuditEvent captures one normalized audit event for optional external audit sinks.
type AuditEvent struct {
	Timestamp    string            `json:"timestamp"`
	Component    string            `json:"component,omitempty"`
	Action       string            `json:"action"`
	Outcome      string            `json:"outcome"`
	Namespace    string            `json:"namespace,omitempty"`
	ResourceKind string            `json:"resource_kind,omitempty"`
	ResourceName string            `json:"resource_name,omitempty"`
	Principal    string            `json:"principal,omitempty"`
	Message      string            `json:"message,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// Capability describes one discoverable feature exposed by the current runtime.
type Capability struct {
	ID          string `json:"id"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
	Source      string `json:"source,omitempty"`
}

// CapabilitySnapshot returns the effective capability set for this deployment.
type CapabilitySnapshot struct {
	GeneratedAt  string       `json:"generated_at"`
	Capabilities []Capability `json:"capabilities"`
}

// MeteringSink receives metering events. Implementations should be non-blocking and resilient.
type MeteringSink interface {
	RecordMetering(ctx context.Context, event MeteringEvent)
}

// AuditSink receives audit events. Implementations should be non-blocking and resilient.
type AuditSink interface {
	RecordAudit(ctx context.Context, event AuditEvent)
}

// CapabilityProvider returns deployment capabilities for API/UI/CLI feature discovery.
type CapabilityProvider interface {
	Capabilities(ctx context.Context) CapabilitySnapshot
}

// Extensions groups optional runtime hooks for commercial/add-on integrations.
type Extensions struct {
	Metering     MeteringSink
	Audit        AuditSink
	Capabilities CapabilityProvider
}

type noopMeteringSink struct{}

func (noopMeteringSink) RecordMetering(_ context.Context, _ MeteringEvent) {}

type noopAuditSink struct{}

func (noopAuditSink) RecordAudit(_ context.Context, _ AuditEvent) {}

type defaultCapabilityProvider struct{}

func (defaultCapabilityProvider) Capabilities(_ context.Context) CapabilitySnapshot {
	return CapabilitySnapshot{
		GeneratedAt: time.Now().UTC().Format(time.RFC3339Nano),
		Capabilities: []Capability{
			{ID: "core.api.crud", Enabled: true, Description: "Resource CRUD/status/watch APIs", Source: "oss"},
			{ID: "core.runtime.sequential", Enabled: true, Description: "Sequential task execution mode", Source: "oss"},
			{ID: "core.runtime.message_driven", Enabled: true, Description: "Message-driven execution mode", Source: "oss"},
			{ID: "core.governance.agent_roles", Enabled: true, Description: "AgentRole and ToolPermission governance", Source: "oss"},
			{ID: "core.ui.baseline", Enabled: true, Description: "Built-in baseline web console", Source: "oss"},
			{ID: "core.self_host", Enabled: true, Description: "Self-hostable OSS deployment", Source: "oss"},
		},
	}
}

// NormalizeExtensions applies safe defaults so callers can omit all optional hooks.
func NormalizeExtensions(ext Extensions) Extensions {
	if ext.Metering == nil {
		ext.Metering = noopMeteringSink{}
	}
	if ext.Audit == nil {
		ext.Audit = noopAuditSink{}
	}
	if ext.Capabilities == nil {
		ext.Capabilities = defaultCapabilityProvider{}
	}
	return ext
}

// DefaultExtensions returns OSS-safe defaults for all extension hooks.
func DefaultExtensions() Extensions {
	return NormalizeExtensions(Extensions{})
}
