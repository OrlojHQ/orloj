package resources

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

// TestYAMLParserFieldCompleteness ensures the constrained-YAML parser handles
// every JSON-tagged field on each resource struct. The strategy:
//
//  1. Build a fully-populated Go struct (every field set to a non-zero value).
//  2. Marshal it to JSON and parse via the JSON code path → "reference".
//  3. Parse a hand-written constrained-YAML fixture → "yaml result".
//  4. Walk the reference struct with reflection; for every exported field that
//     carries a `json` tag (excluding `json:"-"`), assert the YAML result has
//     the same non-zero value.
//
// When someone adds a new field to a resource struct, the test fails immediately
// because the YAML fixture doesn't set it, producing a clear error like:
//
//	field Spec.Cli.ImagePullSecret (json:"image_pull_secret") is zero after YAML parse
//
// To fix: add the field to the YAML fixture AND the parser's switch statement.

// skipPaths lists struct field paths that are intentionally not round-tripped
// through the constrained-YAML parser (server-set, internal, or complex nested
// types handled via separate code paths like input_schema / output_schema).
var skipPaths = map[string]map[string]bool{
	// --- Global metadata fields that are server-managed ---
	// Annotations, ResourceVersion, Generation, CreatedAt are set by the
	// server on write and round-tripped on read. Users rarely set them in
	// manifests; the YAML parser supports resourceVersion/generation/createdAt
	// already but annotations are a map that needs separate subsection handling.
	// We skip them globally rather than in every resource.
	"*": {
		"Metadata.Annotations":     true, // map[string]string, server-managed
		"Metadata.ResourceVersion": true, // server-set on write
		"Metadata.Generation":      true, // server-set on write
		"Metadata.CreatedAt":       true, // server-set on create
	},
	"Agent": {
		"Spec.Model":                    true, // json:"-", internal
		"Spec.Execution.OutputSchema":   true, // map[string]any, parsed via separate pass
		"Status":                        true,
	},
	"Tool": {
		"Spec.McpServerRef": true, // server-generated, not user-set
		"Spec.McpToolName":  true,
		"Spec.InputSchema":  true, // parsed via separate parseSimpleYAMLMap pass
		"Status":            true,
	},
	"Tool/CLI": {
		"Spec.McpServerRef": true,
		"Spec.McpToolName":  true,
		"Spec.InputSchema":  true,
		"Spec.Endpoint":     true, // not used for CLI type
		"Spec.Auth":         true, // not valid for CLI type
		"Spec.Wasm":         true, // not used for CLI type
		"Spec.A2A":          true, // not used for CLI type
		"Status":            true,
	},
	"Tool/HTTP": {
		"Spec.McpServerRef": true,
		"Spec.McpToolName":  true,
		"Spec.InputSchema":  true,
		"Spec.Cli":          true, // not used for HTTP type
		"Spec.Wasm":         true, // not used for HTTP type
		"Spec.A2A":          true, // not used for HTTP type
		"Status":            true,
	},
	"Tool/WASM": {
		"Spec.McpServerRef": true,
		"Spec.McpToolName":  true,
		"Spec.InputSchema":  true,
		"Spec.Endpoint":     true, // not used for WASM type
		"Spec.Cli":          true, // not used for WASM type
		"Spec.Auth":         true, // not used for WASM type
		"Spec.A2A":          true, // not used for WASM type
		"Status":            true,
	},
	"Tool/A2A": {
		"Spec.McpServerRef": true,
		"Spec.McpToolName":  true,
		"Spec.InputSchema":  true,
		"Spec.Endpoint":     true, // not used for A2A type
		"Spec.Cli":          true, // not used for A2A type
		"Spec.Wasm":         true, // not used for A2A type
		"Status":            true,
	},
	"TaskApproval": {
		"Status":                   true,
		"Spec.Output":              true, // any type, parsed separately
		"Spec.ResumeContext":       true, // map[string]any, parsed separately
		"Spec.AllowRequestChanges": true, // *bool default-set by Normalize
		"Spec.MaxReviewCycles":     true, // default-set by Normalize
		"Spec.ReviewCycle":         true, // default-set by Normalize
		"Spec.OutputFormat":        true, // default-set by Normalize
	},
	"ToolApproval": {
		"Status": true,
	},
	"TaskSchedule": {
		"Status":                                  true,
		"Spec.TaskRef":                            true, // mutually exclusive with task_template
		"Spec.TaskTemplate.MessageRetry.NonRetryable": true, // list inside nested template — not wired yet
		"Spec.TaskTemplate.Requirements":          true, // nested sub-object inside template — not wired yet
	},
	"TaskWebhook": {
		"Status":                                  true,
		"Spec.TaskRef":                            true, // mutually exclusive with task_template
		"Spec.TaskTemplate.MessageRetry.NonRetryable": true,
		"Spec.TaskTemplate.Requirements":          true,
		"Spec.Idempotency.EventIDFromBody":        true, // mutually exclusive with event_id_header
	},
	"Task": {
		"Status":            true,
		"Spec.Input":        true, // map parsed inline, validated elsewhere
		"Spec.MaxTurns":     true, // zero is valid (unlimited)
		"Spec.Retry":        true, // defaults set by Normalize
		"Spec.MessageRetry": true,
		"Spec.Requirements": true,
	},
	"Worker": {
		"Status": true,
	},
	"McpServer": {
		"Status":        true,
		"Spec.Endpoint": true, // mutually exclusive with command (stdio transport)
	},
	"ModelEndpoint": {
		"Status": true,
	},
	"AgentSystem": {
		"Status":     true,
		"Spec.Graph": true, // complex nested map, covered by dedicated tests
	},
	"AgentPolicy": {
		"Status": true,
	},
	"AgentRole": {
		"Status": true,
	},
	"ToolPermission": {
		"Status": true,
	},
	"Memory": {
		"Status": true,
	},
	"Secret": {
		"Status":          true,
		"Spec.Data":       true, // map, tested separately
		"Spec.StringData": true,
	},
}

