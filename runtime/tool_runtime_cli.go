package agentruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"

	"github.com/OrlojHQ/orloj/resources"
)

// CLICommandRunner abstracts process execution for testability.
type CLICommandRunner interface {
	Run(ctx context.Context, command string, args []string, stdin string, env []string, dir string) (stdout string, stderr string, exitCode int, err error)
}

type osExecCLICommandRunner struct{}

func (r *osExecCLICommandRunner) Run(ctx context.Context, command string, args []string, stdin string, env []string, dir string) (string, string, int, error) {
	cmd := exec.CommandContext(ctx, command, args...) //nolint:gosec
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	stdout := NewBoundedWriter(DefaultMaxToolOutputBytes)
	stderr := NewBoundedWriter(DefaultMaxToolOutputBytes)
	cmd.Stdout = stdout
	cmd.Stderr = stderr
	if len(env) > 0 {
		cmd.Env = env
	}
	if dir != "" {
		cmd.Dir = dir
	}
	err := cmd.Run()
	exitCode := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			return stdout.String(), stderr.String(), -1, err
		}
	}
	return stdout.String(), stderr.String(), exitCode, nil
}

// CLIToolRuntimeConfig holds optional policy for direct CLI execution.
type CLIToolRuntimeConfig struct {
	AllowedCommands []string
	MaxArgvLength   int
}

func DefaultCLIToolRuntimeConfig() CLIToolRuntimeConfig {
	return CLIToolRuntimeConfig{
		MaxArgvLength: 4096,
	}
}

// CLIToolRuntime executes CLI tools directly on the worker host.
type CLIToolRuntime struct {
	registry  ToolCapabilityRegistry
	secrets   SecretResolver
	runner    CLICommandRunner
	config    CLIToolRuntimeConfig
	namespace string
}

func NewCLIToolRuntime(registry ToolCapabilityRegistry, secrets SecretResolver, runner CLICommandRunner, config CLIToolRuntimeConfig) *CLIToolRuntime {
	if runner == nil {
		runner = &osExecCLICommandRunner{}
	}
	if secrets == nil {
		secrets = NewEnvSecretResolver("ORLOJ_SECRET_")
	}
	if config.MaxArgvLength <= 0 {
		config.MaxArgvLength = DefaultCLIToolRuntimeConfig().MaxArgvLength
	}
	return &CLIToolRuntime{
		registry: registry,
		secrets:  secrets,
		runner:   runner,
		config:   config,
	}
}

func (r *CLIToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewCLIToolRuntime(registry, nil, nil, DefaultCLIToolRuntimeConfig())
	}
	return &CLIToolRuntime{
		registry:  registry,
		secrets:   r.secrets,
		runner:    r.runner,
		config:    r.config,
		namespace: r.namespace,
	}
}

func (r *CLIToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewCLIToolRuntime(nil, nil, nil, DefaultCLIToolRuntimeConfig())
	}
	cp := *r
	cp.namespace = resources.NormalizeNamespace(strings.TrimSpace(namespace))
	if aware, ok := cp.secrets.(namespaceAwareSecretResolver); ok {
		cp.secrets = aware.WithNamespace(cp.namespace)
	}
	return &cp
}

