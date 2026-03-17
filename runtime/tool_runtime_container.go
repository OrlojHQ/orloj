package agentruntime

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"

	"github.com/OrlojHQ/orloj/crds"
)

// ContainerToolRuntimeConfig defines isolated tool execution in a locked-down container.
type ContainerToolRuntimeConfig struct {
	RuntimeBinary string
	Image         string
	Network       string
	Memory        string
	CPUs          string
	PidsLimit     int
	User          string
	Shell         string
}

var ErrToolSecretResolution = errors.New("tool secret resolution failed")

// SecretResolver resolves tool auth secret references.
type SecretResolver interface {
	Resolve(ctx context.Context, secretRef string) (string, error)
}

// EnvSecretResolver resolves secret refs from worker environment variables.
type EnvSecretResolver struct {
	Prefix string
}

func NewEnvSecretResolver(prefix string) *EnvSecretResolver {
	return &EnvSecretResolver{Prefix: prefix}
}

func (r *EnvSecretResolver) WithNamespace(_ string) SecretResolver {
	return r
}

func (r *EnvSecretResolver) Resolve(_ context.Context, secretRef string) (string, error) {
	secretRef = strings.TrimSpace(secretRef)
	if secretRef == "" {
		return "", fmt.Errorf("secretRef is required")
	}
	keys := make([]string, 0, 3)
	keys = append(keys, secretRef)
	normalized := normalizeEnvKey(secretRef)
	if normalized != "" && !strings.EqualFold(normalized, secretRef) {
		keys = append(keys, normalized)
	}
	prefix := strings.TrimSpace(r.Prefix)
	if prefix != "" && normalized != "" {
		keys = append(keys, prefix+normalized)
	}
	for _, key := range dedupeStrings(keys) {
		if value, ok := os.LookupEnv(key); ok {
			value = strings.TrimSpace(value)
			if value == "" {
				return "", fmt.Errorf("secret %q resolved from env var %q but value is empty", secretRef, key)
			}
			return value, nil
		}
	}
	return "", fmt.Errorf("%w: secret %q not found in environment", ErrToolSecretNotFound, secretRef)
}

func DefaultContainerToolRuntimeConfig() ContainerToolRuntimeConfig {
	return ContainerToolRuntimeConfig{
		RuntimeBinary: "docker",
		Image:         "curlimages/curl:8.8.0",
		Network:       "none",
		Memory:        "128m",
		CPUs:          "0.50",
		PidsLimit:     64,
		User:          "65532:65532",
		Shell:         "/bin/sh",
	}
}

// SandboxedContainerDefaults returns secure-by-default container settings
// for tools running in sandboxed isolation mode. These enforce:
//   - network=none (no network access)
//   - memory=128m (128 MB ceiling)
//   - cpus=0.50 (half a core)
//   - pids_limit=64 (process limit)
//   - user=65532:65532 (non-root nobody user)
//   - read-only filesystem (via containerRunArgs --read-only)
//   - no Linux capabilities (via --cap-drop=ALL)
//   - no privilege escalation (via --security-opt no-new-privileges)
//
// These defaults match DefaultContainerToolRuntimeConfig but are preserved
// as an explicit contract so callers can distinguish between default and
// sandboxed modes.
func SandboxedContainerDefaults() ContainerToolRuntimeConfig {
	return DefaultContainerToolRuntimeConfig()
}

func (c ContainerToolRuntimeConfig) normalized() ContainerToolRuntimeConfig {
	out := c
	defaults := DefaultContainerToolRuntimeConfig()
	if strings.TrimSpace(out.RuntimeBinary) == "" {
		out.RuntimeBinary = defaults.RuntimeBinary
	}
	if strings.TrimSpace(out.Image) == "" {
		out.Image = defaults.Image
	}
	if strings.TrimSpace(out.Network) == "" {
		out.Network = defaults.Network
	}
	if strings.TrimSpace(out.Memory) == "" {
		out.Memory = defaults.Memory
	}
	if strings.TrimSpace(out.CPUs) == "" {
		out.CPUs = defaults.CPUs
	}
	if out.PidsLimit <= 0 {
		out.PidsLimit = defaults.PidsLimit
	}
	if strings.TrimSpace(out.User) == "" {
		out.User = defaults.User
	}
	if strings.TrimSpace(out.Shell) == "" {
		out.Shell = defaults.Shell
	}
	return out
}

// ContainerCommandRunner executes container runtime commands.
type ContainerCommandRunner interface {
	Run(ctx context.Context, binary string, args []string, stdin string, env map[string]string) (stdout string, stderr string, err error)
}

type osExecContainerCommandRunner struct{}

