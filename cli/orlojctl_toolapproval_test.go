package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/OrlojHQ/orloj/resources"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}

func withRoundTripper(t *testing.T, rt http.RoundTripper, fn func()) {
	t.Helper()
	oldTransport := http.DefaultTransport
	oldClient := http.DefaultClient
	http.DefaultTransport = rt
	defer func() {
		http.DefaultTransport = oldTransport
		http.DefaultClient = oldClient
	}()
	fn()
}

func mockResponse(req *http.Request, status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Header:     make(http.Header),
		Body:       io.NopCloser(bytes.NewReader([]byte(body))),
		Request:    req,
	}
}

func TestNormalizeResourceToolApproval(t *testing.T) {
	if got := normalizeResource("tool-approval"); got != "tool-approvals" {
		t.Fatalf("expected tool-approvals, got %q", got)
	}
	if got := normalizeResource("toolapprovals"); got != "tool-approvals" {
		t.Fatalf("expected tool-approvals, got %q", got)
	}
}

func TestListEndpointForToolApproval(t *testing.T) {
	endpoint, err := listEndpointForResource("tool-approvals")
	if err != nil {
		t.Fatalf("listEndpointForResource returned error: %v", err)
	}
	if endpoint != "/v1/tool-approvals" {
		t.Fatalf("unexpected endpoint %q", endpoint)
	}
}

func TestRunApproveToolApproval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var (
		gotMethod      string
		gotPath        string
		gotNamespace   string
		gotContentType string
		gotBody        map[string]string
	)
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw := []byte{}
		if r.Body != nil {
			raw, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotNamespace = r.URL.Query().Get("namespace")
		gotContentType = r.Header.Get("Content-Type")
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &gotBody)
		}
		return mockResponse(r, http.StatusOK, `{"ok":true}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{
				"approve",
				"tool-approval",
				"approval-1",
				"--server", "http://orloj.test",
				"--namespace", "team-a",
				"--decided-by", "ops-user",
				"--reason", "approved by on-call",
			})
		})
	})
	if err != nil {
		t.Fatalf("approve command failed: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/v1/tool-approvals/approval-1/approve" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if gotNamespace != "team-a" {
		t.Fatalf("expected namespace query team-a, got %q", gotNamespace)
	}
	if gotContentType != "application/json" {
		t.Fatalf("expected content-type application/json, got %q", gotContentType)
	}
	if gotBody["decided_by"] != "ops-user" {
		t.Fatalf("expected decided_by in body, got %v", gotBody)
	}
	if gotBody["reason"] != "approved by on-call" {
		t.Fatalf("expected reason in body, got %v", gotBody)
	}
	if !strings.Contains(out, "approved tool-approval/approval-1") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestRunDenyToolApproval(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	var (
		gotMethod string
		gotPath   string
		gotBody   []byte
	)
	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		raw := []byte{}
		if r.Body != nil {
			raw, _ = io.ReadAll(r.Body)
			_ = r.Body.Close()
		}
		gotMethod = r.Method
		gotPath = r.URL.Path
		gotBody = append([]byte(nil), raw...)
		return mockResponse(r, http.StatusOK, `{"ok":true}`), nil
	})

	var (
		out string
		err error
	)
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{
				"deny",
				"tool-approval",
				"approval-2",
				"--server", "http://orloj.test",
			})
		})
	})
	if err != nil {
		t.Fatalf("deny command failed: %v", err)
	}
	if gotMethod != http.MethodPost {
		t.Fatalf("expected POST, got %s", gotMethod)
	}
	if gotPath != "/v1/tool-approvals/approval-2/deny" {
		t.Fatalf("unexpected path %q", gotPath)
	}
	if len(gotBody) != 0 {
		t.Fatalf("expected empty body without flags, got %q", string(gotBody))
	}
	if !strings.Contains(out, "denied tool-approval/approval-2") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestRunGetToolApprovals(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	list := resources.ToolApprovalList{
		Items: []resources.ToolApproval{
			{
				Metadata: resources.ObjectMeta{Name: "approval-1"},
				Spec: resources.ToolApprovalSpec{
					TaskRef:        "task-1",
					Tool:           "web-search",
					OperationClass: "write",
					Agent:          "planner",
				},
				Status: resources.ToolApprovalStatus{
					Phase:     "Pending",
					ExpiresAt: "2026-03-30T12:00:00Z",
				},
			},
		},
	}
	raw, err := json.Marshal(list)
	if err != nil {
		t.Fatalf("marshal list: %v", err)
	}

	rt := roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Method != http.MethodGet {
			return mockResponse(r, http.StatusMethodNotAllowed, "method not allowed"), nil
		}
		if r.URL.Path != "/v1/tool-approvals" {
			return mockResponse(r, http.StatusNotFound, "not found"), nil
		}
		return mockResponse(r, http.StatusOK, string(raw)), nil
	})

	var out string
	withRoundTripper(t, rt, func() {
		out, err = captureStdout(t, func() error {
			return Run([]string{"get", "tool-approvals", "--server", "http://orloj.test"})
		})
	})
	if err != nil {
		t.Fatalf("get tool-approvals failed: %v", err)
	}
	required := []string{
		"NAME",
		"TASK",
		"TOOL",
		"approval-1",
		"task-1",
		"web-search",
		"Pending",
	}
	for _, token := range required {
		if !strings.Contains(out, token) {
			t.Fatalf("expected output to contain %q, got:\n%s", token, out)
		}
	}
}
