package agentruntime

import (
	"strings"
	"testing"
)

func TestBuildWASMToolModuleRequest(t *testing.T) {
	req := BuildWASMToolModuleRequest(WASMToolExecuteRequest{
		Namespace:    "default",
		Tool:         "wasm_tool",
		Input:        "{\"q\":\"hi\"}",
		Capabilities: []string{"web.search", "web.search", " vector.db.invoke "},
		RiskLevel:    "HIGH",
		Runtime: WASMToolRuntimeConfig{
			Entrypoint:     "run_tool",
			MaxMemoryBytes: 32 * 1024 * 1024,
			Fuel:           1000,
			EnableWASI:     true,
		},
	})
	if req.ContractVersion != WASMToolModuleContractVersionV1 {
		t.Fatalf("unexpected contract version %q", req.ContractVersion)
	}
	if req.Tool != "wasm_tool" {
		t.Fatalf("unexpected tool %q", req.Tool)
	}
	if req.RiskLevel != "high" {
		t.Fatalf("unexpected risk level %q", req.RiskLevel)
	}
	if len(req.Capabilities) != 2 {
		t.Fatalf("expected deduped capabilities length 2, got %d (%v)", len(req.Capabilities), req.Capabilities)
	}
	if req.Runtime.Entrypoint != "run_tool" {
		t.Fatalf("unexpected runtime entrypoint %q", req.Runtime.Entrypoint)
	}
}

func TestDecodeWASMToolModuleResponse(t *testing.T) {
	resp, err := DecodeWASMToolModuleResponse(`{"contract_version":"v1","status":"ok","output":"ok"}`)
	if err != nil {
		t.Fatalf("decode failed: %v", err)
	}
	if resp.Status != wasmToolModuleStatusOK {
		t.Fatalf("unexpected status %q", resp.Status)
	}
	if strings.TrimSpace(resp.Output) != "ok" {
		t.Fatalf("unexpected output %q", resp.Output)
	}
}

func TestDecodeWASMToolModuleResponseInvalidVersion(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse(`{"contract_version":"v2","status":"ok","output":"ok"}`)
	if err == nil {
		t.Fatal("expected contract error")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}

func TestDecodeWASMToolModuleResponseInvalidStatus(t *testing.T) {
	_, err := DecodeWASMToolModuleResponse(`{"contract_version":"v1","status":"weird","output":"ok"}`)
	if err == nil {
		t.Fatal("expected contract error")
	}
	if !IsWASMToolModuleContractError(err) {
		t.Fatalf("expected contract error classification, got %v", err)
	}
}