func (r *osExecContainerCommandRunner) Run(ctx context.Context, binary string, args []string, stdin string, env map[string]string) (string, string, error) {
	cmd := exec.CommandContext(ctx, binary, args...) //nolint:gosec
	cmd.Stdin = strings.NewReader(stdin)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if len(env) > 0 {
		cmd.Env = append(os.Environ(), mapToEnv(env)...)
	}
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// ContainerToolRuntime executes tools inside a containerized sandbox.
type ContainerToolRuntime struct {
	registry  ToolCapabilityRegistry
	secrets   SecretResolver
	runner    ContainerCommandRunner
	config    ContainerToolRuntimeConfig
	namespace string
}

func NewContainerToolRuntime(registry ToolCapabilityRegistry, config ContainerToolRuntimeConfig) *ContainerToolRuntime {
	return NewContainerToolRuntimeWithRunnerAndSecrets(
		registry,
		config,
		&osExecContainerCommandRunner{},
		NewEnvSecretResolver("ORLOJ_SECRET_"),
	)
}

func NewContainerToolRuntimeWithRunner(
	registry ToolCapabilityRegistry,
	config ContainerToolRuntimeConfig,
	runner ContainerCommandRunner,
) *ContainerToolRuntime {
	return NewContainerToolRuntimeWithRunnerAndSecrets(
		registry,
		config,
		runner,
		NewEnvSecretResolver("ORLOJ_SECRET_"),
	)
}

func NewContainerToolRuntimeWithRunnerAndSecrets(
	registry ToolCapabilityRegistry,
	config ContainerToolRuntimeConfig,
	runner ContainerCommandRunner,
	secrets SecretResolver,
) *ContainerToolRuntime {
	if runner == nil {
		runner = &osExecContainerCommandRunner{}
	}
	if secrets == nil {
		secrets = NewEnvSecretResolver("ORLOJ_SECRET_")
	}
	return &ContainerToolRuntime{
		registry: registry,
		secrets:  secrets,
		runner:   runner,
		config:   config.normalized(),
	}
}

func (r *ContainerToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewContainerToolRuntime(registry, DefaultContainerToolRuntimeConfig())
	}
	return &ContainerToolRuntime{
		registry:  registry,
		secrets:   r.secrets,
		runner:    r.runner,
		config:    r.config,
		namespace: r.namespace,
	}
}

func (r *ContainerToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewContainerToolRuntime(nil, DefaultContainerToolRuntimeConfig())
	}
	copy := *r
	copy.namespace = crds.NormalizeNamespace(strings.TrimSpace(namespace))
	if aware, ok := copy.secrets.(namespaceAwareSecretResolver); ok {
		copy.secrets = aware.WithNamespace(copy.namespace)
	}
	return &copy
}

func (r *ContainerToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeInvalidInput,
			ToolReasonInvalidInput,
			false,
			"missing tool name",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"field": "tool"},
		)
	}
	if r.registry == nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			"missing tool registry for isolated runtime",
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}
	spec, ok := r.registry.Resolve(tool)
	if !ok {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeUnsupportedTool,
			ToolReasonToolUnsupported,
			false,
			fmt.Sprintf("unsupported tool %s", tool),
			ErrUnsupportedTool,
			map[string]string{"tool": tool},
		)
	}

	switch strings.ToLower(strings.TrimSpace(spec.Type)) {
	case "", "http":
		return r.callHTTP(ctx, tool, spec, input)
	default:
		return "", NewToolError(
			ToolStatusError,
			ToolCodeUnsupportedTool,
			ToolReasonToolUnsupported,
			false,
			fmt.Sprintf("tool=%s type=%s unsupported by container isolation path", tool, spec.Type),
			ErrUnsupportedTool,
			map[string]string{
				"tool": tool,
				"type": strings.TrimSpace(spec.Type),
			},
		)
	}
}

func (r *ContainerToolRuntime) callHTTP(ctx context.Context, tool string, spec crds.ToolSpec, input string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", mapContainerContextError(tool, err)
	}
	endpoint := strings.TrimSpace(spec.Endpoint)
	if endpoint == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s missing endpoint", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}

	containerEnv := map[string]string{}
	args := r.containerRunArgs(endpoint, false)
	if strings.TrimSpace(spec.Auth.SecretRef) != "" {
		if r.secrets == nil {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeSecretResolution,
				ToolReasonSecretResolution,
				false,
				fmt.Sprintf("tool=%s has auth.secretRef but no secret resolver is configured", tool),
				ErrToolSecretResolution,
				map[string]string{"tool": tool},
			)
		}
		secretValue, err := r.secrets.Resolve(ctx, spec.Auth.SecretRef)
		if err != nil {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeSecretResolution,
				ToolReasonSecretResolution,
				false,
				fmt.Sprintf("tool=%s secretRef=%s resolution failed", tool, spec.Auth.SecretRef),
				fmt.Errorf("%w: %v", ErrToolSecretResolution, err),
				map[string]string{
					"tool":       tool,
					"secret_ref": strings.TrimSpace(spec.Auth.SecretRef),
				},
			)
		}
		containerEnv["TOOL_AUTH_BEARER"] = secretValue
		args = r.containerRunArgs(endpoint, true)
	}
	stdout, stderr, err := runContainerCommandBounded(ctx, r.runner, r.config.RuntimeBinary, args, input, containerEnv)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
			return "", mapContainerContextError(tool, err)
		}
		return "", NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("container sandbox execution failed for tool=%s stderr=%s", tool, RedactSensitive(compactStderr(stderr))),
			err,
			map[string]string{
				"tool":           tool,
				"runtime":        strings.TrimSpace(r.config.RuntimeBinary),
				"isolation_mode": "container",
			},
		)
	}
	return strings.TrimSpace(stdout), nil
}

