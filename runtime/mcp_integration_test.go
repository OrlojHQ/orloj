package agentruntime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

// mockMcpTransport simulates an MCP server for integration testing.
type mockMcpTransport struct {
	tools       []McpToolDefinition
	callResults map[string]*McpToolResult
}

func (m *mockMcpTransport) Initialize(_ context.Context) (*McpInitResult, error) {
	return &McpInitResult{
		ProtocolVersion: "2025-03-26",
		ServerInfo:      McpServerInfo{Name: "mock-mcp", Version: "1.0.0"},
		Capabilities:    McpCapabilities{Tools: &McpToolCapability{ListChanged: false}},
	}, nil
}

func (m *mockMcpTransport) ListTools(_ context.Context) ([]McpToolDefinition, error) {
	return m.tools, nil
}

func (m *mockMcpTransport) CallTool(_ context.Context, name string, arguments map[string]any) (*McpToolResult, error) {
	if result, ok := m.callResults[name]; ok {
		return result, nil
	}
	argsJSON, _ := json.Marshal(arguments)
	return &McpToolResult{
		Content: []McpContent{{Type: "text", Text: "called " + name + " with " + string(argsJSON)}},
	}, nil
}

func (m *mockMcpTransport) Close() error { return nil }

func TestMcpEndToEnd(t *testing.T) {
	mcpTransport := &mockMcpTransport{
		tools: []McpToolDefinition{
			{
				Name:        "create_issue",
				Description: "Create a new GitHub issue",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"repo":  map[string]any{"type": "string"},
						"title": map[string]any{"type": "string"},
						"body":  map[string]any{"type": "string"},
					},
					"required": []any{"repo", "title"},
				},
			},
			{
				Name:        "search_repos",
				Description: "Search GitHub repositories",
				InputSchema: map[string]any{
					"type": "object",
					"properties": map[string]any{
						"query": map[string]any{"type": "string"},
					},
					"required": []any{"query"},
				},
			},
		},
		callResults: map[string]*McpToolResult{
			"create_issue": {
				Content: []McpContent{{Type: "text", Text: `{"number": 42, "url": "https://github.com/test/repo/issues/42"}`}},
			},
		},
	}

	mcpServer := resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "github-mcp", Namespace: "default"},
		Spec: resources.McpServerSpec{
			Transport: "stdio",
			Command:   "mock-mcp",
		},
	}
	if err := mcpServer.Normalize(); err != nil {
		t.Fatalf("normalize mcpserver: %v", err)
	}

	mcpServerStore := store.NewMcpServerStore()
	mcpServer, err := mcpServerStore.Upsert(context.Background(), mcpServer)
	if err != nil {
		t.Fatalf("upsert mcpserver: %v", err)
	}

	toolStore := store.NewToolStore()

	sessionMgr := &McpSessionManager{
		sessions:       make(map[string]*McpSession),
		secretResolver: nil,
	}
	sessionMgr.sessions["default/github-mcp"] = &McpSession{
		Transport:  mcpTransport,
		InitResult: &McpInitResult{ProtocolVersion: "2025-03-26"},
		ServerName: "github-mcp",
	}

	// --- Step 1: Simulate controller tool discovery ---
	ctx := context.Background()
	session, err := sessionMgr.GetOrCreate(ctx, mcpServer)
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	tools, err := session.Transport.ListTools(ctx)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	// Generate tool resources (simulating what the controller does)
	for _, mcpTool := range tools {
		toolName := mcpServer.Metadata.Name + "--" + strings.ReplaceAll(mcpTool.Name, "_", "-")
		tool := resources.Tool{
			APIVersion: "orloj.dev/v1",
			Kind:       "Tool",
			Metadata: resources.ObjectMeta{
				Name:      toolName,
				Namespace: "default",
				Labels: map[string]string{
					"orloj.dev/mcp-server":    mcpServer.Metadata.Name,
					"orloj.dev/mcp-generated": "true",
				},
			},
			Spec: resources.ToolSpec{
				Type:         "mcp",
				McpServerRef: mcpServer.Metadata.Name,
				McpToolName:  mcpTool.Name,
				Description:  mcpTool.Description,
				InputSchema:  mcpTool.InputSchema,
			},
		}
		_, err := toolStore.Upsert(context.Background(), tool)
		if err != nil {
			t.Fatalf("upsert tool %s: %v", toolName, err)
		}
	}

	// Verify tools were created
	allTools, _ := toolStore.List(context.Background())
	if len(allTools) != 2 {
		t.Fatalf("expected 2 generated tools, got %d", len(allTools))
	}

	// Verify tool properties
	for _, tool := range allTools {
		if tool.Spec.Type != "mcp" {
			t.Errorf("expected type=mcp, got %q", tool.Spec.Type)
		}
		if tool.Spec.McpServerRef != "github-mcp" {
			t.Errorf("expected mcp_server_ref=github-mcp, got %q", tool.Spec.McpServerRef)
		}
		if tool.Spec.Description == "" {
			t.Error("expected non-empty description")
		}
		if len(tool.Spec.InputSchema) == 0 {
			t.Error("expected non-empty input_schema")
		}
		if tool.Metadata.Labels["orloj.dev/mcp-server"] != "github-mcp" {
			t.Errorf("expected owner label, got %v", tool.Metadata.Labels)
		}
	}

	// --- Step 2: Verify MCPToolRuntime can execute tool calls ---
	specs := make(map[string]resources.ToolSpec, len(allTools))
	for _, tool := range allTools {
		key := strings.ToLower(strings.TrimSpace(tool.Metadata.Name))
		specs[key] = tool.Spec
	}
	registry := NewStaticToolCapabilityRegistry(specs)

	mcpRuntime := NewMCPToolRuntime(registry, sessionMgr, mcpServerStore)

	result, err := mcpRuntime.Call(ctx, "github-mcp--create-issue", `{"repo":"test/repo","title":"Test Issue"}`)
	if err != nil {
		t.Fatalf("mcp tool call failed: %v", err)
	}
	if !strings.Contains(result, "42") {
		t.Fatalf("expected issue number 42 in result, got %q", result)
	}

	result, err = mcpRuntime.Call(ctx, "github-mcp--search-repos", `{"query":"orloj"}`)
	if err != nil {
		t.Fatalf("mcp tool call failed: %v", err)
	}
	if !strings.Contains(result, "search_repos") {
		t.Fatalf("expected search_repos call output, got %q", result)
	}

	// --- Step 3: Verify schema propagation to model gateway ---
	schemaMap := map[string]ToolSchemaInfo{}
	for _, tool := range allTools {
		schemaMap[tool.Metadata.Name] = ToolSchemaInfo{
			Description: tool.Spec.Description,
			InputSchema: tool.Spec.InputSchema,
		}
	}

	toolNames := make([]string, 0, len(allTools))
	for _, tool := range allTools {
		toolNames = append(toolNames, tool.Metadata.Name)
	}

	openAITools := buildOpenAIChatTools(toolNames, schemaMap)
	if len(openAITools) != 2 {
		t.Fatalf("expected 2 OpenAI tools, got %d", len(openAITools))
	}
	for _, oaiTool := range openAITools {
		if oaiTool.Function.Description == "" || strings.HasPrefix(oaiTool.Function.Description, "Invoke tool ") {
			t.Errorf("expected rich description, got %q", oaiTool.Function.Description)
		}
		props, ok := oaiTool.Function.Parameters["properties"]
		if !ok {
			t.Error("expected properties in parameters")
		}
		propsMap, _ := props.(map[string]any)
		if _, hasInput := propsMap["input"]; hasInput {
			t.Error("expected rich schema, not generic {input: string}")
		}
	}

	anthropicTools, aliases := buildAnthropicTools(toolNames, schemaMap)
	if len(anthropicTools) != 2 {
		t.Fatalf("expected 2 Anthropic tools, got %d", len(anthropicTools))
	}
	if len(aliases) != 2 {
		t.Fatalf("expected 2 aliases, got %d", len(aliases))
	}
	for _, aToolSpec := range anthropicTools {
		if aToolSpec.Description == "" || strings.HasPrefix(aToolSpec.Description, "Invoke tool ") {
			t.Errorf("expected rich description, got %q", aToolSpec.Description)
		}
	}

	// --- Step 4: GovernedToolRuntime dispatches type=mcp to MCP runtime ---
	governed := NewGovernedToolRuntimeWithAuthorizer(nil, nil, registry, nil, true)
	governed.SetMcpRuntime(mcpRuntime)

	result, err = governed.Call(ctx, "github-mcp--create-issue", `{"repo":"test/repo","title":"Governed Test"}`)
	if err != nil {
		t.Fatalf("governed mcp tool call failed: %v", err)
	}
	if !strings.Contains(result, "42") {
		t.Fatalf("expected issue 42 in governed result, got %q", result)
	}

	// --- Step 5: Verify ToolSchemaResolver on GovernedToolRuntime ---
	schemas := governed.ResolveToolSchemas(toolNames)
	for _, toolName := range toolNames {
		info, ok := schemas[toolName]
		if !ok {
			t.Errorf("expected schema for %s", toolName)
			continue
		}
		if info.Description == "" {
			t.Errorf("expected description for %s", toolName)
		}
		if len(info.InputSchema) == 0 {
			t.Errorf("expected input_schema for %s", toolName)
		}
	}
}

