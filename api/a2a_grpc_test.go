package api_test

import (
	"context"
	"io"
	"log"
	"net"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
	lf "github.com/a2aproject/a2a-go/v2/a2a"
	"github.com/a2aproject/a2a-go/v2/a2aclient"
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	a2apb "github.com/a2aproject/a2a-go/v2/a2apb/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

func newA2AGRPCTestServer(t *testing.T) (a2aclient.Transport, api.Stores) {
	t.Helper()
	t.Setenv("ORLOJ_API_TOKENS", "grpc-client:grpc-token:a2a:default/v1-system|team/v1-system")
	t.Setenv("ORLOJ_API_TOKEN", "")

	stores := api.Stores{
		Agents:       store.NewAgentStore(),
		AgentSystems: store.NewAgentSystemStore(),
		Tools:        store.NewToolStore(),
		Tasks:        store.NewTaskStore(),
		Workers:      store.NewWorkerStore(),
		Memories:     store.NewMemoryStore(),
		Policies:     store.NewAgentPolicyStore(),
	}
	logger := log.New(io.Discard, "", 0)
	server := api.NewServerWithOptions(stores, agentruntime.NewManager(logger), logger, api.ServerOptions{})
	server.SetA2AConfig(&api.A2AConfig{
		PublicBaseURL:    "https://test.example.com",
		GRPCPublicURL:    "https://test.example.com:8443",
		ProtocolVersion:  "1.0",
		StreamingEnabled: true,
		AuthSchemes:      []string{"bearer"},
	})

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := grpc.NewServer()
	server.RegisterA2AGRPC(grpcServer)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(func() {
		grpcServer.Stop()
		_ = listener.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create gRPC client: %v", err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return a2agrpc.NewGRPCTransportFromClient(a2apb.NewA2AServiceClient(conn)), stores
}

func TestA2AGRPCSendAndGetTask(t *testing.T) {
	transport, stores := newA2AGRPCTestServer(t)
	seedAgent(t, stores, "v1-system", "A v1 gRPC agent", nil)
	auth := a2aclient.ServiceParams{"authorization": {"Bearer grpc-token"}}

	result, err := transport.SendMessage(context.Background(), auth, &lf.SendMessageRequest{
		Tenant: "default/v1-system",
		Message: &lf.Message{
			ID:    "grpc-message-1",
			Role:  lf.MessageRoleUser,
			Parts: lf.ContentParts{lf.NewTextPart("do the work over gRPC")},
		},
		Config: &lf.SendMessageConfig{ReturnImmediately: true},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	task, ok := result.(*lf.Task)
	if !ok {
		t.Fatalf("SendMessage() result type = %T, want *a2a.Task", result)
	}
	if task.ID == "" || task.ContextID == "" || task.Status.State != lf.TaskStateSubmitted {
		t.Fatalf("unexpected task: %#v", task)
	}

	got, err := transport.GetTask(context.Background(), auth, &lf.GetTaskRequest{ID: task.ID})
	if err != nil {
		t.Fatalf("GetTask() error = %v", err)
	}
	if got.ID != task.ID || got.ContextID != task.ContextID {
		t.Fatalf("GetTask() = (%q, %q), want (%q, %q)", got.ID, got.ContextID, task.ID, task.ContextID)
	}
}

func TestA2AGRPCRejectsMissingBearerForProtectedSystem(t *testing.T) {
	transport, stores := newA2AGRPCTestServer(t)
	seedAgent(t, stores, "v1-system", "A protected gRPC agent", nil)

	_, err := transport.SendMessage(context.Background(), nil, &lf.SendMessageRequest{
		Tenant: "default/v1-system",
		Message: &lf.Message{
			ID:    "grpc-message-1",
			Role:  lf.MessageRoleUser,
			Parts: lf.ContentParts{lf.NewTextPart("unauthorized")},
		},
		Config: &lf.SendMessageConfig{ReturnImmediately: true},
	})
	if err == nil {
		t.Fatal("expected protected AgentSystem to reject unauthenticated gRPC call")
	}
}

func TestA2AGRPCTenantSelectsNamespacedSystem(t *testing.T) {
	transport, stores := newA2AGRPCTestServer(t)
	seedAgentInNamespace(t, stores, "team", "v1-system", "A namespaced gRPC agent", nil)
	auth := a2aclient.ServiceParams{"authorization": {"Bearer grpc-token"}}

	result, err := transport.SendMessage(context.Background(), auth, &lf.SendMessageRequest{
		Tenant: "team/v1-system",
		Message: &lf.Message{
			ID:    "grpc-team-message",
			Role:  lf.MessageRoleUser,
			Parts: lf.ContentParts{lf.NewTextPart("run in team namespace")},
		},
		Config: &lf.SendMessageConfig{ReturnImmediately: true},
	})
	if err != nil {
		t.Fatalf("SendMessage() error = %v", err)
	}
	task := result.(*lf.Task)
	items, err := stores.Tasks.List(context.Background())
	if err != nil {
		t.Fatalf("list tasks: %v", err)
	}
	if len(items) != 1 || items[0].Metadata.Namespace != "team" {
		t.Fatalf("stored tasks = %#v, want one task in team namespace", items)
	}
	if items[0].Metadata.Labels["orloj.dev/a2a-task-id"] != string(task.ID) {
		t.Fatalf("stored A2A task ID does not match gRPC response")
	}
}