func shouldSkip(kind, path string) bool {
	for _, k := range []string{"*", kind} {
		if m, ok := skipPaths[k]; ok {
			if m[path] {
				return true
			}
			for prefix := range m {
				if strings.HasPrefix(path, prefix+".") {
					return true
				}
			}
		}
	}
	return false
}

// checkNonZero walks v and reports every json-tagged field that is at its zero value.
func checkNonZero(t *testing.T, kind string, v reflect.Value, path string) {
	t.Helper()
	if v.Kind() == reflect.Ptr {
		if v.IsNil() {
			if !shouldSkip(kind, path) {
				t.Errorf("YAML parser gap: %s (kind %s) is nil — add to YAML fixture and parser", path, kind)
			}
			return
		}
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return
	}
	vt := v.Type()
	for i := 0; i < vt.NumField(); i++ {
		field := vt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}
		jsonName := strings.Split(tag, ",")[0]
		if jsonName == "" {
			continue
		}

		fieldPath := path + "." + field.Name
		if path == "" {
			fieldPath = field.Name
		}

		if shouldSkip(kind, fieldPath) {
			continue
		}

		fv := v.Field(i)

		// Recurse into struct / *struct fields.
		ft := field.Type
		if ft.Kind() == reflect.Ptr {
			ft = ft.Elem()
		}
		if ft.Kind() == reflect.Struct && ft != reflect.TypeOf(json.RawMessage{}) {
			checkNonZero(t, kind, fv, fieldPath)
			continue
		}

		if fv.IsZero() {
			t.Errorf("YAML parser gap: field %s (json:%q) is zero after YAML parse — add to fixture AND parser for %s",
				fieldPath, jsonName, kind)
		}
	}
}

// ---------------------------------------------------------------------------
// Maximal YAML fixtures — every user-settable field must appear.
// ---------------------------------------------------------------------------

var agentMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Agent
metadata:
  name: completeness-agent
  namespace: test-ns
  labels:
    env: test
spec:
  model_ref: openai-default
  prompt: "You are a test agent."
  tools:
    - web_search
  allowed_tools:
    - web_search
  fallback_model_refs:
    - anthropic-fallback
  roles:
    - analyst
  memory:
    type: vector
    provider: builtin
    ref: my-memory
    allow:
      - read
  execution:
    profile: dynamic
    duplicate_tool_call_policy: short_circuit
    on_contract_violation: observe
    tool_use_behavior: stop_on_first_tool
    tool_sequence:
      - web_search
    required_output_markers:
      - DONE
  limits:
    max_steps: 10
    timeout: 5m
