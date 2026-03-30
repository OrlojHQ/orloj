package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"testing"

	"github.com/OrlojHQ/orloj/api"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
)

func TestNativeAuthUserCRUDAndRoleEnforcement(t *testing.T) {
	server := newNativeAuthServer(t)
	defer server.Close()

	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	setupResp, err := adminClient.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	setupResp.Body.Close()

	createUserBody := []byte(`{"username":"reader-user","role":"reader"}`)
	resp, err := adminClient.Post(server.URL+"/v1/auth/users", "application/json", bytes.NewReader(createUserBody))
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201 create user, got %d body=%s", resp.StatusCode, string(body))
	}
	var created struct {
		Username string `json:"username"`
		Role     string `json:"role"`
		Password string `json:"password"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		resp.Body.Close()
		t.Fatalf("decode create user response failed: %v", err)
	}
	resp.Body.Close()
	if created.Username != "reader-user" || created.Role != "reader" {
		t.Fatalf("unexpected create-user response: %+v", created)
	}
	if created.Password == "" {
		t.Fatal("expected generated password in create-user response")
	}

	resp, err = adminClient.Get(server.URL + "/v1/auth/users")
	if err != nil {
		t.Fatalf("list users request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 list users, got %d body=%s", resp.StatusCode, string(body))
	}
	var listed struct {
		Items []struct {
			Username string `json:"username"`
			Role     string `json:"role"`
		} `json:"items"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&listed); err != nil {
		resp.Body.Close()
		t.Fatalf("decode list users response failed: %v", err)
	}
	resp.Body.Close()
	if len(listed.Items) != 2 {
		t.Fatalf("expected 2 users (admin + reader-user), got %d", len(listed.Items))
	}

	readerJar, _ := cookiejar.New(nil)
	readerClient := &http.Client{Jar: readerJar}
	loginPayload := map[string]string{"username": "reader-user", "password": created.Password}
	loginBody, _ := json.Marshal(loginPayload)
	resp, err = readerClient.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("reader login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 reader login, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = readerClient.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("reader get tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 reader get tasks, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	toolBody := []byte(`{"apiVersion":"orloj.dev/v1","kind":"Tool","metadata":{"name":"rtool"},"spec":{"type":"http","endpoint":"https://example"}}`)
	req, err := http.NewRequest(http.MethodPost, server.URL+"/v1/tools", bytes.NewReader(toolBody))
	if err != nil {
		t.Fatalf("build tool create request failed: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err = readerClient.Do(req)
	if err != nil {
		t.Fatalf("reader create tool request failed: %v", err)
	}
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 403 for reader mutating request, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestNativeAuthDeleteLastAdminBlocked(t *testing.T) {
	server := newNativeAuthServer(t)
	defer server.Close()

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}
	setupResp, err := client.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	setupResp.Body.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/v1/auth/users/admin", nil)
	if err != nil {
		t.Fatalf("build delete request failed: %v", err)
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("delete last admin request failed: %v", err)
	}
	if resp.StatusCode != http.StatusConflict {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 409 for last-admin delete, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestNativeAuthResetPasswordTargetsUserAndInvalidatesSessions(t *testing.T) {
	server := newNativeAuthServer(t)
	defer server.Close()

	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}
	setupResp, err := adminClient.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	setupResp.Body.Close()

	resp, err := adminClient.Post(server.URL+"/v1/auth/users", "application/json", bytes.NewReader([]byte(`{"username":"writer-user","role":"writer"}`)))
	if err != nil {
		t.Fatalf("create writer user request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201 create writer user, got %d body=%s", resp.StatusCode, string(body))
	}
	var created struct {
		Password string `json:"password"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		resp.Body.Close()
		t.Fatalf("decode create writer user response failed: %v", err)
	}
	resp.Body.Close()

	writerJar, _ := cookiejar.New(nil)
	writerClient := &http.Client{Jar: writerJar}
	loginBody, _ := json.Marshal(map[string]string{"username": "writer-user", "password": created.Password})
	resp, err = writerClient.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		t.Fatalf("writer login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 writer login, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = writerClient.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("writer get tasks failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 writer tasks before reset, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resetPayload := []byte(`{"username":"writer-user","new_password":"writer-new-strong-pass"}`)
	resp, err = adminClient.Post(server.URL+"/v1/auth/admin/reset-password", "application/json", bytes.NewReader(resetPayload))
	if err != nil {
		t.Fatalf("admin reset-password request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 reset password, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = adminClient.Get(server.URL + "/v1/auth/users")
	if err != nil {
		t.Fatalf("admin list-users request after reset failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected admin session to remain valid after resetting another user, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	resp, err = writerClient.Get(server.URL + "/v1/tasks")
	if err != nil {
		t.Fatalf("writer tasks request after reset failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 401 after session invalidation, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	oldLoginBody, _ := json.Marshal(map[string]string{"username": "writer-user", "password": created.Password})
	resp, err = writerClient.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader(oldLoginBody))
	if err != nil {
		t.Fatalf("writer old password login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 401 old password after reset, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	newLoginBody, _ := json.Marshal(map[string]string{"username": "writer-user", "password": "writer-new-strong-pass"})
	resp, err = writerClient.Post(server.URL+"/v1/auth/login", "application/json", bytes.NewReader(newLoginBody))
	if err != nil {
		t.Fatalf("writer new password login request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 new password login after reset, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()
}

func TestNativeAuthUserCRUDAuditPrincipal(t *testing.T) {
	audit := &captureAuditSink{}
	server := newNativeAuthServerWithOptions(t, api.ServerOptions{
		AuthMode: api.AuthModeNative,
		Extensions: agentruntime.Extensions{
			Audit: audit,
		},
	}, true)
	defer server.Close()

	adminJar, _ := cookiejar.New(nil)
	adminClient := &http.Client{Jar: adminJar}

	setupResp, err := adminClient.Post(server.URL+"/v1/auth/setup", "application/json", bytes.NewReader([]byte(`{"username":"admin","password":"very-strong-pass"}`)))
	if err != nil {
		t.Fatalf("setup request failed: %v", err)
	}
	setupResp.Body.Close()

	resp, err := adminClient.Post(server.URL+"/v1/auth/users", "application/json", bytes.NewReader([]byte(`{"username":"ops-user","role":"reader"}`)))
	if err != nil {
		t.Fatalf("create user request failed: %v", err)
	}
	if resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 201 create user, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	req, err := http.NewRequest(http.MethodDelete, server.URL+"/v1/auth/users/ops-user", nil)
	if err != nil {
		t.Fatalf("build delete user request failed: %v", err)
	}
	resp, err = adminClient.Do(req)
	if err != nil {
		t.Fatalf("delete user request failed: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		t.Fatalf("expected 200 delete user, got %d body=%s", resp.StatusCode, string(body))
	}
	resp.Body.Close()

	events := audit.snapshot()
	var sawCreate bool
	var sawDelete bool
	for _, event := range events {
		if event.Action == "user.create" && event.Principal == "admin" {
			sawCreate = true
		}
		if event.Action == "user.delete" && event.Principal == "admin" {
			sawDelete = true
		}
	}
	if !sawCreate {
		t.Fatalf("expected user.create audit principal=admin, events=%+v", events)
	}
	if !sawDelete {
		t.Fatalf("expected user.delete audit principal=admin, events=%+v", events)
	}
}