func TestMcpToolFilter(t *testing.T) {
	tools := []McpToolDefinition{
		{Name: "create_issue"},
		{Name: "search_repos"},
		{Name: "list_prs"},
	}

	t.Run("no_filter", func(t *testing.T) {
		filtered := filterMcpTools(tools, nil)
		if len(filtered) != 3 {
			t.Fatalf("expected 3 tools, got %d", len(filtered))
		}
	})

	t.Run("with_allowlist", func(t *testing.T) {
		filtered := filterMcpTools(tools, []string{"create_issue", "list_prs"})
		if len(filtered) != 2 {
			t.Fatalf("expected 2 tools, got %d", len(filtered))
		}
		names := map[string]bool{}
		for _, tool := range filtered {
			names[tool.Name] = true
		}
		if !names["create_issue"] || !names["list_prs"] {
			t.Fatalf("expected create_issue and list_prs, got %v", names)
		}
	})
}

func filterMcpTools(tools []McpToolDefinition, include []string) []McpToolDefinition {
	if len(include) == 0 {
		return tools
	}
	allowed := make(map[string]struct{}, len(include))
	for _, name := range include {
		allowed[strings.TrimSpace(name)] = struct{}{}
	}
	filtered := make([]McpToolDefinition, 0, len(tools))
	for _, t := range tools {
		if _, ok := allowed[t.Name]; ok {
			filtered = append(filtered, t)
		}
	}
	return filtered
}

