package resources

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

var legacyAgentModelPattern = regexp.MustCompile(`(?m)^\s{2}model:\s*\S+`) //nolint:gochecknoglobals

func TestAgentManifestSamplesDoNotUseLegacyModelField(t *testing.T) {
	roots := []string{
		filepath.Clean("../examples"),
		filepath.Clean("../testing/scenarios"),
		filepath.Clean("../testing/scenarios-real"),
	}
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() {
				return nil
			}
			name := strings.ToLower(d.Name())
			if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
				return nil
			}
			raw, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			content := string(raw)
			if !strings.Contains(content, "\nkind: Agent\n") {
				return nil
			}
			if legacyAgentModelPattern.MatchString(content) {
				t.Fatalf("legacy Agent field spec.model found in %s", path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("scan manifests in %s: %v", root, err)
		}
	}
}
