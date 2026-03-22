package agentruntime

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding"
	"google.golang.org/grpc/status"
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
	registry      ToolCapabilityRegistry
	secrets       SecretResolver
	authInjector  *AuthInjector
	dialer        GRPCDialer
	namespace     string
	allowInsecure bool // opt-in to plaintext gRPC; defaults to TLS
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
		registry:     registry,
		secrets:      secrets,
		authInjector: NewAuthInjector(secrets, nil),
		dialer:       dialer,
	}
}

// SetAllowInsecure enables plaintext gRPC connections. This should only be
// used in development or when the transport is otherwise secured (e.g.
// service mesh with mTLS). Callers must explicitly opt in.
func (r *GRPCToolRuntime) SetAllowInsecure(allow bool) {
	r.allowInsecure = allow
}

func (r *GRPCToolRuntime) WithRegistry(registry ToolCapabilityRegistry) ToolRuntime {
	if r == nil {
		return NewGRPCToolRuntime(registry, nil, nil)
	}
	return &GRPCToolRuntime{
		registry:      registry,
		secrets:       r.secrets,
		authInjector:  r.authInjector,
		dialer:        r.dialer,
		namespace:     r.namespace,
		allowInsecure: r.allowInsecure,
	}
}

func (r *GRPCToolRuntime) WithNamespace(namespace string) ToolRuntime {
	if r == nil {
		return NewGRPCToolRuntime(nil, nil, nil)
	}
	cp := *r
	cp.namespace = resources.NormalizeNamespace(strings.TrimSpace(namespace))
	if aware, ok := cp.secrets.(namespaceAwareSecretResolver); ok {
		cp.secrets = aware.WithNamespace(cp.namespace)
	}
	cp.authInjector = NewAuthInjector(cp.secrets, nil)
	if r.authInjector != nil {
		cp.authInjector.tokenCache = r.authInjector.tokenCache
	}
	return &cp
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

	if r.authInjector != nil {
		authResult, authErr := r.authInjector.Resolve(ctx, tool, spec.Auth)
		if authErr != nil {
			return "", authErr
		}
		if authResult.Profile != "" {
			execReq.Auth.Profile = authResult.Profile
			execReq.Auth.SecretRef = strings.TrimSpace(spec.Auth.SecretRef)
		}
	}

	if err := ValidateEndpointURL("grpc://"+endpoint, false); err != nil {
		return "", NewToolError(
			ToolStatusError,
			ToolCodeRuntimePolicyInvalid,
			ToolReasonRuntimePolicyInvalid,
			false,
			fmt.Sprintf("tool=%s gRPC endpoint blocked: %s", tool, err),
			err,
			map[string]string{"tool": tool},
		)
	}

	var transportCreds grpc.DialOption
	if r.allowInsecure {
		transportCreds = grpc.WithTransportCredentials(insecure.NewCredentials())
	} else {
		transportCreds = grpc.WithTransportCredentials(credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12}))
	}
	dialOpts := []grpc.DialOption{
		transportCreds,
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
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.Unauthenticated:
				return NewToolError(
					ToolStatusError,
					ToolCodeAuthInvalid,
					ToolReasonAuthInvalid,
					false,
					fmt.Sprintf("gRPC tool auth failed for tool=%s: %s", tool, RedactSensitive(st.Message())),
					err,
					map[string]string{"tool": tool, "isolation_mode": "grpc"},
				)
			case codes.PermissionDenied:
				return NewToolError(
					ToolStatusError,
					ToolCodeAuthForbidden,
					ToolReasonAuthForbidden,
					false,
					fmt.Sprintf("gRPC tool permission denied for tool=%s: %s", tool, RedactSensitive(st.Message())),
					err,
					map[string]string{"tool": tool, "isolation_mode": "grpc"},
				)
			}
		}
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
