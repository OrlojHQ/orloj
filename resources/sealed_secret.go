package resources

import (
	"encoding/base64"
	"fmt"
	"strings"
)

const (
	SealedSecretOwnerAnnotation = "orloj.dev/sealedsecret-owner"
	SealingAlgorithm            = "rsa-oaep-sha256+aes-256-gcm"
)

type SealedSecret struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Metadata   ObjectMeta         `json:"metadata"`
	Spec       SealedSecretSpec   `json:"spec"`
	Status     SealedSecretStatus `json:"status,omitempty"`
}

type SealedSecretSpec struct {
	EncryptedData map[string]SealedValue     `json:"encryptedData,omitempty"`
	Template      SealedSecretTemplateSecret `json:"template,omitempty"`
}

type SealedValue struct {
	KeyID      string `json:"keyId,omitempty"`
	WrappedKey string `json:"wrappedKey,omitempty"`
	Ciphertext string `json:"ciphertext,omitempty"`
}

type SealedSecretTemplateSecret struct {
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type SealedSecretStatus struct {
	Phase              string `json:"phase,omitempty"`
	LastError          string `json:"lastError,omitempty"`
	ObservedGeneration int64  `json:"observedGeneration,omitempty"`
}

type SealedSecretList struct {
	ListMeta `json:",inline"`
	Items    []SealedSecret `json:"items"`
}

func (s *SealedSecret) Normalize() error {
	if s.APIVersion == "" {
		s.APIVersion = "orloj.dev/v1"
	}
	if s.Kind == "" {
		s.Kind = "SealedSecret"
	}
	if !strings.EqualFold(s.Kind, "SealedSecret") {
		return fmt.Errorf("unsupported kind %q for SealedSecret", s.Kind)
	}
	NormalizeObjectMetaNamespace(&s.Metadata)
	s.Metadata.Name = strings.TrimSpace(s.Metadata.Name)
	if err := ValidateMetadataName(s.Metadata.Name); err != nil {
		return err
	}

	if s.Spec.EncryptedData == nil {
		s.Spec.EncryptedData = make(map[string]SealedValue)
	}
	normalizedData := make(map[string]SealedValue, len(s.Spec.EncryptedData))
	for key, value := range s.Spec.EncryptedData {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		value.KeyID = strings.TrimSpace(value.KeyID)
		value.WrappedKey = strings.TrimSpace(value.WrappedKey)
		value.Ciphertext = strings.TrimSpace(value.Ciphertext)
		if value.KeyID == "" {
			return fmt.Errorf("spec.encryptedData.%s.keyId is required", key)
		}
		if value.WrappedKey == "" {
			return fmt.Errorf("spec.encryptedData.%s.wrappedKey is required", key)
		}
		if value.Ciphertext == "" {
			return fmt.Errorf("spec.encryptedData.%s.ciphertext is required", key)
		}
		if _, err := base64.StdEncoding.DecodeString(value.WrappedKey); err != nil {
			return fmt.Errorf("spec.encryptedData.%s.wrappedKey must be valid base64: %w", key, err)
		}
		if _, err := base64.StdEncoding.DecodeString(value.Ciphertext); err != nil {
			return fmt.Errorf("spec.encryptedData.%s.ciphertext must be valid base64: %w", key, err)
		}
		normalizedData[key] = value
	}
	s.Spec.EncryptedData = normalizedData
	s.Spec.Template.Labels = trimStringMap(s.Spec.Template.Labels)
	s.Spec.Template.Annotations = trimStringMap(s.Spec.Template.Annotations)
	if s.Status.Phase == "" {
		s.Status.Phase = "Pending"
	}
	return nil
}

func trimStringMap(in map[string]string) map[string]string {
	if len(in) == 0 {
		return nil
	}
	out := make(map[string]string, len(in))
	for key, value := range in {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		out[key] = strings.TrimSpace(value)
	}
	if len(out) == 0 {
		return nil
	}
	return out
}