func (r *CLIToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
	tool = strings.TrimSpace(tool)
	if tool == "" {
		return "", NewToolError(ToolStatusError, ToolCodeInvalidInput, ToolReasonInvalidInput, false,
			"missing tool name", ErrInvalidToolRuntimePolicy, map[string]string{"field": "tool"})
	}
	if r.registry == nil {
		return "", NewToolError(ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			"missing tool registry for CLI runtime", ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool})
	}
	spec, ok := r.registry.Resolve(tool)
	if !ok {
		return "", NewToolError(ToolStatusError, ToolCodeUnsupportedTool, ToolReasonToolUnsupported, false,
			fmt.Sprintf("unsupported tool %s", tool), ErrUnsupportedTool, map[string]string{"tool": tool})
	}
	if strings.ToLower(strings.TrimSpace(spec.Type)) != "cli" {
		return "", NewToolError(ToolStatusError, ToolCodeUnsupportedTool, ToolReasonToolUnsupported, false,
			fmt.Sprintf("tool=%s type=%s unsupported by CLI runtime", tool, spec.Type), ErrUnsupportedTool,
			map[string]string{"tool": tool, "type": spec.Type})
	}

	command := strings.TrimSpace(spec.Cli.Command)
	if command == "" {
		return "", NewToolError(ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			fmt.Sprintf("tool=%s missing cli.command", tool), ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool})
	}

	// Defense in depth: any tool reaching this runtime is about to execute on
	// the worker host (the governed dispatcher only routes cli+isolation_mode=none
	// here). Refuse to run unless an explicit command allowlist has been
	// configured. This protects against stored Tool objects that predate the
	// admission gate or that were written via non-API code paths (controllers,
	// migrations, direct store writes).
	if len(r.config.AllowedCommands) == 0 {
		return "", NewToolError(ToolStatusError, ToolCodeRuntimePolicyInvalid, ToolReasonRuntimePolicyInvalid, false,
			fmt.Sprintf("tool=%s host CLI execution refused: no command allowlist configured (set ORLOJ_CLI_TOOL_ALLOWED_COMMANDS or use isolation_mode=container)", tool),
			ErrInvalidToolRuntimePolicy, map[string]string{"tool": tool, "command": command})
	}
	if !isCommandAllowed(command, r.config.AllowedCommands) {
		return "", NewToolError(ToolStatusError, ToolCodeUnsupportedTool, ToolReasonToolUnsupported, false,
			fmt.Sprintf("tool=%s command=%s not in allowed commands list", tool, command), ErrUnsupportedTool,
			map[string]string{"tool": tool, "command": command})
	}

	args, err := evaluateCLIArgs(spec.Cli.Args, input)
	if err != nil {
		return "", err
	}

	if totalArgvLength(append([]string{command}, args...)) > r.config.MaxArgvLength {
		return "", NewToolError(ToolStatusError, ToolCodeInvalidInput, ToolReasonInvalidInput, false,
			fmt.Sprintf("tool=%s total argv length exceeds limit of %d bytes", tool, r.config.MaxArgvLength),
			nil, map[string]string{"tool": tool})
	}

	env, err := r.buildEnv(ctx, spec.Cli)
	if err != nil {
		return "", err
	}

	stdinData := ""
	if spec.Cli.StdinFromInput {
		stdinData = strings.TrimSpace(input)
	}

	stdout, stderr, exitCode, runErr := r.runner.Run(ctx, command, args, stdinData, env, spec.Cli.WorkingDir)
	if runErr != nil {
		return "", NewToolError(ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
			fmt.Sprintf("cli tool=%s execution failed: %v", tool, runErr), runErr,
			map[string]string{"tool": tool, "command": command})
	}

	if exitCode != 0 {
		stderrTail := compactStderr(stderr)
		return "", NewToolError(ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, true,
			fmt.Sprintf("cli tool=%s exited with code %d stderr=%s", tool, exitCode, RedactSensitive(stderrTail)), nil,
			map[string]string{"tool": tool, "exit_code": fmt.Sprintf("%d", exitCode)})
	}

	return formatCLIOutput(spec.Cli.Output, stdout, stderr), nil
}

func (r *CLIToolRuntime) buildEnv(ctx context.Context, cli resources.ToolCliSpec) ([]string, error) {
	env := make([]string, 0, len(cli.Env)+len(cli.EnvFrom))
	for k, v := range cli.Env {
		env = append(env, k+"="+v)
	}
	for _, ref := range cli.EnvFrom {
		secretRef := ref.SecretRef
		if ref.Key != "" {
			secretRef = secretRef + ":" + ref.Key
		}
		value, err := r.secrets.Resolve(ctx, secretRef)
		if err != nil {
			return nil, NewToolError(ToolStatusError, ToolCodeExecutionFailed, ToolReasonBackendFailure, false,
				fmt.Sprintf("failed to resolve secret for env var %s: %v", ref.Name, err),
				fmt.Errorf("%w: %v", ErrToolSecretResolution, err),
				map[string]string{"env_var": ref.Name, "secretRef": ref.SecretRef})
		}
		env = append(env, ref.Name+"="+value)
	}
	return env, nil
}

func formatCLIOutput(mode string, stdout string, stderr string) string {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "stderr":
		return strings.TrimSpace(stderr)
	case "both":
		out := map[string]string{
			"stdout": strings.TrimSpace(stdout),
			"stderr": strings.TrimSpace(stderr),
		}
		data, err := json.Marshal(out)
		if err != nil {
			return strings.TrimSpace(stdout)
		}
		return string(data)
	default:
		return strings.TrimSpace(stdout)
	}
}

func isCommandAllowed(command string, allowed []string) bool {
	for _, a := range allowed {
		if strings.TrimSpace(a) == command {
			return true
		}
	}
	return false
}
