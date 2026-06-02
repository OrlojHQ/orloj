package agentruntime

import (
	"context"
	"net"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
	"google.golang.org/grpc"
)

type blockingGRPCDialer struct {
	resolvedIP string
}

func (d blockingGRPCDialer) DialContext(ctx context.Context, target string, opts ...grpc.DialOption) (*grpc.ClientConn, error) {
	safeDialer := SafeDialer(false)
	_, err := safeDialer.DialContext(ctx, "tcp", net.JoinHostPort(d.resolvedIP, "50051"))
	return nil, err
}

func TestGRPCToolRuntimeBlocksHostnameRebindToLoopback(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"grpc_tool": {
			Type:     "grpc",
			Endpoint: "rebind.example:50051",
		},
	})
	runtime := NewGRPCToolRuntime(registry, nil, blockingGRPCDialer{resolvedIP: "127.0.0.1"})
	runtime.SetAllowInsecure(true)

	_, err := runtime.Call(context.Background(), "grpc_tool", "input")
	if err == nil {
		t.Fatal("expected blocked loopback dial")
	}
	code, _, _, _ := ToolErrorMeta(err)
	if code != ToolCodeRuntimePolicyInvalid && code != ToolCodeExecutionFailed {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGRPCToolRuntimeBlocksHostnameRebindToMetadata(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"grpc_tool": {
			Type:     "grpc",
			Endpoint: "metadata.example:50051",
		},
	})
	runtime := NewGRPCToolRuntime(registry, nil, blockingGRPCDialer{resolvedIP: "169.254.169.254"})
	runtime.SetAllowInsecure(true)

	_, err := runtime.Call(context.Background(), "grpc_tool", "input")
	if err == nil {
		t.Fatal("expected blocked metadata IP dial")
	}
}

func TestGRPCToolRuntimeBlocksHostnameRebindToPrivate(t *testing.T) {
	registry := NewStaticToolCapabilityRegistry(map[string]resources.ToolSpec{
		"grpc_tool": {
			Type:     "grpc",
			Endpoint: "private.example:50051",
		},
	})
	runtime := NewGRPCToolRuntime(registry, nil, blockingGRPCDialer{resolvedIP: "10.0.0.5"})
	runtime.SetAllowInsecure(true)

	_, err := runtime.Call(context.Background(), "grpc_tool", "input")
	if err == nil {
		t.Fatal("expected blocked private IP dial")
	}
}
