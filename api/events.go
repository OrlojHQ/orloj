package api

import (
	"strings"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/eventbus"
)

func (s *Server) publishResourceEvent(kind, name, action string, resource any) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(eventbus.Event{
		Source:    "apiserver",
		Type:      "resource." + strings.ToLower(strings.TrimSpace(action)),
		Kind:      strings.TrimSpace(kind),
		Name:      strings.TrimSpace(name),
		Namespace: extractResourceNamespace(resource),
		Action:    strings.ToLower(strings.TrimSpace(action)),
		Data:      resource,
	})
}

func extractResourceNamespace(resource any) string {
	switch obj := resource.(type) {
	case resources.Agent:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.AgentSystem:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.ModelEndpoint:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.Tool:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.Secret:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.Memory:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.AgentPolicy:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.AgentRole:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.ToolPermission:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.Task:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.TaskSchedule:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.TaskWebhook:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case resources.Worker:
		return resources.NormalizeNamespace(obj.Metadata.Namespace)
	case map[string]any:
		metaRaw, ok := obj["metadata"]
		if !ok {
			return ""
		}
		switch meta := metaRaw.(type) {
		case map[string]string:
			return resources.NormalizeNamespace(meta["namespace"])
		case map[string]any:
			if ns, ok := meta["namespace"].(string); ok {
				return resources.NormalizeNamespace(ns)
			}
		}
	}
	return ""
}
