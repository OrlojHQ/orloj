package agentruntime

import (
	"fmt"
	"strings"
	"unicode"
)

type providerToolAliases struct {
	RuntimeToProvider map[string]string
	ProviderToRuntime map[string]string
}

func buildProviderToolAliases(toolNames []string) providerToolAliases {
	deduped := dedupeStrings(toolNames)
	aliases := providerToolAliases{
		RuntimeToProvider: make(map[string]string, len(deduped)),
		ProviderToRuntime: make(map[string]string, len(deduped)),
	}
	used := make(map[string]struct{}, len(deduped))
	for _, name := range deduped {
		runtimeName := strings.TrimSpace(name)
		if runtimeName == "" {
			continue
		}
		providerName := sanitizeToolName(runtimeName, used)
		aliases.RuntimeToProvider[runtimeName] = providerName
		aliases.ProviderToRuntime[providerName] = runtimeName
	}
	return aliases
}

func providerToolNameForHistory(runtimeName, providerName string, aliases map[string]string) string {
	runtimeName = strings.TrimSpace(runtimeName)
	if runtimeName != "" && aliases != nil {
		if mapped := strings.TrimSpace(aliases[runtimeName]); mapped != "" {
			return mapped
		}
	}
	if providerName = strings.TrimSpace(providerName); providerName != "" {
		return providerName
	}
	if runtimeName == "" {
		return ""
	}
	used := map[string]struct{}{}
	return sanitizeToolName(runtimeName, used)
}

// sanitizeToolName maps an arbitrary tool name to one that is safe for
// providers with strict tool-name constraints (letters, digits, underscore,
// hyphen; max 128 chars). Collisions are resolved by appending _2, _3, …
// Callers must pass a shared `used` set across all tools in the same request.
func sanitizeToolName(name string, used map[string]struct{}) string {
	base := strings.TrimSpace(name)
	if base == "" {
		base = "tool"
	}
	var b strings.Builder
	b.Grow(len(base))
	lastUnderscore := false
	for _, r := range base {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			lastUnderscore = false
		case r == '_' || r == '-':
			b.WriteRune(r)
			lastUnderscore = false
		default:
			if !lastUnderscore {
				b.WriteByte('_')
				lastUnderscore = true
			}
		}
	}
	alias := strings.Trim(strings.TrimSpace(b.String()), "_-")
	if alias == "" {
		alias = "tool"
	}
	if len(alias) > 128 {
		alias = strings.TrimRight(alias[:128], "_-")
		if alias == "" {
			alias = "tool"
		}
	}
	candidate := alias
	for suffix := 2; ; suffix++ {
		if _, exists := used[strings.ToLower(candidate)]; !exists {
			used[strings.ToLower(candidate)] = struct{}{}
			return candidate
		}
		tag := fmt.Sprintf("_%d", suffix)
		trimmed := alias
		if len(trimmed)+len(tag) > 128 {
			trimmed = strings.TrimRight(trimmed[:128-len(tag)], "_-")
			if trimmed == "" {
				trimmed = "tool"
			}
		}
		candidate = trimmed + tag
	}
}
