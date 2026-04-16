package store

import (
	"context"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

func TestTaskStoreClaimAndRenewLease(t *testing.T) {
	s := NewTaskStore()
	bg := context.Background()
	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "t1"},
		Spec: resources.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "x"},
			Retry:  resources.TaskRetryPolicy{MaxAttempts: 3, Backoff: "10ms"},
		},
	}
	if _, err := s.Upsert(bg, task); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	claimed, ok, err := s.ClaimIfDue(bg, "t1", "worker-a", 50*time.Millisecond)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claim success")
	}
	if claimed.Status.Phase != "Running" {
		t.Fatalf("expected Running, got %q", claimed.Status.Phase)
	}
	if claimed.Status.ClaimedBy != "worker-a" {
		t.Fatalf("expected claimedBy=worker-a, got %q", claimed.Status.ClaimedBy)
	}

	if err := s.RenewLease(bg, "t1", "worker-a", 50*time.Millisecond); err != nil {
		t.Fatalf("renew lease failed: %v", err)
	}

	if err := s.RenewLease(bg, "t1", "worker-b", 50*time.Millisecond); err == nil {
		t.Fatal("expected renew lease to fail for non-owner worker")
	}
}

func TestTaskStoreClaimFailoverOnLeaseExpiry(t *testing.T) {
	s := NewTaskStore()
	bg := context.Background()
	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata:   resources.ObjectMeta{Name: "t2"},
		Spec: resources.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "x"},
		},
	}
	if _, err := s.Upsert(bg, task); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	if _, ok, err := s.ClaimIfDue(bg, "t2", "worker-a", 20*time.Millisecond); err != nil {
		t.Fatalf("first claim failed: %v", err)
	} else if !ok {
		t.Fatal("expected first claim success")
	}

	if _, ok, err := s.ClaimIfDue(bg, "t2", "worker-b", 20*time.Millisecond); err != nil {
		t.Fatalf("second claim before expiry failed: %v", err)
	} else if ok {
		t.Fatal("expected second claim to fail before lease expiry")
	}

	time.Sleep(30 * time.Millisecond)
	claimed, ok, err := s.ClaimIfDue(bg, "t2", "worker-b", 20*time.Millisecond)
	if err != nil {
		t.Fatalf("claim after expiry failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claim success after lease expiry")
	}
	if claimed.Status.ClaimedBy != "worker-b" {
		t.Fatalf("expected claimedBy=worker-b, got %q", claimed.Status.ClaimedBy)
	}
}

