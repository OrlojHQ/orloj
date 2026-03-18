package resources

import (
	"fmt"
	"strings"
)

const (
	MemoryOperationRead   = "read"
	MemoryOperationWrite  = "write"
	MemoryOperationSearch = "search"
	MemoryOperationList   = "list"
	MemoryOperationIngest = "ingest"
)

var memoryOperationToToolName = map[string]string{
	MemoryOperationRead:   "memory.read",
	MemoryOperationWrite:  "memory.write",
	MemoryOperationSearch: "memory.search",
	MemoryOperationList:   "memory.list",
	MemoryOperationIngest: "memory.ingest",
}

// NormalizeMemoryOperation normalizes operation names and accepts either
// short names like "read" or legacy-style tool names like "memory.read".
func NormalizeMemoryOperation(raw string) (string, bool) {
	value := strings.ToLower(strings.TrimSpace(raw))
	switch value {
	case MemoryOperationRead, memoryOperationToToolName[MemoryOperationRead]:
		return MemoryOperationRead, true
	case MemoryOperationWrite, memoryOperationToToolName[MemoryOperationWrite]:
		return MemoryOperationWrite, true
	case MemoryOperationSearch, memoryOperationToToolName[MemoryOperationSearch]:
		return MemoryOperationSearch, true
	case MemoryOperationList, memoryOperationToToolName[MemoryOperationList]:
		return MemoryOperationList, true
	case MemoryOperationIngest, memoryOperationToToolName[MemoryOperationIngest]:
		return MemoryOperationIngest, true
	default:
		return "", false
	}
}

// NormalizeMemoryOperations deduplicates and validates memory operations.
func NormalizeMemoryOperations(raw []string) ([]string, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	seen := make(map[string]struct{}, len(raw))
	out := make([]string, 0, len(raw))
	for _, item := range raw {
		op, ok := NormalizeMemoryOperation(item)
		if !ok {
			return nil, fmt.Errorf("invalid memory operation %q: expected read, write, search, list, or ingest", strings.TrimSpace(item))
		}
		if _, exists := seen[op]; exists {
			continue
		}
		seen[op] = struct{}{}
		out = append(out, op)
	}
	return out, nil
}

// MemoryToolNamesForOperations returns built-in memory tool names for the
// normalized operation set.
func MemoryToolNamesForOperations(ops []string) []string {
	normalized, err := NormalizeMemoryOperations(ops)
	if err != nil || len(normalized) == 0 {
		return nil
	}
	out := make([]string, 0, len(normalized))
	for _, op := range normalized {
		if tool, ok := memoryOperationToToolName[op]; ok {
			out = append(out, tool)
		}
	}
	return out
}