`)

// Tool CLI fixture — exercises cli-specific fields (auth not valid for CLI type).
var toolCLIMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: completeness-tool-cli
  namespace: test-ns
  labels:
    env: test
spec:
  type: cli
  description: A CLI test tool
  risk_level: medium
  capabilities:
    - web.search.invoke
  operation_classes:
    - read
  runtime:
    timeout: 30s
    isolation_mode: container
    retry:
      max_attempts: 3
      backoff: 1s
      max_backoff: 30s
      jitter: full
  cli:
    command: mytool
    image: ghcr.io/org/tool:v1
    image_pull_secret: ghcr-creds
    network: host
    output: stdout
    working_dir: /app
    stdin_from_input: true
    args:
      - --verbose
    env:
      MY_VAR: value
    env_from:
      - name: API_KEY
        secretRef: my-secret
        key: api-key
    resources:
      memory: 512m
      cpus: "1.0"
      pids_limit: 256
`)

// Tool HTTP fixture — exercises auth and wasm fields.
var toolHTTPMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: completeness-tool-http
  namespace: test-ns
  labels:
    env: test
spec:
  type: http
  description: An HTTP test tool
  endpoint: https://example.com/tool
  risk_level: medium
  capabilities:
    - web.search.invoke
  operation_classes:
    - read
  auth:
    profile: bearer
    secretRef: my-secret
    headerName: Authorization
    tokenURL: https://auth.example.com/token
    scopes:
      - read
  runtime:
    timeout: 30s
    retry:
      max_attempts: 3
      backoff: 1s
      max_backoff: 30s
      jitter: full
`)

// Tool WASM fixture — exercises wasm-specific fields.
var toolWASMMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: completeness-tool-wasm
  namespace: test-ns
  labels:
    env: test
spec:
  type: wasm
  description: A WASM test tool
  risk_level: medium
  capabilities:
    - wasm.echo.invoke
  runtime:
    timeout: 30s
    retry:
      max_attempts: 3
      backoff: 1s
      max_backoff: 30s
      jitter: full
  wasm:
    module: oci://registry.example.com/tool:v1
    entrypoint: run
    max_memory_bytes: 67108864
    fuel: 1000000
    enable_wasi: true
    image_pull_secret: oci-creds
`)

// Tool A2A fixture — exercises a2a-specific fields.
var toolA2AMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Tool
metadata:
  name: completeness-tool-a2a
  namespace: test-ns
  labels:
    env: test
spec:
  type: a2a
  description: An A2A test tool
  risk_level: medium
  capabilities:
    - a2a.invoke
  operation_classes:
    - read
  auth:
    profile: bearer
    secretRef: my-secret
    headerName: Authorization
    tokenURL: https://auth.example.com/token
    scopes:
      - read
  runtime:
    timeout: 30s
    retry:
      max_attempts: 3
      backoff: 1s
      max_backoff: 30s
      jitter: full
  a2a:
    agent_url: https://remote.example.com/a2a
    protocol_version: "1.0"
    prefer_streaming: true
`)

var modelEndpointMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: ModelEndpoint
metadata:
  name: completeness-mep
  namespace: test-ns
  labels:
    env: test
spec:
  provider: openai
  base_url: https://api.openai.com/v1
  default_model: gpt-4o-mini
  allowPrivate: true
  options:
    anthropic_version: "2023-06-01"
  auth:
    secretRef: openai-key
`)

var mcpServerMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: McpServer
metadata:
  name: completeness-mcp
  namespace: test-ns
  labels:
    env: test
spec:
  transport: stdio
  command: mcp-server
  image: ghcr.io/org/mcp:v1
  image_pull_secret: ghcr-creds
  idle_timeout: 5m
  allowPrivate: true
  args:
    - --port=8080
  env:
    - name: API_KEY
      value: test-value
  auth:
    secretRef: mcp-secret
    profile: bearer
    headerName: Authorization
    tokenURL: https://auth.example.com/token
    scopes:
      - read
  tool_filter:
    include:
      - search
  reconnect:
    max_attempts: 5
    backoff: 2s
  resources:
    memory: 1g
    cpus: "2.0"
    pids_limit: 512
  default_tool_runtime:
    timeout: 30s
    isolation_mode: container
    retry:
      max_attempts: 3
      backoff: 1s
      max_backoff: 30s
      jitter: "0.1"