func TestMcpTextResult(t *testing.T) {
	t.Run("nil_result", func(t *testing.T) {
		var r *McpToolResult
		if r.McpTextResult() != "" {
			t.Error("expected empty string for nil result")
		}
	})

	t.Run("single_text", func(t *testing.T) {
		r := &McpToolResult{Content: []McpContent{{Type: "text", Text: "hello"}}}
		if r.McpTextResult() != "hello" {
			t.Errorf("expected 'hello', got %q", r.McpTextResult())
		}
	})

	t.Run("multiple_text", func(t *testing.T) {
		r := &McpToolResult{Content: []McpContent{
			{Type: "text", Text: "line1"},
			{Type: "text", Text: "line2"},
		}}
		result := r.McpTextResult()
		if !strings.Contains(result, "line1") || !strings.Contains(result, "line2") {
			t.Errorf("expected both lines, got %q", result)
		}
	})
}

func TestMcpToolRuntimeErrors(t *testing.T) {
	ctx := context.Background()

	t.Run("missing_tool", func(t *testing.T) {
		rt := NewMCPToolRuntime(NewStaticToolCapabilityRegistry(nil), nil, nil)
		_, err := rt.Call(ctx, "nonexistent", "{}")
		if err == nil {
			t.Fatal("expected error for missing tool")
		}
	})

	t.Run("missing_mcp_server", func(t *testing.T) {
		specs := map[string]resources.ToolSpec{
			"test-tool": {Type: "mcp", McpServerRef: "missing-server", McpToolName: "test"},
		}
		mcpStore := store.NewMcpServerStore()
		rt := NewMCPToolRuntime(NewStaticToolCapabilityRegistry(specs), nil, mcpStore)
		_, err := rt.Call(ctx, "test-tool", "{}")
		if err == nil {
			t.Fatal("expected error for missing MCP server")
		}
	})
}