func runContainerCommandBounded(
	ctx context.Context,
	runner ContainerCommandRunner,
	binary string,
	args []string,
	stdin string,
	env map[string]string,
) (string, string, error) {
	if runner == nil {
		return "", "", fmt.Errorf("missing container command runner")
	}
	type runResult struct {
		stdout string
		stderr string
		err    error
	}
	resultCh := make(chan runResult, 1)
	go func() {
		stdout, stderr, err := runner.Run(ctx, binary, args, stdin, env)
		resultCh <- runResult{
			stdout: stdout,
			stderr: stderr,
			err:    err,
		}
	}()
	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case result := <-resultCh:
		return result.stdout, result.stderr, result.err
	}
}

func mapContainerContextError(tool string, err error) error {
	tool = strings.TrimSpace(tool)
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolError(
			ToolStatusError,
			ToolCodeTimeout,
			ToolReasonExecutionTimeout,
			true,
			fmt.Sprintf("container tool execution timed out for tool=%s", tool),
			err,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "container",
			},
		)
	case errors.Is(err, context.Canceled):
		return NewToolError(
			ToolStatusError,
			ToolCodeCanceled,
			ToolReasonExecutionCanceled,
			false,
			fmt.Sprintf("container tool execution canceled for tool=%s", tool),
			err,
			map[string]string{
				"tool":           tool,
				"isolation_mode": "container",
			},
		)
	default:
		return err
	}
}

func (r *ContainerToolRuntime) containerRunArgs(endpoint string, includeAuth bool) []string {
	// Keep the container constrained: read-only fs, no extra Linux capabilities, no privilege escalation.
	args := []string{
		"run", "--rm", "-i",
		"--network", strings.TrimSpace(r.config.Network),
		"--read-only",
		"--cap-drop=ALL",
		"--security-opt", "no-new-privileges",
	}
	if strings.TrimSpace(r.config.User) != "" {
		args = append(args, "--user", strings.TrimSpace(r.config.User))
	}
	if strings.TrimSpace(r.config.Memory) != "" {
		args = append(args, "--memory", strings.TrimSpace(r.config.Memory))
	}
	if strings.TrimSpace(r.config.CPUs) != "" {
		args = append(args, "--cpus", strings.TrimSpace(r.config.CPUs))
	}
	if r.config.PidsLimit > 0 {
		args = append(args, "--pids-limit", strconv.Itoa(r.config.PidsLimit))
	}
	if includeAuth {
		args = append(args, "--env", "TOOL_AUTH_BEARER")
	}
	args = append(
		args,
		"-e", "TOOL_ENDPOINT="+endpoint,
		"--entrypoint", strings.TrimSpace(r.config.Shell),
		strings.TrimSpace(r.config.Image),
		"-lc",
		`if [ -n "$TOOL_AUTH_BEARER" ]; then HEADER="Authorization: Bearer $TOOL_AUTH_BEARER"; cat | curl -sS --fail-with-body -X POST -H "$HEADER" --data-binary @- "$TOOL_ENDPOINT"; else cat | curl -sS --fail-with-body -X POST --data-binary @- "$TOOL_ENDPOINT"; fi`,
	)
	return args
}

func compactStderr(stderr string) string {
	value := strings.TrimSpace(stderr)
	if len(value) <= 400 {
		return value
	}
	return value[:400]
}

func mapToEnv(values map[string]string) []string {
	if len(values) == 0 {
		return nil
	}
	out := make([]string, 0, len(values))
	for key, value := range values {
		if strings.TrimSpace(key) == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}

func normalizeEnvKey(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(raw))
	for i := 0; i < len(raw); i++ {
		ch := raw[i]
		switch {
		case ch >= 'a' && ch <= 'z':
			builder.WriteByte(ch - ('a' - 'A'))
		case ch >= 'A' && ch <= 'Z':
			builder.WriteByte(ch)
		case ch >= '0' && ch <= '9':
			builder.WriteByte(ch)
		default:
			builder.WriteByte('_')
		}
	}
	return builder.String()
}

func dedupeStrings(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(values))
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		key := strings.ToLower(value)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
	}
	return out
}