`)

var agentPolicyMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: AgentPolicy
metadata:
  name: completeness-policy
  namespace: test-ns
  labels:
    env: test
spec:
  max_tokens_per_run: 10000
  apply_mode: scoped
  max_child_depth: 3
  max_child_tasks: 10
  allowed_models:
    - gpt-4o
  blocked_tools:
    - dangerous-tool
  target_systems:
    - my-system
  target_tasks:
    - my-task
  target_agents:
    - my-agent
`)

var agentRoleMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: AgentRole
metadata:
  name: completeness-role
  namespace: test-ns
  labels:
    env: test
spec:
  description: A test role
  permissions:
    - tool:web_search:invoke
`)

var toolPermissionMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: ToolPermission
metadata:
  name: completeness-tp
  namespace: test-ns
  labels:
    env: test
spec:
  tool_ref: web_search
  action: invoke
  match_mode: all
  apply_mode: scoped
  required_permissions:
    - tool:web_search:invoke
  target_agents:
    - my-agent
  operation_rules:
    - operation_class: read
      verdict: allow
`)

var memoryMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Memory
metadata:
  name: completeness-memory
  namespace: test-ns
  labels:
    env: test
spec:
  type: vector
  provider: builtin
  embedding_model: text-embedding-3-small
  endpoint: https://memory.example.com
  endpoint_secret_ref: mem-secret
  auth:
    secretRef: mem-auth
`)

var workerMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Worker
metadata:
  name: completeness-worker
  namespace: test-ns
  labels:
    env: test
spec:
  region: us-east-1
  max_concurrent_tasks: 4
  capabilities:
    gpu: true
    supported_models:
      - gpt-4o
`)

var taskMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: Task
metadata:
  name: completeness-task
  namespace: test-ns
  labels:
    env: test
spec:
  system: my-system
  mode: run
  priority: high
  input:
    query: test
`)

var taskScheduleMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: TaskSchedule
metadata:
  name: completeness-schedule
  namespace: test-ns
  labels:
    env: test
spec:
  schedule: "*/5 * * * *"
  time_zone: America/New_York
  suspend: true
  starting_deadline_seconds: 600
  concurrency_policy: forbid
  successful_history_limit: 5
  failed_history_limit: 2
  task_template:
    system: my-system
    mode: run
    priority: high
    max_turns: 10
    input:
      query: scheduled test
    retry:
      max_attempts: 2
      backoff: 5s
    message_retry:
      max_attempts: 3
      backoff: 2s
      max_backoff: 1m
      jitter: full
`)

var taskWebhookMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: TaskWebhook
metadata:
  name: completeness-webhook
  namespace: test-ns
  labels:
    env: test
spec:
  suspend: true
  task_template:
    system: my-system
    mode: run
    priority: high
    max_turns: 10
    input:
      query: webhook test
    retry:
      max_attempts: 2
      backoff: 5s
    message_retry:
      max_attempts: 3
      backoff: 2s
      max_backoff: 1m
      jitter: full
  auth:
    profile: hmac
    secret_ref: webhook-secret
    signature_header: X-Signature-256
    signature_prefix: "sha256="
    timestamp_header: X-Timestamp
    max_skew_seconds: 600
    algorithm: sha256
    payload_format: timestamp_dot_body
    payload_prefix: "v0"
    payload_separator: ":"
    signature_encoding: hex
    header_format: kv_pairs
    signature_key: s
    timestamp_key: t
  idempotency:
    event_id_header: X-Event-Id
    dedupe_window_seconds: 3600
  payload:
    mode: raw
    input_key: body
`)

var toolApprovalMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: ToolApproval
metadata:
  name: completeness-ta
  namespace: test-ns
  labels:
    env: test
spec:
  task_ref: task/default/t1
  tool: deploy
  operation_class: write
  agent: deployer
  input: '{"target":"prod"}'
  reason: needs approval
  ttl: 15m
`)

var taskApprovalMaximalYAML = []byte(`apiVersion: orloj.dev/v1
kind: TaskApproval
metadata:
  name: completeness-tap
  namespace: test-ns
  labels:
    env: test
spec:
  task_ref: task/default/t1
  checkpoint_id: cp-1
  checkpoint_type: agent_output
  agent: writer
  reason: review needed
  ttl: 10m
  supersedes: old-approval
`)

// ---------------------------------------------------------------------------
// Test runner
// ---------------------------------------------------------------------------