func TestTaskStoreIsolationAcrossBoundaries(t *testing.T) {
	s := NewTaskStore()
	bg := context.Background()
	retryable := true
	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:        "isolated-task",
			Labels:      map[string]string{"env": "test"},
			Annotations: map[string]string{"owner": "qa"},
		},
		Spec: resources.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "orig"},
			MessageRetry: resources.TaskMessageRetryPolicy{
				NonRetryable: []string{"denied"},
			},
		},
		Status: resources.TaskStatus{
			Output: map[string]string{"result": "orig"},
			Trace: []resources.TaskTraceEvent{{
				Type:      "tool_call",
				Message:   "trace-orig",
				Retryable: &retryable,
			}},
			History: []resources.TaskHistoryEvent{{
				Type:    "claim",
				Message: "history-orig",
			}},
			Messages: []resources.TaskMessage{{
				MessageID: "msg-1",
				Phase:     "Queued",
				Content:   "message-orig",
			}},
			MessageIdempotency: []resources.TaskMessageIdempotency{{
				Key:   "msg-1",
				State: "completed",
			}},
			JoinStates: []resources.TaskJoinState{{
				Node: "reviewer",
				Sources: []resources.TaskJoinSource{{
					MessageID: "msg-join",
					Payload:   "join-orig",
				}},
			}},
			DelegationStates: []resources.TaskDelegationState{{
				Node: "delegate",
				Sources: []resources.TaskJoinSource{{
					MessageID: "msg-delegate",
					Payload:   "delegate-orig",
				}},
			}},
		},
	}

	stored, err := s.Upsert(bg, task)
	if err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	task.Metadata.Labels["env"] = "mutated-input"
	task.Metadata.Annotations["owner"] = "mutated-input"
	task.Spec.Input["topic"] = "mutated-input"
	task.Spec.MessageRetry.NonRetryable[0] = "mutated-input"
	task.Status.Output["result"] = "mutated-input"
	task.Status.Trace[0].Message = "mutated-input"
	*task.Status.Trace[0].Retryable = false
	task.Status.History[0].Message = "mutated-input"
	task.Status.Messages[0].Content = "mutated-input"
	task.Status.MessageIdempotency[0].State = "mutated-input"
	task.Status.JoinStates[0].Sources[0].Payload = "mutated-input"
	task.Status.DelegationStates[0].Sources[0].Payload = "mutated-input"

	stored.Metadata.Labels["env"] = "mutated-return"
	stored.Metadata.Annotations["owner"] = "mutated-return"
	stored.Spec.Input["topic"] = "mutated-return"
	stored.Spec.MessageRetry.NonRetryable[0] = "mutated-return"
	stored.Status.Output["result"] = "mutated-return"
	stored.Status.Trace[0].Message = "mutated-return"
	*stored.Status.Trace[0].Retryable = false
	stored.Status.History[0].Message = "mutated-return"
	stored.Status.Messages[0].Content = "mutated-return"
	stored.Status.MessageIdempotency[0].State = "mutated-return"
	stored.Status.JoinStates[0].Sources[0].Payload = "mutated-return"
	stored.Status.DelegationStates[0].Sources[0].Payload = "mutated-return"

	got, ok, err := s.Get(bg, "isolated-task")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok {
		t.Fatal("expected task to exist")
	}
	assertTaskIsolation(t, got)

	list, err := s.List(bg)
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if len(list) != 1 {
		t.Fatalf("expected 1 listed task, got %d", len(list))
	}
	list[0].Spec.Input["topic"] = "mutated-list"
	list[0].Status.Output["result"] = "mutated-list"
	list[0].Status.JoinStates[0].Sources[0].Payload = "mutated-list"

	cursor, err := s.ListCursor(bg, 10, "", "")
	if err != nil {
		t.Fatalf("list cursor failed: %v", err)
	}
	if len(cursor) != 1 {
		t.Fatalf("expected 1 cursor task, got %d", len(cursor))
	}
	cursor[0].Metadata.Labels["env"] = "mutated-cursor"
	cursor[0].Status.Messages[0].Content = "mutated-cursor"

	afterList, ok, err := s.Get(bg, "isolated-task")
	if err != nil {
		t.Fatalf("get after list mutation failed: %v", err)
	}
	if !ok {
		t.Fatal("expected task to exist after list mutation")
	}
	assertTaskIsolation(t, afterList)
}

func TestTaskStoreClaimReturnsIsolatedCopy(t *testing.T) {
	s := NewTaskStore()
	bg := context.Background()
	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:   "claim-copy-task",
			Labels: map[string]string{"team": "core"},
		},
		Spec: resources.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "orig"},
		},
		Status: resources.TaskStatus{
			Output: map[string]string{"result": "orig"},
		},
	}
	if _, err := s.Upsert(bg, task); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	claimed, ok, err := s.ClaimIfDue(bg, "claim-copy-task", "worker-a", time.Minute)
	if err != nil {
		t.Fatalf("claim failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claim success")
	}

	claimed.Metadata.Labels["team"] = "mutated"
	claimed.Spec.Input["topic"] = "mutated"
	claimed.Status.Output = map[string]string{"result": "mutated"}

	stored, ok, err := s.Get(bg, "claim-copy-task")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claimed task to exist")
	}
	if stored.Metadata.Labels["team"] != "core" {
		t.Fatalf("expected stored labels to remain unchanged, got %q", stored.Metadata.Labels["team"])
	}
	if stored.Spec.Input["topic"] != "orig" {
		t.Fatalf("expected stored input to remain unchanged, got %q", stored.Spec.Input["topic"])
	}
	if len(stored.Status.Output) != 0 {
		t.Fatalf("expected stored output to remain cleared after claim, got %+v", stored.Status.Output)
	}
	if stored.Status.ClaimedBy != "worker-a" {
		t.Fatalf("expected stored claim owner worker-a, got %q", stored.Status.ClaimedBy)
	}
}

