package resources

import (
	"encoding/json"
	"fmt"
	"strings"
)

func ParseSealedSecretManifest(data []byte) (SealedSecret, error) {
	if err := rejectMultiDocumentYAML(data); err != nil {
		return SealedSecret{}, err
	}
	var out SealedSecret
	if json.Valid(data) {
		if err := json.Unmarshal(data, &out); err != nil {
			return SealedSecret{}, fmt.Errorf("failed to decode JSON manifest: %w", err)
		}
		if err := out.Normalize(); err != nil {
			return SealedSecret{}, err
		}
		return out, nil
	}

	lines := strings.Split(strings.ReplaceAll(string(data), "\r\n", "\n"), "\n")
	section := ""
	subsection := ""
	currentEncryptedKey := ""
	currentTemplateMap := ""

	resetNested := func(indent int, trimmed string) {
		if section == "metadata" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
		}
		if section == "spec" && indent <= 2 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			subsection = ""
			currentEncryptedKey = ""
			currentTemplateMap = ""
		}
		if section == "spec" && subsection == "encryptedData" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			currentEncryptedKey = ""
		}
		if section == "spec" && subsection == "template" && indent <= 4 && !strings.HasSuffix(trimmed, ":") && !strings.HasPrefix(trimmed, "- ") {
			currentTemplateMap = ""
		}
	}

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		indent := leadingSpaces(line)
		resetNested(indent, trimmed)

		if strings.HasSuffix(trimmed, ":") {
			key := strings.TrimSuffix(trimmed, ":")
			switch {
			case indent == 0 && key == "metadata":
				section = "metadata"
				subsection = ""
			case indent == 0 && key == "spec":
				section = "spec"
				subsection = ""
			case section == "metadata" && indent == 2 && key == "labels":
				subsection = "labels"
			case section == "metadata" && indent == 2 && key == "annotations":
				subsection = "annotations"
			case section == "spec" && indent == 2 && key == "encryptedData":
				subsection = "encryptedData"
				currentEncryptedKey = ""
			case section == "spec" && indent == 2 && key == "template":
				subsection = "template"
				currentTemplateMap = ""
			case section == "spec" && subsection == "encryptedData" && indent == 4:
				currentEncryptedKey = key
				if out.Spec.EncryptedData == nil {
					out.Spec.EncryptedData = make(map[string]SealedValue)
				}
				out.Spec.EncryptedData[currentEncryptedKey] = SealedValue{}
			case section == "spec" && subsection == "template" && indent == 4 && (key == "labels" || key == "annotations"):
				currentTemplateMap = key
			}
			continue
		}

		key, value, ok := parseKeyValue(trimmed)
		if !ok {
			continue
		}
		value = stripQuotes(value)
		switch {
		case indent == 0 && key == "apiVersion":
			out.APIVersion = value
		case indent == 0 && key == "kind":
			out.Kind = value
		case section == "metadata" && subsection == "labels" && indent >= 4:
			if out.Metadata.Labels == nil {
				out.Metadata.Labels = make(map[string]string)
			}
			out.Metadata.Labels[key] = value
		case section == "metadata" && subsection == "annotations" && indent >= 4:
			if out.Metadata.Annotations == nil {
				out.Metadata.Annotations = make(map[string]string)
			}
			out.Metadata.Annotations[key] = value
		case section == "metadata":
			if err := applyObjectMetaField(&out.Metadata, key, value); err != nil {
				return SealedSecret{}, err
			}
		case section == "spec" && subsection == "encryptedData" && currentEncryptedKey != "" && indent >= 6:
			current := out.Spec.EncryptedData[currentEncryptedKey]
			switch key {
			case "keyId", "key_id":
				current.KeyID = value
			case "wrappedKey", "wrapped_key":
				current.WrappedKey = value
			case "ciphertext":
				current.Ciphertext = value
			}
			out.Spec.EncryptedData[currentEncryptedKey] = current
		case section == "spec" && subsection == "template" && currentTemplateMap == "labels" && indent >= 6:
			if out.Spec.Template.Labels == nil {
				out.Spec.Template.Labels = make(map[string]string)
			}
			out.Spec.Template.Labels[key] = value
		case section == "spec" && subsection == "template" && currentTemplateMap == "annotations" && indent >= 6:
			if out.Spec.Template.Annotations == nil {
				out.Spec.Template.Annotations = make(map[string]string)
			}
			out.Spec.Template.Annotations[key] = value
		}
	}

	if err := out.Normalize(); err != nil {
		return SealedSecret{}, err
	}
	return out, nil
}