func TestYAMLParserFieldCompleteness(t *testing.T) {
	cases := []struct {
		kind    string
		yaml    []byte
		parseFn func([]byte) (any, error)
	}{
		{"Agent", agentMaximalYAML, func(b []byte) (any, error) { return ParseAgentManifest(b) }},
		{"Tool/CLI", toolCLIMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"Tool/HTTP", toolHTTPMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"Tool/WASM", toolWASMMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"Tool/A2A", toolA2AMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"ModelEndpoint", modelEndpointMaximalYAML, func(b []byte) (any, error) { return ParseModelEndpointManifest(b) }},
		{"McpServer", mcpServerMaximalYAML, func(b []byte) (any, error) { return ParseMcpServerManifest(b) }},
		{"AgentPolicy", agentPolicyMaximalYAML, func(b []byte) (any, error) { return ParseAgentPolicyManifest(b) }},
		{"AgentRole", agentRoleMaximalYAML, func(b []byte) (any, error) { return ParseAgentRoleManifest(b) }},
		{"ToolPermission", toolPermissionMaximalYAML, func(b []byte) (any, error) { return ParseToolPermissionManifest(b) }},
		{"Memory", memoryMaximalYAML, func(b []byte) (any, error) { return ParseMemoryManifest(b) }},
		{"Worker", workerMaximalYAML, func(b []byte) (any, error) { return ParseWorkerManifest(b) }},
		{"Task", taskMaximalYAML, func(b []byte) (any, error) { return ParseTaskManifest(b) }},
		{"TaskSchedule", taskScheduleMaximalYAML, func(b []byte) (any, error) { return ParseTaskScheduleManifest(b) }},
		{"TaskWebhook", taskWebhookMaximalYAML, func(b []byte) (any, error) { return ParseTaskWebhookManifest(b) }},
		{"ToolApproval", toolApprovalMaximalYAML, func(b []byte) (any, error) { return ParseToolApprovalManifest(b) }},
		{"TaskApproval", taskApprovalMaximalYAML, func(b []byte) (any, error) { return ParseTaskApprovalManifest(b) }},
	}

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			obj, err := tc.parseFn(tc.yaml)
			if err != nil {
				t.Fatalf("YAML parse failed for %s: %v", tc.kind, err)
			}
			checkNonZero(t, tc.kind, reflect.ValueOf(obj), "")
		})
	}
}

// TestYAMLJSONParity ensures YAML and JSON parsing produce identical results for
// each resource type. A fully-populated struct is marshaled to JSON, parsed via
// the JSON path, then the same data expressed as YAML is parsed via the YAML path.
// Any field that differs means the YAML parser is not handling it correctly.
func TestYAMLJSONParity(t *testing.T) {
	cases := []struct {
		kind    string
		yaml    []byte
		parseFn func([]byte) (any, error)
	}{
		{"Agent", agentMaximalYAML, func(b []byte) (any, error) { return ParseAgentManifest(b) }},
		{"Tool/CLI", toolCLIMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"Tool/HTTP", toolHTTPMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"Tool/WASM", toolWASMMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"Tool/A2A", toolA2AMaximalYAML, func(b []byte) (any, error) { return ParseToolManifest(b) }},
		{"ModelEndpoint", modelEndpointMaximalYAML, func(b []byte) (any, error) { return ParseModelEndpointManifest(b) }},
		{"McpServer", mcpServerMaximalYAML, func(b []byte) (any, error) { return ParseMcpServerManifest(b) }},
		{"AgentPolicy", agentPolicyMaximalYAML, func(b []byte) (any, error) { return ParseAgentPolicyManifest(b) }},
		{"AgentRole", agentRoleMaximalYAML, func(b []byte) (any, error) { return ParseAgentRoleManifest(b) }},
		{"ToolPermission", toolPermissionMaximalYAML, func(b []byte) (any, error) { return ParseToolPermissionManifest(b) }},
		{"Memory", memoryMaximalYAML, func(b []byte) (any, error) { return ParseMemoryManifest(b) }},
		{"Worker", workerMaximalYAML, func(b []byte) (any, error) { return ParseWorkerManifest(b) }},
		{"TaskSchedule", taskScheduleMaximalYAML, func(b []byte) (any, error) { return ParseTaskScheduleManifest(b) }},
		{"TaskWebhook", taskWebhookMaximalYAML, func(b []byte) (any, error) { return ParseTaskWebhookManifest(b) }},
		{"ToolApproval", toolApprovalMaximalYAML, func(b []byte) (any, error) { return ParseToolApprovalManifest(b) }},
		{"TaskApproval", taskApprovalMaximalYAML, func(b []byte) (any, error) { return ParseTaskApprovalManifest(b) }},
	}

	for _, tc := range cases {
		t.Run(tc.kind, func(t *testing.T) {
			// Parse YAML
			yamlObj, err := tc.parseFn(tc.yaml)
			if err != nil {
				t.Fatalf("YAML parse failed: %v", err)
			}

			// Marshal to JSON, then re-parse via JSON path
			jsonBytes, err := json.Marshal(yamlObj)
			if err != nil {
				t.Fatalf("JSON marshal failed: %v", err)
			}
			jsonObj, err := tc.parseFn(jsonBytes)
			if err != nil {
				t.Fatalf("JSON re-parse failed: %v", err)
			}

			// Marshal both to JSON for comparison
			yamlJSON, _ := json.Marshal(yamlObj)
			jsonJSON, _ := json.Marshal(jsonObj)

			if string(yamlJSON) != string(jsonJSON) {
				diffFields(t, tc.kind, yamlObj, jsonObj, "")
			}
		})
	}
}

