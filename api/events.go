package api

import (
	"strings"

	"github.com/OrlojHQ/orloj/crds"
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
	case crds.Agent:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.AgentSystem:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.ModelEndpoint:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.Tool:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.Secret:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.Memory:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.AgentPolicy:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.AgentRole:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.ToolPermission:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.Task:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.TaskSchedule:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.TaskWebhook:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case crds.Worker:
		return crds.NormalizeNamespace(obj.Metadata.Namespace)
	case map[string]any:
		metaRaw, ok := obj["metadata"]
		if !ok {
			return ""
		}
		switch meta := metaRaw.(type) {
		case map[string]string:
			return crds.NormalizeNamespace(meta["namespace"])
		case map[string]any:
			if ns, ok := meta["namespace"].(string); ok {
				return crds.NormalizeNamespace(ns)
			}
		}
	}
	return ""
}
