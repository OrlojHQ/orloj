package api

import (
	a2agrpc "github.com/a2aproject/a2a-go/v2/a2agrpc/v1"
	"google.golang.org/grpc"
)

// RegisterA2AGRPC registers the normative A2A v1 service on a gRPC server.
// JSON-RPC and gRPC share the same transport-neutral Orloj handler.
func (s *Server) RegisterA2AGRPC(grpcServer *grpc.Server) {
	if grpcServer == nil {
		return
	}
	a2agrpc.NewHandler(&a2aV1Handler{server: s}).RegisterWith(grpcServer)
}
