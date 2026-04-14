package resources_test

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

func TestParseSecretLiteralBlockKubeconfig(t *testing.T) {
	manifest := `apiVersion: orloj.dev/v1
kind: Secret
metadata:
  name: k8s-kubeconfig
  namespace: test
spec:
  stringData:
    value: |
      apiVersion: v1
      clusters:
      - cluster:
          server: https://1.2.3.4:6443
        name: mycluster
      kind: Config
      current-context: mycluster
`
	secret, err := resources.ParseSecretManifest([]byte(manifest))
	if err != nil {
		t.Fatalf("unexpected parse error: %v", err)
	}

	// Normalize base64-encodes stringData into Data and nils StringData.
	if secret.Kind != "Secret" {
		t.Errorf("expected Kind=Secret, got %q (embedded kubeconfig kind: Config should not overwrite)", secret.Kind)
	}

	// The kubeconfig value should be stored in Data["value"] as base64.
	encoded, ok := secret.Spec.Data["value"]
	if !ok {
		t.Fatalf("expected Data[\"value\"] to be present; keys: %v", keysOf(secret.Spec.Data))
	}
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("Data[\"value\"] is not valid base64: %v", err)
	}
	if !strings.Contains(string(decoded), "kind: Config") {
		t.Errorf("decoded value should contain kubeconfig content, got %q", string(decoded))
	}
	if !strings.Contains(string(decoded), "server: https://1.2.3.4:6443") {
		t.Errorf("decoded value missing server URL, got %q", string(decoded))
	}

	// The kubeconfig sub-keys (apiVersion, kind, clusters…) must NOT appear as
	// separate secret entries — they are content of the literal block, not
	// independent stringData keys.
	if _, bad := secret.Spec.Data["kind"]; bad {
		t.Error("kubeconfig 'kind: Config' leaked into Data as a separate key")
	}
	if _, bad := secret.Spec.Data["apiVersion"]; bad {
		t.Error("kubeconfig 'apiVersion: v1' leaked into Data as a separate key")
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