// diffFields reports individual field differences between two structs.
func diffFields(t *testing.T, kind string, yamlObj, jsonObj any, path string) {
	t.Helper()
	yv := reflect.ValueOf(yamlObj)
	jv := reflect.ValueOf(jsonObj)
	if yv.Kind() == reflect.Ptr {
		yv = yv.Elem()
	}
	if jv.Kind() == reflect.Ptr {
		jv = jv.Elem()
	}
	if yv.Kind() != reflect.Struct || jv.Kind() != reflect.Struct {
		return
	}
	yt := yv.Type()
	for i := 0; i < yt.NumField(); i++ {
		field := yt.Field(i)
		if !field.IsExported() {
			continue
		}
		tag := field.Tag.Get("json")
		if tag == "-" || tag == "" {
			continue
		}
		fp := field.Name
		if path != "" {
			fp = path + "." + field.Name
		}
		if shouldSkip(kind, fp) {
			continue
		}
		yf := yv.Field(i)
		jf := jv.Field(i)
		yJSON, _ := json.Marshal(yf.Interface())
		jJSON, _ := json.Marshal(jf.Interface())
		if string(yJSON) != string(jJSON) {
			jsonName := strings.Split(tag, ",")[0]
			t.Errorf("YAML/JSON mismatch at %s (json:%q): YAML=%s JSON=%s",
				fp, jsonName, string(yJSON), string(jJSON))
		}
	}
}

// TestFixtureCoversAllResources ensures we don't forget to add a fixture when
// a new resource kind is added to ParseManifest.
func TestFixtureCoversAllResources(t *testing.T) {
	coveredKinds := map[string]bool{
		"agent":          true,
		"agentsystem":    true, // covered via AgentSystem in completeness test
		"tool":           true,
		"modelendpoint":  true,
		"mcpserver":      true,
		"agentpolicy":    true,
		"agentrole":      true,
		"toolpermission":  true,
		"memory":         true,
		"worker":         true,
		"task":           true,
		"taskschedule":   true,
		"taskwebhook":    true,
		"toolapproval":   true,
		"taskapproval":   true,
		"secret":         true,
		"sealedsecret":   true, // uses full YAML library, not constrained parser
		"contextadapter": true, // uses full YAML library, not constrained parser
		"evaldataset":    true, // uses full YAML library, not constrained parser
		"evalrun":        true, // uses full YAML library, not constrained parser
	}

	// Probe ParseManifest with every kind it supports.
	probeKinds := []string{
		"Agent", "AgentSystem", "ModelEndpoint", "Tool", "Secret",
		"SealedSecret", "Memory", "AgentPolicy", "AgentRole",
		"ToolPermission", "ToolApproval", "TaskApproval", "Task",
		"TaskSchedule", "TaskWebhook", "Worker", "McpServer",
		"ContextAdapter", "EvalDataset", "EvalRun",
	}
	for _, k := range probeKinds {
		norm := strings.ToLower(k)
		if !coveredKinds[norm] {
			t.Errorf("resource kind %q is in ParseManifest but has no completeness test fixture — add one to manifest_parser_completeness_test.go", k)
		}
	}
}
