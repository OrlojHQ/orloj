package agentruntime

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// registryCredentials holds a temporary Docker config directory that
// contains registry authentication for private image pulls.
type registryCredentials struct {
	configDir string
}

// DockerConfigEnv returns the DOCKER_CONFIG environment variable assignment
// pointing to the temporary config directory.
func (c *registryCredentials) DockerConfigEnv() string {
	if c == nil || c.configDir == "" {
		return ""
	}
	return "DOCKER_CONFIG=" + c.configDir
}

// Cleanup removes the temporary config directory.
func (c *registryCredentials) Cleanup() {
	if c == nil || c.configDir == "" {
		return
	}
	_ = os.RemoveAll(c.configDir)
}

// resolveRegistryAuth resolves an image pull secret into a temporary Docker
// config directory. The Secret can contain either:
//
//   - A ".dockerconfigjson" key with a complete Docker config JSON blob
//   - Structured "registry", "username", "password" keys
//
// The caller must call Cleanup() on the returned credentials when done.
func resolveRegistryAuth(ctx context.Context, resolver SecretResolver, secretRef string) (*registryCredentials, error) {
	if resolver == nil {
		return nil, fmt.Errorf("no secret resolver available for image_pull_secret %q", secretRef)
	}

	dockerConfig, err := resolver.Resolve(ctx, secretRef+":.dockerconfigjson")
	if err == nil && dockerConfig != "" {
		return writeDockerConfig([]byte(dockerConfig))
	}

	registry, err := resolver.Resolve(ctx, secretRef+":registry")
	if err != nil {
		return nil, fmt.Errorf("image_pull_secret %q: missing .dockerconfigjson or registry key: %w", secretRef, err)
	}
	username, err := resolver.Resolve(ctx, secretRef+":username")
	if err != nil {
		return nil, fmt.Errorf("image_pull_secret %q: missing username key: %w", secretRef, err)
	}
	password, err := resolver.Resolve(ctx, secretRef+":password")
	if err != nil {
		return nil, fmt.Errorf("image_pull_secret %q: missing password key: %w", secretRef, err)
	}

	authToken := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	configJSON := dockerConfigJSON{
		Auths: map[string]dockerAuthEntry{
			registry: {Auth: authToken},
		},
	}
	data, err := json.Marshal(configJSON)
	if err != nil {
		return nil, fmt.Errorf("image_pull_secret %q: failed to build config.json: %w", secretRef, err)
	}
	return writeDockerConfig(data)
}

type dockerConfigJSON struct {
	Auths map[string]dockerAuthEntry `json:"auths"`
}

type dockerAuthEntry struct {
	Auth string `json:"auth"`
}

func writeDockerConfig(data []byte) (*registryCredentials, error) {
	tmpDir, err := os.MkdirTemp("", "orloj-registry-*")
	if err != nil {
		return nil, fmt.Errorf("failed to create temp dir for registry auth: %w", err)
	}
	configPath := filepath.Join(tmpDir, "config.json")
	if err := os.WriteFile(configPath, data, 0600); err != nil {
		_ = os.RemoveAll(tmpDir)
		return nil, fmt.Errorf("failed to write registry config.json: %w", err)
	}
	return &registryCredentials{configDir: tmpDir}, nil
}
