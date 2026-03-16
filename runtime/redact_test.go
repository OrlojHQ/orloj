package agentruntime

import (
	"strings"
	"testing"
)

func TestRedactSensitive(t *testing.T) {
	tests := []struct {
		name  string
		input string
		must  []string
		deny  []string
	}{
		{
			name:  "bearer token",
			input: "Authorization: Bearer sk-abc123secret",
			must:  []string{"[REDACTED]"},
			deny:  []string{"sk-abc123secret"},
		},
		{
			name:  "api key header",
			input: "X-Api-Key: my-super-secret-key",
			must:  []string{"[REDACTED]"},
			deny:  []string{"my-super-secret-key"},
		},
		{
			name:  "key=value pair",
			input: "api_key=sk-proj-1234567890abcdef",
			must:  []string{"[REDACTED]"},
			deny:  []string{"sk-proj-1234567890abcdef"},
		},
		{
			name:  "openai key pattern",
			input: "using key sk-1234567890abcdef for request",
			must:  []string{"[REDACTED]"},
			deny:  []string{"sk-1234567890abcdef"},
		},
		{
			name:  "password in output",
			input: "password=hunter2 connecting",
			must:  []string{"[REDACTED]"},
			deny:  []string{"hunter2"},
		},
		{
			name:  "safe content unchanged",
			input: "curl: (7) Failed to connect to host port 443",
			must:  []string{"curl", "443"},
			deny:  []string{"[REDACTED]"},
		},
		{
			name:  "empty string",
			input: "",
			must:  []string{""},
			deny:  nil,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := RedactSensitive(tc.input)
			for _, m := range tc.must {
				if !strings.Contains(got, m) {
					t.Errorf("expected %q in output, got %q", m, got)
				}
			}
			for _, d := range tc.deny {
				if strings.Contains(got, d) {
					t.Errorf("unexpected %q in output, got %q", d, got)
				}
			}
		})
	}
}
