package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
)

func postA2AV1JSONRPC(t *testing.T, url, method string, params any, version string) *http.Response {
	t.Helper()
	body, err := json.Marshal(map[string]any{
		"jsonrpc": "2.0",
		"id":      "v1-test",
		"method":  method,
		"params":  params,
	})
	if err != nil {
		t.Fatalf("marshal JSON-RPC request: %v", err)
	}
	req, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new A2A v1 request: %v", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if version != "" {
		req.Header.Set("A2A-Version", version)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("A2A v1 request failed: %v", err)
	}
	return resp
}

func TestA2AV1SendGetListAndCancel(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()
	seedAgent(t, stores, "v1-system", "A v1 agent", nil)
	endpoint := ts.URL + "/v1/agent-systems/v1-system/a2a"

	send := postA2AV1JSONRPC(t, endpoint, "SendMessage", map[string]any{
		"message": map[string]any{
			"messageId": "message-1",
			"role":      "ROLE_USER",
			"parts":     []map[string]any{{"text": "do the work"}},
		},
		"configuration": map[string]any{"returnImmediately": true},
	}, "1.0")
	sendRPC := decodeJSONRPCResponse(t, send)
	if sendRPC.Error != nil {
		t.Fatalf("SendMessage error = %#v", sendRPC.Error)
	}
	var sendResult struct {
		Task struct {
			ID        string `json:"id"`
			ContextID string `json:"contextId"`
			Status    struct {
				State string `json:"state"`
			} `json:"status"`
		} `json:"task"`
	}
	if err := json.Unmarshal(sendRPC.Result, &sendResult); err != nil {
		t.Fatalf("decode SendMessage result: %v", err)
	}
	if sendResult.Task.ID == "" || sendResult.Task.ContextID == "" {
		t.Fatalf("missing generated v1 identity: %#v", sendResult.Task)
	}
	if sendResult.Task.Status.State != "TASK_STATE_SUBMITTED" {
		t.Fatalf("state = %q, want TASK_STATE_SUBMITTED", sendResult.Task.Status.State)
	}

	get := postA2AV1JSONRPC(t, endpoint, "GetTask", map[string]any{"id": sendResult.Task.ID}, "1.0")
	getRPC := decodeJSONRPCResponse(t, get)
	if getRPC.Error != nil {
		t.Fatalf("GetTask error = %#v", getRPC.Error)
	}
	var gotTask struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(getRPC.Result, &gotTask); err != nil {
		t.Fatalf("decode GetTask result: %v", err)
	}
	if gotTask.ID != sendResult.Task.ID {
		t.Fatalf("GetTask ID = %q, want %q", gotTask.ID, sendResult.Task.ID)
	}

	list := postA2AV1JSONRPC(t, endpoint, "ListTasks", map[string]any{"pageSize": 10}, "1.0")
	listRPC := decodeJSONRPCResponse(t, list)
	if listRPC.Error != nil {
		t.Fatalf("ListTasks error = %#v", listRPC.Error)
	}
	var listResult struct {
		Tasks     []json.RawMessage `json:"tasks"`
		TotalSize int               `json:"totalSize"`
	}
	if err := json.Unmarshal(listRPC.Result, &listResult); err != nil {
		t.Fatalf("decode ListTasks result: %v", err)
	}
	if listResult.TotalSize != 1 || len(listResult.Tasks) != 1 {
		t.Fatalf("ListTasks result = %#v", listResult)
	}

	cancel := postA2AV1JSONRPC(t, endpoint, "CancelTask", map[string]any{"id": sendResult.Task.ID}, "1.0")
	cancelRPC := decodeJSONRPCResponse(t, cancel)
	if cancelRPC.Error != nil {
		t.Fatalf("CancelTask error = %#v", cancelRPC.Error)
	}
	var canceled struct {
		Status struct {
			State string `json:"state"`
		} `json:"status"`
	}
	if err := json.Unmarshal(cancelRPC.Result, &canceled); err != nil {
		t.Fatalf("decode CancelTask result: %v", err)
	}
	if canceled.Status.State != "TASK_STATE_CANCELED" {
		t.Fatalf("cancel state = %q, want TASK_STATE_CANCELED", canceled.Status.State)
	}
}

