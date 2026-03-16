package agentruntime

import (
	"regexp"
	"strings"
)

var sensitivePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)(Bearer\s+)\S+`),
	regexp.MustCompile(`(?i)(Authorization:\s*)\S+`),
	regexp.MustCompile(`(?i)(X-Api-Key:\s*)\S+`),
	regexp.MustCompile(`(?i)(api[_-]?key\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(secret\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(password\s*[=:]\s*)\S+`),
	regexp.MustCompile(`(?i)(token\s*[=:]\s*)\S+`),
	regexp.MustCompile(`sk-[A-Za-z0-9_-]{10,}`),
}

const redactedPlaceholder = "[REDACTED]"

// RedactSensitive replaces known sensitive patterns (auth headers, API keys,
// tokens) in s with a redacted placeholder. This is applied as defense-in-depth
// before including process output (stderr, error messages) in logs or traces.
func RedactSensitive(s string) string {
	if s == "" {
		return s
	}
	for _, pat := range sensitivePatterns {
		if pat.NumSubexp() > 0 {
			s = pat.ReplaceAllString(s, "${1}"+redactedPlaceholder)
		} else {
			s = pat.ReplaceAllString(s, redactedPlaceholder)
		}
	}
	return strings.TrimSpace(s)
}
