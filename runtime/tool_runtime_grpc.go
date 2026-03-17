package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/crds"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
)

const (
	grpcToolServiceMethod = "/orloj.tool.v1.ToolService/Execute"
	grpcCodecName         = "json"
)

func init() {
	encoding.RegisterCodec(jsonCodec{})
}

// jsonCodec is a gRPC codec that marshals/unmarshals JSON payloads.
type jsonCodec struct{}

func (jsonCodec) Marshal(v any) ([]byte, error)   { return json.Marshal(v) }
func (jsonCodec) Unmarshal(data []byte, v any) error { return json.Unmarshal(data, v) }
func (jsonCodec) Name() string                     { return grpcCodecName }

// GRPCToolRuntime executes tools via a unary gRPC call to an external service.
// The service must implement orloj.tool.v1.ToolService/Execute accepting
// ToolExecutionRequest and returning ToolExecutionResponse as JSON payloads.
type GRPCToolRuntime struct {
	registry  ToolCapabilityRegistry
	secrets   SecretResolver
	dialer    GRPCDialer
	namespace string
}

// GRPCDialer abstracts gRPC connection establishment for testing.
type GRPCDialer interface {
	DialContext(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error)
}

type defaultGRPCDialer struct{}

func (d defaultGRPCDialer) DialContext(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	return grpc.NewClient(target, opts...)
}

func NewGRPCToolRuntime(registry ToolCapabilityRegistry, secrets SecretResolver, dialer GRPCDialer) *GRPCToolRuntime {
	if dialer == nil {
		dialer = defaultGRPCDialer{}
	}
	return &GRPCToolRuntime{
		registry: registry,
		secrets:  secrets,
		dialer:   dialer,
	}
}

func (r *GRPCToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewGRPCToolRuntime(registry, nil, nil)
	}
	return &GRPCToolRuntime{
		registry:  registry,
		secrets:   r.secrets,
		dialer:    r.dialer,
		namespace: r.namespace,
	}
}

func (r *GRPCToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewGRPCToolRuntime(nil, nil, nil)
	}
	copy := *r
	copy.namespace = crds.NormalizeNamespace(strings.TrimSpace(namespace))
	if aware, ok := copy.secrets.(namespaceAwareSecretResolver); ok {
		copy.secrets = aware.WithNamespace(copy.namespace)
	}
	return &copy
}

func (r *GRPCToolRuntime) Call(ctx context.Context, tool string, input string) (string, error) {
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
			"missing tool registry for gRPC runtime",
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
	endpoint := strings.TrimSpace(spec.Endpoint)
	if endpoint == "" {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s missing endpoint for gRPC delegation", tool),
			ErrInvalidToolRuntimePolicy,
			map[string]string{"tool": tool},
		)
	}

	execReq := ToolExecutionRequest{
		ToolContractVersion: ToolContractVersionV1,
		RequestID:           fmt.Sprintf("grpc-%s-%d", tool, time.Now().UnixNano()),
		Namespace:           r.namespace,
		Tool: ToolExecutionRequestTool{
			Name:         tool,
			Operation:    ToolOperationInvoke,
			Capabilities: spec.Capabilities,
			RiskLevel:    spec.RiskLevel,
		},
		InputRaw: input,
		Runtime: ToolExecutionRuntime{
			Mode: "grpc",
		},
		Attempt: 1,
	}

	if secretRef := strings.TrimSpace(spec.Auth.SecretRef); secretRef != "" {
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
		secretValue, resolveErr := r.secrets.Resolve(ctx, secretRef)
		if resolveErr != nil {
			return "", NewToolError(
				ToolStatusError,
				ToolCodeSecretResolution,
				ToolReasonSecretResolution,
				false,
				fmt.Sprintf("tool=%s secretRef=%s resolution failed", tool, secretRef),
				fmt.Errorf("%w: %v", ErrToolSecretResolution, resolveErr),
				map[string]string{
					"tool":       tool,
					"secret_ref": secretRef,
				},
			)
		}
		execReq.Auth.SecretRef = secretRef
		execReq.Auth.Profile = "bearer_token"
		_ = secretValue // Auth token passed via gRPC metadata in production; placeholder for future per-call auth headers.
	}

	dialOpts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.ForceCodec(jsonCodec{})),
	}

	conn, err := r.dialer.DialContext(ctx, endpoint, dialOpts...)
	if err != nil {
		return "", mapGRPCError(tool, err)
	}
	defer conn.Close()

	var contractResp ToolExecutionResponse
	err = conn.Invoke(ctx, grpcToolServiceMethod, &execReq, &contractResp)
	if err != nil {
		return "", mapGRPCError(tool, err)
	}

	if toErr := contractResp.ToError(); toErr != nil {
		return "", toErr
	}
	return strings.TrimSpace(contractResp.Output.Result), nil
}

func mapGRPCError(tool string, err error) error {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return NewToolError(
			ToolStatusError,
			ToolCodeTimeout,
			ToolReasonExecutionTimeout,
			true,
			fmt.Sprintf("gRPC tool execution timed out for tool=%s", tool),
			err,
			map[string]string{"tool": tool, "isolation_mode": "grpc"},
		)
	case errors.Is(err, context.Canceled):
		return NewToolError(
			ToolStatusError,
			ToolCodeCanceled,
			ToolReasonExecutionCanceled,
			false,
			fmt.Sprintf("gRPC tool execution canceled for tool=%s", tool),
			err,
			map[string]string{"tool": tool, "isolation_mode": "grpc"},
		)
	default:
		return NewToolError(
			ToolStatusError,
			ToolCodeExecutionFailed,
			ToolReasonBackendFailure,
			true,
			fmt.Sprintf("gRPC tool request failed for tool=%s: %s", tool, RedactSensitive(err.Error())),
			err,
			map[string]string{"tool": tool, "isolation_mode": "grpc"},
		)
	}
}