func TestMcpServerNormalize(t *testing.T) {
	t.Run("valid_stdio", func(t *testing.T) {
		s := resources.McpServer{
			Metadata: resources.ObjectMeta{Name: "test"},
			Spec:     resources.McpServerSpec{Transport: "stdio", Command: "test-cmd"},
		}
		if err := s.Normalize(); err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
		if s.Spec.Transport != "stdio" {
			t.Errorf("expected stdio, got %q", s.Spec.Transport)
		}
	})

	t.Run("valid_http", func(t *testing.T) {
		s := resources.McpServer{
			Metadata: resources.ObjectMeta{Name: "test"},
			Spec:     resources.McpServerSpec{Transport: "http", Endpoint: "https://example.com/mcp"},
		}
		if err := s.Normalize(); err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
	})

	t.Run("missing_transport", func(t *testing.T) {
		s := resources.McpServer{Metadata: resources.ObjectMeta{Name: "test"}}
		if err := s.Normalize(); err == nil {
			t.Fatal("expected error for missing transport")
		}
	})

	t.Run("stdio_missing_command", func(t *testing.T) {
		s := resources.McpServer{
			Metadata: resources.ObjectMeta{Name: "test"},
			Spec:     resources.McpServerSpec{Transport: "stdio"},
		}
		if err := s.Normalize(); err == nil {
			t.Fatal("expected error for missing command")
		}
	})

	t.Run("http_missing_endpoint", func(t *testing.T) {
		s := resources.McpServer{
			Metadata: resources.ObjectMeta{Name: "test"},
			Spec:     resources.McpServerSpec{Transport: "http"},
		}
		if err := s.Normalize(); err == nil {
			t.Fatal("expected error for missing endpoint")
		}
	})
}

func TestMcpToolTypeValidation(t *testing.T) {
	t.Run("mcp_type_accepted", func(t *testing.T) {
		tool := resources.Tool{
			Metadata: resources.ObjectMeta{Name: "test-tool"},
			Spec: resources.ToolSpec{
				Type:         "mcp",
				McpServerRef: "test-server",
				McpToolName:  "test",
			},
		}
		if err := tool.Normalize(); err != nil {
			t.Fatalf("normalize failed: %v", err)
		}
	})

	t.Run("mcp_missing_server_ref", func(t *testing.T) {
		tool := resources.Tool{
			Metadata: resources.ObjectMeta{Name: "test-tool"},
			Spec: resources.ToolSpec{
				Type:        "mcp",
				McpToolName: "test",
			},
		}
		if err := tool.Normalize(); err == nil {
			t.Fatal("expected error for missing mcp_server_ref")
		}
	})

	t.Run("mcp_missing_tool_name", func(t *testing.T) {
		tool := resources.Tool{
			Metadata: resources.ObjectMeta{Name: "test-tool"},
			Spec: resources.ToolSpec{
				Type:         "mcp",
				McpServerRef: "test-server",
			},
		}
		if err := tool.Normalize(); err == nil {
			t.Fatal("expected error for missing mcp_tool_name")
		}
	})
}

