package agentruntime

import (
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// ToolApprovalScopedStoreKey returns the same lookup key used for ToolApproval
// resources created by pauseTaskForToolApproval (namespace/name).
func ToolApprovalScopedStoreKey(taskKey, messageID string) string {
	ns, _ := splitTaskKeyForToolApproval(taskKey)
	h := fnv.New32a()
	_, _ = h.Write([]byte(taskKey))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strings.TrimSpace(messageID)))
	name := fmt.Sprintf("ta-%08x", h.Sum32())
	return resources.NormalizeNamespace(ns) + "/" + strings.TrimSpace(name)
}

func splitTaskKeyForToolApproval(taskKey string) (namespace, taskName string) {
	taskKey = strings.TrimSpace(taskKey)
	if taskKey == "" {
		return resources.DefaultNamespace, ""
	}
	if strings.Contains(taskKey, "/") {
		parts := strings.SplitN(taskKey, "/", 2)
		return resources.NormalizeNamespace(parts[0]), strings.TrimSpace(parts[1])
	}
	return resources.DefaultNamespace, taskKey
}

func toolApprovalResourceName(taskKey, messageID string) string {
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(taskKey)))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(strings.TrimSpace(messageID)))
	return fmt.Sprintf("ta-%08x", h.Sum32())
}
