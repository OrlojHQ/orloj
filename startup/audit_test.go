package startup

import (
	"bytes"
	"context"
	"errors"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

type fakeAuditInserter struct {
	mu         sync.Mutex
	records    []store.AuditRecord
	failN      int
	failAll    bool
	blockOnCtx bool
	attempts   atomic.Int64
	started    chan struct{}
	release    chan struct{}
}

func (f *fakeAuditInserter) Insert(ctx context.Context, rec store.AuditRecord) error {
	f.attempts.Add(1)
	if f.started != nil {
		select {
		case f.started <- struct{}{}:
		default:
		}
	}
	if f.blockOnCtx {
		<-ctx.Done()
		return ctx.Err()
	}
	if f.failAll {
		return errors.New("insert failed")
	}
	if f.release != nil {
		<-f.release
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.failN > 0 {
		f.failN--
		return errors.New("insert failed")
	}
	f.records = append(f.records, rec)
	return nil
}

func (f *fakeAuditInserter) stored() []store.AuditRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]store.AuditRecord(nil), f.records...)
}

func TestPostgresAuditSinkRecordsEvents(t *testing.T) {
	ts := time.Date(2026, 3, 4, 5, 6, 7, 8, time.UTC)

	tests := []struct {
		name       string
		events     []agentruntime.AuditEvent
		failN      int
		wantStored int
		check      func(t *testing.T, recs []store.AuditRecord, before time.Time)
	}{
		{
			name: "maps_fields_and_timestamp",
			events: []agentruntime.AuditEvent{{
				Timestamp:    ts.Format(time.RFC3339Nano),
				Component:    "api",
				Action:       "token.create",
				Outcome:      "success",
				Namespace:    "default",
				ResourceKind: "APIToken",
				ResourceName: "ci",
				Principal:    "admin",
				Message:      "created",
				Metadata:     map[string]string{"k": "v"},
			}},
			wantStored: 1,
			check: func(t *testing.T, recs []store.AuditRecord, _ time.Time) {
				got := recs[0]
				if !got.OccurredAt.Equal(ts) || got.Component != "api" || got.Action != "token.create" ||
					got.Outcome != "success" || got.Namespace != "default" || got.ResourceKind != "APIToken" ||
					got.ResourceName != "ci" || got.Principal != "admin" || got.Message != "created" ||
					got.Metadata["k"] != "v" {
					t.Fatalf("unexpected record: %+v", got)
				}
			},
		},
		{
			name:       "empty_timestamp_uses_now",
			events:     []agentruntime.AuditEvent{{Action: "a", Outcome: "success"}},
			wantStored: 1,
			check: func(t *testing.T, recs []store.AuditRecord, before time.Time) {
				if recs[0].OccurredAt.Before(before) {
					t.Fatalf("occurred_at %v is before test start %v", recs[0].OccurredAt, before)
				}
			},
		},
		{
			name:       "invalid_timestamp_uses_now",
			events:     []agentruntime.AuditEvent{{Timestamp: "not-a-time", Action: "a", Outcome: "success"}},
			wantStored: 1,
			check: func(t *testing.T, recs []store.AuditRecord, before time.Time) {
				if recs[0].OccurredAt.Before(before) {
					t.Fatalf("occurred_at %v is before test start %v", recs[0].OccurredAt, before)
				}
			},
		},
		{
			name: "insert_error_does_not_stop_sink",
			events: []agentruntime.AuditEvent{
				{Action: "lost", Outcome: "success"},
				{Action: "kept", Outcome: "success"},
			},
			failN:      1,
			wantStored: 1,
			check: func(t *testing.T, recs []store.AuditRecord, _ time.Time) {
				if recs[0].Action != "kept" {
					t.Fatalf("stored action = %q, want kept", recs[0].Action)
				}
			},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			before := time.Now().UTC()
			fake := &fakeAuditInserter{failN: tc.failN}
			sink := NewPostgresAuditSink(fake, 16, nil)
			for _, ev := range tc.events {
				sink.RecordAudit(context.Background(), ev)
			}
			sink.Close()

			recs := fake.stored()
			if len(recs) != tc.wantStored {
				t.Fatalf("stored %d records, want %d", len(recs), tc.wantStored)
			}
			if tc.check != nil {
				tc.check(t, recs, before)
			}
		})
	}
}