func TestMcpSessionManager(t *testing.T) {
	server := resources.McpServer{
		APIVersion: "orloj.dev/v1",
		Kind:       "McpServer",
		Metadata:   resources.ObjectMeta{Name: "test-mcp", Namespace: "default"},
		Spec:       resources.McpServerSpec{Transport: "stdio", Command: "echo"},
	}
	_ = server.Normalize()

	mgr := NewMcpSessionManager(nil)
	defer mgr.Close()

	// Pre-populate with mock session
	mockTransport := &mockMcpTransport{tools: []McpToolDefinition{{Name: "test"}}}
	mgr.sessions["default/test-mcp"] = &McpSession{
		Transport:  mockTransport,
		InitResult: &McpInitResult{ProtocolVersion: "2025-03-26"},
		ServerName: "test-mcp",
	}

	session, err := mgr.GetOrCreate(context.Background(), server)
	if err != nil {
		t.Fatalf("get or create: %v", err)
	}
	if session.ServerName != "test-mcp" {
		t.Errorf("expected test-mcp, got %q", session.ServerName)
	}

	mgr.Remove(server)
	if _, ok := mgr.sessions["default/test-mcp"]; ok {
		t.Error("expected session to be removed")
	}
}

func TestBuildOpenAIChatToolsWithSchemas(t *testing.T) {
	schemas := map[string]ToolSchemaInfo{
		"my-tool": {
			Description: "Custom tool description",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query": map[string]any{"type": "string"},
				},
			},
		},
	}

	tools := buildOpenAIChatTools([]string{"my-tool", "generic-tool"}, schemas)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}

	var myTool, genericTool openAIChatTool
	for _, tool := range tools {
		switch tool.Function.Name {
		case "my-tool":
			myTool = tool
		case "generic-tool":
			genericTool = tool
		}
	}

	if myTool.Function.Description != "Custom tool description" {
		t.Errorf("expected custom description, got %q", myTool.Function.Description)
	}
	if _, ok := myTool.Function.Parameters["properties"].(map[string]any)["query"]; !ok {
		t.Error("expected query property in schema")
	}

	if !strings.HasPrefix(genericTool.Function.Description, "Invoke tool") {
		t.Errorf("expected generic description, got %q", genericTool.Function.Description)
	}
	if _, ok := genericTool.Function.Parameters["properties"].(map[string]any)["input"]; !ok {
		t.Error("expected generic input property")
	}
}

func TestParseToolInputAsArguments(t *testing.T) {
	t.Run("valid_json", func(t *testing.T) {
		args := parseToolInputAsArguments(`{"key": "value"}`)
		if args["key"] != "value" {
			t.Errorf("expected key=value, got %v", args)
		}
	})

	t.Run("plain_string", func(t *testing.T) {
		args := parseToolInputAsArguments("hello world")
		if args["input"] != "hello world" {
			t.Errorf("expected input='hello world', got %v", args)
		}
	})

	t.Run("empty_string", func(t *testing.T) {
		args := parseToolInputAsArguments("")
		if args != nil {
			t.Errorf("expected nil for empty input, got %v", args)
		}
	})
}

