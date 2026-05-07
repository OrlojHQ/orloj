package resources

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// ParseMemoryBytes converts a memory string to bytes.
// Accepted suffixes (case-insensitive): gi, mi, ki (IEC binary),
// g, m, k (Docker-style, also 1024-based), b (bytes).
// A bare integer is treated as bytes.
func ParseMemoryBytes(s string) (int64, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" {
		return 0, fmt.Errorf("empty memory value")
	}
	multiplier := int64(1)
	switch {
	case strings.HasSuffix(s, "gi"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "gi")
	case strings.HasSuffix(s, "mi"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "mi")
	case strings.HasSuffix(s, "ki"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "ki")
	case strings.HasSuffix(s, "g"):
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "g")
	case strings.HasSuffix(s, "m"):
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "m")
	case strings.HasSuffix(s, "k"):
		multiplier = 1024
		s = strings.TrimSuffix(s, "k")
	case strings.HasSuffix(s, "b"):
		s = strings.TrimSuffix(s, "b")
	}
	v, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return 0, fmt.Errorf("invalid memory value %q: %w", s, err)
	}
	if v <= 0 {
		return 0, fmt.Errorf("memory value must be positive, got %d", v)
	}
	if multiplier > 1 && v > math.MaxInt64/multiplier {
		return 0, fmt.Errorf("memory value overflows int64")
	}
	return v * multiplier, nil
}

// ValidateContainerResources checks that the fields of a ContainerResources
// struct are well-formed when set.
func ValidateContainerResources(res ContainerResources, fieldPrefix string) error {
	if mem := strings.TrimSpace(res.Memory); mem != "" {
		if _, err := ParseMemoryBytes(mem); err != nil {
			return fmt.Errorf("%s.memory: %w", fieldPrefix, err)
		}
	}
	if cpus := strings.TrimSpace(res.CPUs); cpus != "" {
		v, err := strconv.ParseFloat(cpus, 64)
		if err != nil {
			return fmt.Errorf("%s.cpus: invalid value %q: %w", fieldPrefix, cpus, err)
		}
		if v <= 0 {
			return fmt.Errorf("%s.cpus: must be positive, got %s", fieldPrefix, cpus)
		}
	}
	if res.PidsLimit < 0 {
		return fmt.Errorf("%s.pids_limit: must be non-negative, got %d", fieldPrefix, res.PidsLimit)
	}
	return nil
}

// ContainerResourceCeiling defines operator-level upper bounds for per-tool
// and per-McpServer container resource overrides. Zero/empty means unbounded.
type ContainerResourceCeiling struct {
	MaxMemory   string
	MaxCPUs     string
	MaxPidsLimit int
}

// EnforceContainerResourceCeiling checks that the given resources do not exceed
// the operator ceiling. Returns a descriptive error when exceeded.
func EnforceContainerResourceCeiling(res ContainerResources, ceiling ContainerResourceCeiling, resourceKind, resourceName string) error {
	if mem := strings.TrimSpace(res.Memory); mem != "" && strings.TrimSpace(ceiling.MaxMemory) != "" {
		requested, err := ParseMemoryBytes(mem)
		if err != nil {
			return fmt.Errorf("%s %q resources.memory: %w", resourceKind, resourceName, err)
		}
		max, err := ParseMemoryBytes(ceiling.MaxMemory)
		if err != nil {
			return fmt.Errorf("operator ceiling max-memory %q: %w", ceiling.MaxMemory, err)
		}
		if requested > max {
			return fmt.Errorf("%s %q resources.memory (%s) exceeds operator limit (%s)", resourceKind, resourceName, mem, ceiling.MaxMemory)
		}
	}
	if cpus := strings.TrimSpace(res.CPUs); cpus != "" && strings.TrimSpace(ceiling.MaxCPUs) != "" {
		requested, err := strconv.ParseFloat(cpus, 64)
		if err != nil {
			return fmt.Errorf("%s %q resources.cpus: invalid value %q", resourceKind, resourceName, cpus)
		}
		max, err := strconv.ParseFloat(strings.TrimSpace(ceiling.MaxCPUs), 64)
		if err != nil {
			return fmt.Errorf("operator ceiling max-cpus %q: invalid", ceiling.MaxCPUs)
		}
		if requested > max {
			return fmt.Errorf("%s %q resources.cpus (%s) exceeds operator limit (%s)", resourceKind, resourceName, cpus, ceiling.MaxCPUs)
		}
	}
	if res.PidsLimit > 0 && ceiling.MaxPidsLimit > 0 {
		if res.PidsLimit > ceiling.MaxPidsLimit {
			return fmt.Errorf("%s %q resources.pids_limit (%d) exceeds operator limit (%d)", resourceKind, resourceName, res.PidsLimit, ceiling.MaxPidsLimit)
		}
	}
	return nil
}
