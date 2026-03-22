package agentruntime

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// StdioMcpTransport communicates with an MCP server via a child process's
// stdin/stdout using newline-delimited JSON-RPC 2.0 messages.
type StdioMcpTransport struct {
	command string
	args    []string
	env     []string

	mu      sync.Mutex
	cmd     *exec.Cmd
	stdin   io.WriteCloser
	scanner *bufio.Scanner
	ctx     context.Context

	pending   map[int64]chan jsonrpcResponse
	pendingMu sync.Mutex
	done      chan struct{}
}

// StdioMcpTransportConfig configures the child process to spawn.
type StdioMcpTransportConfig struct {
	Command string
	Args    []string
	Env     []string
}

func NewStdioMcpTransport(cfg StdioMcpTransportConfig) *StdioMcpTransport {
	return &StdioMcpTransport{
		command: cfg.Command,
		args:    cfg.Args,
		env:     cfg.Env,
		pending: make(map[int64]chan jsonrpcResponse),
		done:    make(chan struct{}),
	}
}

func (t *StdioMcpTransport) Initialize(ctx context.Context) (*McpInitResult, error) {
	if err := t.startProcess(ctx); err != nil {
		return nil, fmt.Errorf("mcp stdio: failed to start process: %w", err)
	}

	resp, err := t.sendRequest(ctx, "initialize", mcpInitializeParams{
		ProtocolVersion: "2025-03-26",
		ClientInfo:      McpServerInfo{Name: "orloj", Version: "1.0.0"},
		Capabilities:    map[string]any{},
	})
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: initialize failed: %w", err)
	}

	var result McpInitResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp stdio: failed to decode initialize result: %w", err)
	}

	if err := t.sendNotification("notifications/initialized", nil); err != nil {
		return nil, fmt.Errorf("mcp stdio: initialized notification failed: %w", err)
	}

	return &result, nil
}

func (t *StdioMcpTransport) ListTools(ctx context.Context) ([]McpToolDefinition, error) {
	var all []McpToolDefinition
	var cursor string
	for page := 0; page < maxToolsListPages; page++ {
		var params any
		if cursor != "" {
			params = mcpToolsListParams{Cursor: cursor}
		}
		resp, err := t.sendRequest(ctx, "tools/list", params)
		if err != nil {
			return nil, fmt.Errorf("mcp stdio: tools/list failed: %w", err)
		}
		var result mcpToolsListResult
		if err := json.Unmarshal(resp.Result, &result); err != nil {
			return nil, fmt.Errorf("mcp stdio: failed to decode tools/list result: %w", err)
		}
		all = append(all, result.Tools...)
		if result.NextCursor == "" {
			break
		}
		cursor = result.NextCursor
	}
	return all, nil
}

func (t *StdioMcpTransport) CallTool(ctx context.Context, name string, arguments map[string]any) (*McpToolResult, error) {
	resp, err := t.sendRequest(ctx, "tools/call", mcpToolsCallParams{
		Name:      name,
		Arguments: arguments,
	})
	if err != nil {
		return nil, fmt.Errorf("mcp stdio: tools/call %s failed: %w", name, err)
	}
	var result McpToolResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("mcp stdio: failed to decode tools/call result for %s: %w", name, err)
	}
	return &result, nil
}

func (t *StdioMcpTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin != nil {
		_ = t.stdin.Close()
	}
	if t.cmd != nil && t.cmd.Process != nil {
		_ = t.cmd.Process.Kill()
		_ = t.cmd.Wait()
	}
	select {
	case <-t.done:
	default:
		close(t.done)
	}
	return nil
}

func (t *StdioMcpTransport) startProcess(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	parts := strings.Fields(t.command)
	if len(parts) == 0 {
		return fmt.Errorf("empty command")
	}
	cmdName := parts[0]
	cmdArgs := append(parts[1:], t.args...)

	t.ctx = ctx
	cmd := exec.CommandContext(ctx, cmdName, cmdArgs...)
	if len(t.env) > 0 {
		cmd.Env = append(cmd.Environ(), t.env...)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return fmt.Errorf("stdin pipe: %w", err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return fmt.Errorf("stdout pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return fmt.Errorf("start command %q: %w", t.command, err)
	}

	t.cmd = cmd
	t.stdin = stdin
	t.scanner = bufio.NewScanner(stdout)
	t.scanner.Buffer(make([]byte, 0, 1024*1024), 10*1024*1024)

	go t.readLoop()

	return nil
}

func (t *StdioMcpTransport) readLoop() {
	defer func() {
		select {
		case <-t.done:
		default:
			close(t.done)
		}
		t.pendingMu.Lock()
		for id, ch := range t.pending {
			close(ch)
			delete(t.pending, id)
		}
		t.pendingMu.Unlock()
	}()

	for t.scanner.Scan() {
		line := t.scanner.Bytes()
		if len(line) == 0 {
			continue
		}

		var resp jsonrpcResponse
		if err := json.Unmarshal(line, &resp); err != nil {
			continue
		}

		if len(resp.ID) == 0 || string(resp.ID) == "null" {
			continue
		}

		var id int64
		if err := json.Unmarshal(resp.ID, &id); err != nil {
			continue
		}

		t.pendingMu.Lock()
		ch, ok := t.pending[id]
		if ok {
			delete(t.pending, id)
		}
		t.pendingMu.Unlock()

		if ok {
			ch <- resp
		}
	}
}

func (t *StdioMcpTransport) sendRequest(ctx context.Context, method string, params any) (jsonrpcResponse, error) {
	id := nextMcpRequestID()
	req := jsonrpcRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  params,
	}

	ch := make(chan jsonrpcResponse, 1)
	t.pendingMu.Lock()
	t.pending[id] = ch
	t.pendingMu.Unlock()

	if err := t.writeMessage(req); err != nil {
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
		return jsonrpcResponse{}, err
	}

	select {
	case <-ctx.Done():
		t.pendingMu.Lock()
		delete(t.pending, id)
		t.pendingMu.Unlock()
		return jsonrpcResponse{}, ctx.Err()
	case <-t.done:
		return jsonrpcResponse{}, fmt.Errorf("mcp stdio: process exited")
	case resp, ok := <-ch:
		if !ok {
			return jsonrpcResponse{}, fmt.Errorf("mcp stdio: process exited while waiting for response")
		}
		if resp.Error != nil {
			return resp, resp.Error
		}
		return resp, nil
	}
}

func (t *StdioMcpTransport) sendNotification(method string, params any) error {
	notif := jsonrpcNotification{
		JSONRPC: "2.0",
		Method:  method,
		Params:  params,
	}
	return t.writeMessage(notif)
}

func (t *StdioMcpTransport) writeMessage(msg any) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("marshal message: %w", err)
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.stdin == nil {
		return fmt.Errorf("mcp stdio: process not started")
	}
	data = append(data, '\n')
	_, err = t.stdin.Write(data)
	return err
}