func TestMcpServerManifestParse(t *testing.T) {
	t.Run("json", func(t *testing.T) {
		data := []byte(`{
			"apiVersion": "orloj.dev/v1",
			"kind": "McpServer",
			"metadata": {"name": "github-mcp", "namespace": "default"},
			"spec": {
				"transport": "stdio",
				"command": "npx @github/mcp-server",
				"args": ["--token-from-env"],
				"env": [{"name": "GITHUB_TOKEN", "secretRef": "github-token"}],
				"tool_filter": {"include": ["create_issue"]},
				"reconnect": {"max_attempts": 5, "backoff": "3s"}
			}
		}`)
		server, err := resources.ParseMcpServerManifest(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if server.Metadata.Name != "github-mcp" {
			t.Errorf("expected github-mcp, got %q", server.Metadata.Name)
		}
		if server.Spec.Transport != "stdio" {
			t.Errorf("expected stdio, got %q", server.Spec.Transport)
		}
		if len(server.Spec.Args) != 1 || server.Spec.Args[0] != "--token-from-env" {
			t.Errorf("expected args=[--token-from-env], got %v", server.Spec.Args)
		}
		if len(server.Spec.Env) != 1 || server.Spec.Env[0].SecretRef != "github-token" {
			t.Errorf("expected env with secretRef, got %v", server.Spec.Env)
		}
		if len(server.Spec.ToolFilter.Include) != 1 || server.Spec.ToolFilter.Include[0] != "create_issue" {
			t.Errorf("expected tool_filter include=[create_issue], got %v", server.Spec.ToolFilter.Include)
		}
		if server.Spec.Reconnect.MaxAttempts != 5 {
			t.Errorf("expected max_attempts=5, got %d", server.Spec.Reconnect.MaxAttempts)
		}
	})

	t.Run("http_json", func(t *testing.T) {
		data := []byte(`{
			"kind": "McpServer",
			"metadata": {"name": "remote-mcp"},
			"spec": {
				"transport": "http",
				"endpoint": "https://mcp.example.com/rpc",
				"auth": {"secretRef": "mcp-key", "profile": "bearer"}
			}
		}`)
		server, err := resources.ParseMcpServerManifest(data)
		if err != nil {
			t.Fatalf("parse failed: %v", err)
		}
		if server.Spec.Endpoint != "https://mcp.example.com/rpc" {
			t.Errorf("expected endpoint, got %q", server.Spec.Endpoint)
		}
		if server.Spec.Auth.SecretRef != "mcp-key" {
			t.Errorf("expected auth secretRef, got %q", server.Spec.Auth.SecretRef)
		}
	})
}

func TestMcpSchemaInModelRequest(t *testing.T) {
	schemas := map[string]ToolSchemaInfo{
		"mcp-tool": {
			Description: "MCP generated tool",
			InputSchema: map[string]any{"type": "object"},
		},
	}
	req := ModelRequest{
		Tools:       []string{"mcp-tool"},
		ToolSchemas: schemas,
	}
	if len(req.ToolSchemas) != 1 {
		t.Fatalf("expected 1 schema, got %d", len(req.ToolSchemas))
	}
	info := req.ToolSchemas["mcp-tool"]
	if info.Description != "MCP generated tool" {
		t.Errorf("expected description, got %q", info.Description)
	}
}

func TestGovernedToolRuntimeMcpDispatch(t *testing.T) {
	mockTransport := &mockMcpTransport{
		tools: []McpToolDefinition{{Name: "test_tool"}},
		callResults: map[string]*McpToolResult{
			"test_tool": {Content: []McpContent{{Type: "text", Text: "mcp-result"}}},
		},
	}

	specs := map[string]resources.ToolSpec{
		"test-tool": {
			Type:         "mcp",
			McpServerRef: "test-server",
			McpToolName:  "test_tool",
		},
	}
	registry := NewStaticToolCapabilityRegistry(specs)

	server := resources.McpServer{
		Metadata: resources.ObjectMeta{Name: "test-server", Namespace: "default"},
		Spec:     resources.McpServerSpec{Transport: "stdio", Command: "echo"},
	}
	_ = server.Normalize()

	mcpServerStore := store.NewMcpServerStore()
	_, _ = mcpServerStore.Upsert(context.Background(), server)

	sessionMgr := NewMcpSessionManager(nil)
	defer sessionMgr.Close()
	sessionMgr.sessions["default/test-server"] = &McpSession{
		Transport:  mockTransport,
		InitResult: &McpInitResult{},
		ServerName: "test-server",
	}

	mcpRuntime := NewMCPToolRuntime(registry, sessionMgr, mcpServerStore)
	governed := NewGovernedToolRuntimeWithAuthorizer(nil, nil, registry, nil, true)
	governed.SetMcpRuntime(mcpRuntime)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := governed.Call(ctx, "test-tool", `{"arg": "value"}`)
	if err != nil {
		t.Fatalf("governed call failed: %v", err)
	}
	if result != "mcp-result" {
		t.Errorf("expected mcp-result, got %q", result)
	}
}