func TestTaskStoreClaimNextDueReturnsIsolatedCopy(t *testing.T) {
	s := NewTaskStore()
	bg := context.Background()
	task := resources.Task{
		APIVersion: "orloj.dev/v1",
		Kind:       "Task",
		Metadata: resources.ObjectMeta{
			Name:   "claim-next-copy-task",
			Labels: map[string]string{"team": "core"},
		},
		Spec: resources.TaskSpec{
			System: "sys",
			Input:  map[string]string{"topic": "orig"},
		},
		Status: resources.TaskStatus{
			Output: map[string]string{"result": "orig"},
		},
	}
	if _, err := s.Upsert(bg, task); err != nil {
		t.Fatalf("upsert failed: %v", err)
	}

	claimed, ok, err := s.ClaimNextDue(bg, "worker-a", time.Minute, WorkerClaimHints{}, nil)
	if err != nil {
		t.Fatalf("claim next due failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claim next due success")
	}

	claimed.Metadata.Labels["team"] = "mutated"
	claimed.Spec.Input["topic"] = "mutated"
	claimed.Status.Output = map[string]string{"result": "mutated"}

	stored, ok, err := s.Get(bg, "claim-next-copy-task")
	if err != nil {
		t.Fatalf("get failed: %v", err)
	}
	if !ok {
		t.Fatal("expected claimed task to exist")
	}
	if stored.Metadata.Labels["team"] != "core" {
		t.Fatalf("expected stored labels to remain unchanged, got %q", stored.Metadata.Labels["team"])
	}
	if stored.Spec.Input["topic"] != "orig" {
		t.Fatalf("expected stored input to remain unchanged, got %q", stored.Spec.Input["topic"])
	}
	if len(stored.Status.Output) != 0 {
		t.Fatalf("expected stored output to remain cleared after claim, got %+v", stored.Status.Output)
	}
	if stored.Status.ClaimedBy != "worker-a" {
		t.Fatalf("expected stored claim owner worker-a, got %q", stored.Status.ClaimedBy)
	}
}

func assertTaskIsolation(t *testing.T, task resources.Task) {
	t.Helper()
	if task.Metadata.Labels["env"] != "test" {
		t.Fatalf("expected env label test, got %q", task.Metadata.Labels["env"])
	}
	if task.Metadata.Annotations["owner"] != "qa" {
		t.Fatalf("expected owner annotation qa, got %q", task.Metadata.Annotations["owner"])
	}
	if task.Spec.Input["topic"] != "orig" {
		t.Fatalf("expected task input orig, got %q", task.Spec.Input["topic"])
	}
	if len(task.Spec.MessageRetry.NonRetryable) != 1 || task.Spec.MessageRetry.NonRetryable[0] != "denied" {
		t.Fatalf("expected non_retryable to remain unchanged, got %+v", task.Spec.MessageRetry.NonRetryable)
	}
	if task.Status.Output["result"] != "orig" {
		t.Fatalf("expected output result orig, got %q", task.Status.Output["result"])
	}
	if len(task.Status.Trace) != 1 || task.Status.Trace[0].Message != "trace-orig" {
		t.Fatalf("expected trace to remain unchanged, got %+v", task.Status.Trace)
	}
	if task.Status.Trace[0].Retryable == nil || !*task.Status.Trace[0].Retryable {
		t.Fatalf("expected retryable pointer to remain true, got %+v", task.Status.Trace[0].Retryable)
	}
	if len(task.Status.History) != 1 || task.Status.History[0].Message != "history-orig" {
		t.Fatalf("expected history to remain unchanged, got %+v", task.Status.History)
	}
	if len(task.Status.Messages) != 1 || task.Status.Messages[0].Content != "message-orig" {
		t.Fatalf("expected messages to remain unchanged, got %+v", task.Status.Messages)
	}
	if len(task.Status.MessageIdempotency) != 1 || task.Status.MessageIdempotency[0].State != "completed" {
		t.Fatalf("expected idempotency to remain unchanged, got %+v", task.Status.MessageIdempotency)
	}
	if len(task.Status.JoinStates) != 1 || len(task.Status.JoinStates[0].Sources) != 1 || task.Status.JoinStates[0].Sources[0].Payload != "join-orig" {
		t.Fatalf("expected join state to remain unchanged, got %+v", task.Status.JoinStates)
	}
	if len(task.Status.DelegationStates) != 1 || len(task.Status.DelegationStates[0].Sources) != 1 || task.Status.DelegationStates[0].Sources[0].Payload != "delegate-orig" {
		t.Fatalf("expected delegation state to remain unchanged, got %+v", task.Status.DelegationStates)
	}
}