func TestA2AV1RejectsUnsupportedVersion(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()
	seedAgent(t, stores, "v1-system", "A v1 agent", nil)

	resp := postA2AV1JSONRPC(t, ts.URL+"/v1/agent-systems/v1-system/a2a", "GetTask", map[string]any{"id": "missing"}, "2.0")
	rpc := decodeJSONRPCResponse(t, resp)
	if rpc.Error == nil || rpc.Error.Code != -32009 {
		t.Fatalf("error = %#v, want version-not-supported -32009", rpc.Error)
	}
}

func TestA2AV1PushConfigCRUD(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()
	seedAgent(t, stores, "v1-system", "A v1 agent", nil)
	endpoint := ts.URL + "/v1/agent-systems/v1-system/a2a"

	send := postA2AV1JSONRPC(t, endpoint, "SendMessage", map[string]any{
		"message": map[string]any{
			"messageId": "push-message",
			"role":      "ROLE_USER",
			"parts":     []map[string]any{{"text": "notify me"}},
		},
		"configuration": map[string]any{"returnImmediately": true},
	}, "1.0")
	sendRPC := decodeJSONRPCResponse(t, send)
	var sendResult struct {
		Task struct {
			ID string `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal(sendRPC.Result, &sendResult); err != nil {
		t.Fatalf("decode SendMessage result: %v", err)
	}

	create := postA2AV1JSONRPC(t, endpoint, "CreateTaskPushNotificationConfig", map[string]any{
		"taskId": sendResult.Task.ID,
		"url":    "https://callbacks.example.com/a2a",
		"token":  "notification-token",
	}, "1.0")
	createRPC := decodeJSONRPCResponse(t, create)
	if createRPC.Error != nil {
		t.Fatalf("CreateTaskPushNotificationConfig error = %#v", createRPC.Error)
	}
	var config struct {
		ID     string `json:"id"`
		TaskID string `json:"taskId"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal(createRPC.Result, &config); err != nil {
		t.Fatalf("decode create result: %v", err)
	}
	if config.ID == "" || config.TaskID != sendResult.Task.ID {
		t.Fatalf("created config = %#v", config)
	}

	get := postA2AV1JSONRPC(t, endpoint, "GetTaskPushNotificationConfig", map[string]any{
		"taskId": sendResult.Task.ID,
		"id":     config.ID,
	}, "1.0")
	getRPC := decodeJSONRPCResponse(t, get)
	if getRPC.Error != nil {
		t.Fatalf("GetTaskPushNotificationConfig error = %#v", getRPC.Error)
	}

	list := postA2AV1JSONRPC(t, endpoint, "ListTaskPushNotificationConfigs", map[string]any{
		"taskId":   sendResult.Task.ID,
		"pageSize": 10,
	}, "1.0")
	listRPC := decodeJSONRPCResponse(t, list)
	if listRPC.Error != nil {
		t.Fatalf("ListTaskPushNotificationConfigs error = %#v", listRPC.Error)
	}
	var listed struct {
		Configs []json.RawMessage `json:"configs"`
	}
	if err := json.Unmarshal(listRPC.Result, &listed); err != nil || len(listed.Configs) != 1 {
		t.Fatalf("list result = %s, error = %v", listRPC.Result, err)
	}

	deleteResp := postA2AV1JSONRPC(t, endpoint, "DeleteTaskPushNotificationConfig", map[string]any{
		"taskId": sendResult.Task.ID,
		"id":     config.ID,
	}, "1.0")
	deleteRPC := decodeJSONRPCResponse(t, deleteResp)
	if deleteRPC.Error != nil {
		t.Fatalf("DeleteTaskPushNotificationConfig error = %#v", deleteRPC.Error)
	}
}

func TestA2AV1PascalCaseWorksWithoutVersionHeaderForMethodCompatibility(t *testing.T) {
	ts, stores := newA2ATestServer(t, true)
	defer ts.Close()
	seedAgent(t, stores, "v1-system", "A v1 agent", nil)

	resp := postA2AV1JSONRPC(t, ts.URL+"/v1/agent-systems/v1-system/a2a", "ListTasks", map[string]any{}, "")
	rpc := decodeJSONRPCResponse(t, resp)
	if rpc.Error != nil {
		t.Fatalf("ListTasks without version header error = %#v", rpc.Error)
	}
}
