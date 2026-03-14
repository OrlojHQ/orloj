# Security and Isolation

## Current Controls

- `AgentPolicy` gates model/tool/token usage.
- `AgentRole` and `ToolPermission` enforce per-tool authorization before runtime execution.
- Tool runtime can enforce timeout/retry/isolation from `Tool.spec.runtime`.
- Unsupported tools or disallowed runtime requests fail closed.
- Permission denials are fail-closed and terminal for the current task/message execution path.
- UI runtime diagnostics surface governance context (role bindings, matched tool permission rules, denial reason chips).

## Tool Isolation Modes

- `none`
- `sandboxed`
- `container`
- `wasm` (command-backed runtime path with pluggable executor factory)

Container backend is active for high-risk paths and currently supports HTTP tools with constrained execution settings.
WASM backend uses an executor-factory boundary (`WASMToolExecutorFactory`) and defaults to a command-backed executor (`WASMCommandExecutorFactory`) that invokes a configured wasm runtime binary (default `wasmtime`).
If wasm runtime configuration is invalid (for example missing module path or runtime binary), calls fail closed with deterministic non-retryable runtime policy errors.
WASM guest responses must satisfy the module contract (`contract_version=v1`, `status=ok|error|denied`); contract violations fail closed as non-retryable runtime policy errors.

## Secret Handling

- secrets are namespace-scoped CRDs (`Secret`)
- values in `spec.data` are base64-encoded
- `stringData` supports write-time plaintext input
- runtime resolves `Tool.spec.auth.secretRef` and injects auth safely into isolated execution

## Enterprise Next Steps

- policy scope expansion (model/tool/data/cost/exec policy scopes)
- approval workflows for high-risk operations
- stricter RBAC/role model for agent and tool permissions
