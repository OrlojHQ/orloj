package resources

import (
	"fmt"
	"strings"
)

// APIRedactedSecretPlaceholder is the value the HTTP API uses for redacted secret fields in GET responses.
const APIRedactedSecretPlaceholder = "***"

func isRedactedSecretSentinel(v string) bool {
	return strings.TrimSpace(v) == APIRedactedSecretPlaceholder
}

// MergeRedactedSecretPlaceholders replaces API redaction sentinels in dst with values from current (the stored secret).
// Call before Normalize() on a PUT body that round-tripped through the UI or GET API.
func MergeRedactedSecretPlaceholders(dst *Secret, current Secret) error {
	if dst == nil {
		return fmt.Errorf("secret is nil")
	}
	if dst.Spec.Data == nil {
		dst.Spec.Data = make(map[string]string)
	}
	// Normalize applies stringData first; resolve stringData sentinels into data using current.
	if dst.Spec.StringData != nil {
		for k, v := range dst.Spec.StringData {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if !isRedactedSecretSentinel(v) {
				continue
			}
			cv, ok := current.Spec.Data[k]
			if !ok {
				return fmt.Errorf("spec.stringData.%s: redacted placeholder %q but existing secret has no such key", k, APIRedactedSecretPlaceholder)
			}
			dst.Spec.Data[k] = cv
			delete(dst.Spec.StringData, k)
		}
		if len(dst.Spec.StringData) == 0 {
			dst.Spec.StringData = nil
		}
	}
	for k, v := range dst.Spec.Data {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if !isRedactedSecretSentinel(v) {
			continue
		}
		cv, ok := current.Spec.Data[k]
		if !ok {
			return fmt.Errorf("spec.data.%s: redacted placeholder %q but existing secret has no such key", k, APIRedactedSecretPlaceholder)
		}
		dst.Spec.Data[k] = cv
	}
	return nil
}

// ParseSecretManifestForPut parses a Secret PUT body and merges API redaction placeholders using current stored values.
func ParseSecretManifestForPut(data []byte, current Secret) (Secret, error) {
	out, err := parseSecretManifestWithoutNormalize(data)
	if err != nil {
		return Secret{}, err
	}
	if err := MergeRedactedSecretPlaceholders(&out, current); err != nil {
		return Secret{}, err
	}
	if err := out.Normalize(); err != nil {
		return Secret{}, err
	}
	return out, nil
}