func TestPostgresAuditSinkDropsWhenBufferFull(t *testing.T) {
	tests := []struct {
		name        string
		bufferSize  int
		events      int
		wantStored  int
		wantDropped int64
	}{
		{name: "fits_in_buffer", bufferSize: 2, events: 3, wantStored: 3, wantDropped: 0},
		{name: "overflow_dropped", bufferSize: 1, events: 4, wantStored: 2, wantDropped: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuditInserter{started: make(chan struct{}, 1), release: make(chan struct{})}
			sink := NewPostgresAuditSink(fake, tc.bufferSize, nil)

			sink.RecordAudit(context.Background(), agentruntime.AuditEvent{Action: "a", Outcome: "success"})
			select {
			case <-fake.started:
			case <-time.After(2 * time.Second):
				t.Fatal("writer never picked up the first event")
			}
			for i := 1; i < tc.events; i++ {
				sink.RecordAudit(context.Background(), agentruntime.AuditEvent{Action: "a", Outcome: "success"})
			}
			close(fake.release)
			sink.Close()

			if got := len(fake.stored()); got != tc.wantStored {
				t.Fatalf("stored %d, want %d", got, tc.wantStored)
			}
			if got := sink.Dropped(); got != tc.wantDropped {
				t.Fatalf("dropped %d, want %d", got, tc.wantDropped)
			}
		})
	}
}

func TestPostgresAuditSinkCountsEventsAfterCloseAsDropped(t *testing.T) {
	tests := []struct {
		name   string
		events int
	}{
		{name: "single", events: 1},
		{name: "many", events: 5},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuditInserter{}
			sink := NewPostgresAuditSink(fake, 16, nil)
			sink.Close()
			sink.Close()
			for i := 0; i < tc.events; i++ {
				sink.RecordAudit(context.Background(), agentruntime.AuditEvent{Action: "late", Outcome: "success"})
			}
			if got := len(fake.stored()); got != 0 {
				t.Fatalf("stored %d after close, want 0", got)
			}
			if got := sink.Dropped(); got != int64(tc.events) {
				t.Fatalf("dropped %d after close, want %d", got, tc.events)
			}
		})
	}
}

func TestPostgresAuditSinkConcurrentCloseAccountsForEveryEvent(t *testing.T) {
	tests := []struct {
		name       string
		goroutines int
		perG       int
	}{
		{name: "few", goroutines: 2, perG: 50},
		{name: "many", goroutines: 16, perG: 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fake := &fakeAuditInserter{}
			sink := NewPostgresAuditSink(fake, 64, nil)
			var wg sync.WaitGroup
			for g := 0; g < tc.goroutines; g++ {
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < tc.perG; i++ {
						sink.RecordAudit(context.Background(), agentruntime.AuditEvent{Action: "a", Outcome: "success"})
					}
				}()
			}
			time.Sleep(time.Millisecond)
			sink.Close()
			wg.Wait()

			total := int64(tc.goroutines * tc.perG)
			if got := int64(len(fake.stored())) + sink.Dropped(); got != total {
				t.Fatalf("stored+dropped = %d, want %d", got, total)
			}
		})
	}
}

func TestPostgresAuditSinkCloseIsBounded(t *testing.T) {
	tests := []struct {
		name         string
		inserter     *fakeAuditInserter
		events       int
		drainTimeout time.Duration
		wantStored   int
		wantAttempts int64
		wantDropped  int64
		wantFailLogs int
	}{
		{name: "healthy_store_drains_all", inserter: &fakeAuditInserter{}, events: 5, drainTimeout: time.Second, wantStored: 5, wantAttempts: 5},
		// The first event is in flight (blocked) when Close is called; once the
		// drain deadline cancels it, the other four must be dropped, not tried.
		{name: "blocked_store_stops_at_deadline", inserter: &fakeAuditInserter{blockOnCtx: true, started: make(chan struct{}, 1)}, events: 5, drainTimeout: 50 * time.Millisecond, wantAttempts: 1, wantDropped: 4, wantFailLogs: 1},
		{name: "failing_store_logs_once", inserter: &fakeAuditInserter{failAll: true}, events: 5, drainTimeout: time.Second, wantAttempts: 5, wantFailLogs: 1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var logs bytes.Buffer
			var logMu sync.Mutex
			logger := log.New(&lockedWriter{mu: &logMu, w: &logs}, "", 0)
			sink := newPostgresAuditSink(tc.inserter, 16, time.Hour, tc.drainTimeout, logger)
			for i := 0; i < tc.events; i++ {
				sink.RecordAudit(context.Background(), agentruntime.AuditEvent{Action: "a", Outcome: "success"})
				if i == 0 && tc.inserter.started != nil {
					select {
					case <-tc.inserter.started:
					case <-time.After(2 * time.Second):
						t.Fatal("writer never picked up the first event")
					}
				}
			}

			start := time.Now()
			sink.Close()
			if elapsed := time.Since(start); elapsed > tc.drainTimeout+time.Second {
				t.Fatalf("Close took %v, want about %v", elapsed, tc.drainTimeout)
			}

			if got := len(tc.inserter.stored()); got != tc.wantStored {
				t.Fatalf("stored %d, want %d", got, tc.wantStored)
			}
			if got := tc.inserter.attempts.Load(); got != tc.wantAttempts {
				t.Fatalf("attempts %d, want %d", got, tc.wantAttempts)
			}
			if got := sink.Dropped(); got != tc.wantDropped {
				t.Fatalf("dropped %d, want %d", got, tc.wantDropped)
			}
			logMu.Lock()
			failLogs := strings.Count(logs.String(), "audit sink insert failed")
			logMu.Unlock()
			if failLogs != tc.wantFailLogs {
				t.Fatalf("insert-failed log lines = %d, want %d\n%s", failLogs, tc.wantFailLogs, logs.String())
			}
		})
	}
}

