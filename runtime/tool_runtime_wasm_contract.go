package agentruntime

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

const WASMToolModuleContractVersionV1 = "v1"

const (
	wasmToolModuleStatusOK     = ToolExecutionStatusOK
	wasmToolModuleStatusError  = ToolExecutionStatusError
	wasmToolModuleStatusDenied = ToolExecutionStatusDenied
)

var errWASMToolModuleContract = errors.New("wasm tool module contract violation")

type WASMToolModuleRequest struct {
	ContractVersion string                   `json:"contract_version,omitempty"`
	Namespace       string                   `json:"namespace,omitempty"`
	Tool            string                   `json:"tool,omitempty"`
	Input           string                   `json:"input,omitempty"`
	Capabilities    []string                 `json:"capabilities,omitempty"`
	RiskLevel       string                   `json:"risk_level,omitempty"`
	Runtime         WASMToolModuleReqRuntime `json:"runtime,omitempty"`
}

type WASMToolModuleReqRuntime struct {
	Entrypoint     string `json:"entrypoint,omitempty"`
	MaxMemoryBytes int64  `json:"max_memory_bytes,omitempty"`
	Fuel           uint64 `json:"fuel,omitempty"`
	EnableWASI     bool   `json:"enable_wasi"`
}

type WASMToolModuleResponse struct {
	ContractVersion string                `json:"contract_version,omitempty"`
	Status          string                `json:"status,omitempty"`
	Output          string                `json:"output,omitempty"`
	Error           *ToolExecutionFailure `json:"error,omitempty"`
}

func BuildWASMToolModuleRequest(req WASMToolExecuteRequest) WASMToolModuleRequest {
	runtime := req.Runtime.normalized()
	return WASMToolModuleRequest{
		ContractVersion: WASMToolModuleContractVersionV1,
		Namespace:       strings.TrimSpace(req.Namespace),
		Tool:            strings.TrimSpace(req.Tool),
		Input:           req.Input,
		Capabilities:    normalizeToolContractLabels(req.Capabilities),
		RiskLevel:       strings.ToLower(strings.TrimSpace(req.RiskLevel)),
		Runtime: WASMToolModuleReqRuntime{
			Entrypoint:     strings.TrimSpace(runtime.Entrypoint),
			MaxMemoryBytes: runtime.MaxMemoryBytes,
			Fuel:           runtime.Fuel,
			EnableWASI:     runtime.EnableWASI,
		},
	}
}

func DecodeWASMToolModuleResponse(raw string) (WASMToolModuleResponse, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return WASMToolModuleResponse{}, newWASMToolModuleContractError("empty wasm module response")
	}

	var response WASMToolModuleResponse
	if err := json.Unmarshal([]byte(trimmed), &response); err != nil {
		return WASMToolModuleResponse{}, newWASMToolModuleContractError(fmt.Sprintf("invalid wasm module response JSON: %v", err))
	}

	response.ContractVersion = strings.ToLower(strings.TrimSpace(response.ContractVersion))
	if response.ContractVersion == "" {
		return WASMToolModuleResponse{}, newWASMToolModuleContractError("missing response contract_version")
	}
	if response.ContractVersion != WASMToolModuleContractVersionV1 {
		return WASMToolModuleResponse{}, newWASMToolModuleContractError(
			fmt.Sprintf("unsupported wasm module contract_version %q", response.ContractVersion),
		)
	}

	response.Status = strings.ToLower(strings.TrimSpace(response.Status))
	switch response.Status {
	case wasmToolModuleStatusOK, wasmToolModuleStatusError, wasmToolModuleStatusDenied:
	default:
		return WASMToolModuleResponse{}, newWASMToolModuleContractError(
			fmt.Sprintf("unsupported wasm module status %q", response.Status),
		)
	}

	if response.Error != nil {
		response.Error.Code = strings.TrimSpace(response.Error.Code)
		response.Error.Reason = strings.TrimSpace(response.Error.Reason)
		response.Error.Message = strings.TrimSpace(response.Error.Message)
		if response.Error.Details == nil {
			response.Error.Details = nil
		}
	}

	return response, nil
}

func IsWASMToolModuleContractError(err error) bool {
	return errors.Is(err, errWASMToolModuleContract)
}

func newWASMToolModuleContractError(message string) error {
	return fmt.Errorf("%w: %s", errWASMToolModuleContract, strings.TrimSpace(message))
}
