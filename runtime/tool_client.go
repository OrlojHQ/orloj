package agentruntime

import (
	"context"
	"fmt"
)

// MockToolClient is an in-process placeholder for external tool systems.
type MockToolClient struct{}

func (m *MockToolClient) Call(_ context.Context, tool string, input string) (string, error) {
	return fmt.Sprintf("tool=%s input=%s", tool, input), nil
}