type lockedWriter struct {
	mu *sync.Mutex
	w  *bytes.Buffer
}

func (l *lockedWriter) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.w.Write(p)
}

func TestAuditRecordFromEventSanitizes(t *testing.T) {
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	tests := []struct {
		name         string
		event        agentruntime.AuditEvent
		wantAction   string
		wantMessage  string
		wantMetadata map[string]string
	}{
		{
			name:        "clean_values_unchanged",
			event:       agentruntime.AuditEvent{Action: "a", Message: "héllo", Metadata: map[string]string{"k": "v"}},
			wantAction:  "a",
			wantMessage: "héllo", wantMetadata: map[string]string{"k": "v"},
		},
		{
			name:        "nul_bytes_stripped",
			event:       agentruntime.AuditEvent{Action: "a\x00b", Message: "\x00x\x00", Metadata: map[string]string{"k\x00": "v\x00w"}},
			wantAction:  "ab",
			wantMessage: "x", wantMetadata: map[string]string{"k": "vw"},
		},
		{
			name:        "invalid_utf8_replaced",
			event:       agentruntime.AuditEvent{Action: "a\xffb", Message: "m\xc3", Metadata: map[string]string{"k": "\xfe"}},
			wantAction:  "a\uFFFDb",
			wantMessage: "m\uFFFD", wantMetadata: map[string]string{"k": "\uFFFD"},
		},
		{
			name:       "nil_metadata_stays_nil",
			event:      agentruntime.AuditEvent{Action: "a"},
			wantAction: "a",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rec := auditRecordFromEvent(tc.event, now)
			if rec.Action != tc.wantAction || rec.Message != tc.wantMessage {
				t.Fatalf("action=%q message=%q, want %q %q", rec.Action, rec.Message, tc.wantAction, tc.wantMessage)
			}
			if len(rec.Metadata) != len(tc.wantMetadata) {
				t.Fatalf("metadata = %q, want %q", rec.Metadata, tc.wantMetadata)
			}
			for k, v := range tc.wantMetadata {
				if rec.Metadata[k] != v {
					t.Fatalf("metadata = %q, want %q", rec.Metadata, tc.wantMetadata)
				}
			}
		})
	}
}

type fakeAuditPruner struct {
	mu      sync.Mutex
	cutoffs []time.Time
	err     error
}

func (f *fakeAuditPruner) PruneBefore(_ context.Context, cutoff time.Time) (int64, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.cutoffs = append(f.cutoffs, cutoff)
	return 1, f.err
}

func (f *fakeAuditPruner) calls() []time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]time.Time(nil), f.cutoffs...)
}

func TestRunAuditRetention(t *testing.T) {
	tests := []struct {
		name      string
		maxAge    time.Duration
		pruneErr  error
		wantCalls int // minimum number of prune calls
	}{
		{name: "disabled_when_zero", maxAge: 0, wantCalls: 0},
		{name: "prunes_on_start_and_interval", maxAge: time.Hour, wantCalls: 2},
		{name: "keeps_running_after_error", maxAge: time.Hour, pruneErr: errors.New("db down"), wantCalls: 2},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			pruner := &fakeAuditPruner{err: tc.pruneErr}
			ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
			defer cancel()

			start := time.Now()
			RunAuditRetention(ctx, pruner, tc.maxAge, 10*time.Millisecond, nil)

			calls := pruner.calls()
			if tc.wantCalls == 0 {
				if len(calls) != 0 {
					t.Fatalf("prune called %d times, want 0", len(calls))
				}
				return
			}
			if len(calls) < tc.wantCalls {
				t.Fatalf("prune called %d times, want at least %d", len(calls), tc.wantCalls)
			}
			wantCutoff := start.Add(-tc.maxAge)
			if d := calls[0].Sub(wantCutoff); d < 0 || d > time.Second {
				t.Fatalf("first cutoff %v not within 1s after %v", calls[0], wantCutoff)
			}
		})
	}
}

func TestNewAuditSink(t *testing.T) {
	tests := []struct {
		name     string
		stores   *StoreSet
		enabled  bool
		wantSink bool
	}{
		{name: "memory_backend", stores: &StoreSet{}, enabled: true, wantSink: false},
		{name: "postgres_disabled", stores: &StoreSet{Audit: store.NewAuditStoreWithDB(nil)}, enabled: false, wantSink: false},
		{name: "postgres_enabled", stores: &StoreSet{Audit: store.NewAuditStoreWithDB(nil)}, enabled: true, wantSink: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sink, closeSink := NewAuditSink(tc.stores, tc.enabled, nil)
			defer closeSink()
			if (sink != nil) != tc.wantSink {
				t.Fatalf("sink = %v, wantSink %v", sink, tc.wantSink)
			}
		})
	}
}
