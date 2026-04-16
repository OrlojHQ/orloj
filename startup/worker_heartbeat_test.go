package startup

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

func TestHeartbeatWorkerRegistrationMarksWorkerNotReadyOnShutdown(t *testing.T) {
	db, state := openHeartbeatReproDB(t)
	defer db.Close()

	workerStore := store.NewWorkerStoreWithDB(db)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})

	go func() {
		defer close(done)
		HeartbeatWorkerRegistration(ctx, workerStore, nil, "worker-a", resources.WorkerSpec{
			Region:             "default",
			MaxConcurrentTasks: 1,
		}, 5*time.Millisecond)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		worker, ok, err := workerStore.Get(context.Background(), "worker-a")
		if err == nil && ok && strings.EqualFold(worker.Status.Phase, "ready") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for ready heartbeat; ok=%v err=%v state=%s", ok, err, state.phase())
		}
		time.Sleep(10 * time.Millisecond)
	}

	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for heartbeat shutdown")
	}

	worker, ok, err := workerStore.Get(context.Background(), "worker-a")
	if err != nil {
		t.Fatalf("get after shutdown failed: %v", err)
	}
	if !ok {
		t.Fatal("expected worker to remain stored after shutdown")
	}
	if !strings.EqualFold(worker.Status.Phase, "notready") {
		t.Fatalf("expected shutdown to mark worker not ready, got %q", worker.Status.Phase)
	}
}

type heartbeatReproState struct {
	mu           sync.Mutex
	payload      []byte
	canceledCall int
}

func (s *heartbeatReproState) phase() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.payload) == 0 {
		return ""
	}
	var worker resources.Worker
	if err := json.Unmarshal(s.payload, &worker); err != nil {
		return ""
	}
	return worker.Status.Phase
}

func (s *heartbeatReproState) recordCanceled() {
	s.mu.Lock()
	s.canceledCall++
	s.mu.Unlock()
}

func (s *heartbeatReproState) loadPayload() []byte {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]byte(nil), s.payload...)
}

func (s *heartbeatReproState) storePayload(payload []byte) {
	s.mu.Lock()
	s.payload = append([]byte(nil), payload...)
	s.mu.Unlock()
}

type heartbeatReproDriver struct {
	state *heartbeatReproState
}

type heartbeatReproConn struct {
	state *heartbeatReproState
}

type heartbeatReproTx struct{}

type heartbeatReproRows struct {
	columns []string
	rows    [][]driver.Value
	index   int
}

type heartbeatReproResult int64

func openHeartbeatReproDB(t *testing.T) (*sql.DB, *heartbeatReproState) {
	t.Helper()
	state := &heartbeatReproState{}
	driverName := "heartbeat-repro-" + strings.ReplaceAll(t.Name(), "/", "-")
	sql.Register(driverName, &heartbeatReproDriver{state: state})
	db, err := sql.Open(driverName, "")
	if err != nil {
		t.Fatalf("open repro db failed: %v", err)
	}
	return db, state
}

func (d *heartbeatReproDriver) Open(string) (driver.Conn, error) {
	return &heartbeatReproConn{state: d.state}, nil
}

func (c *heartbeatReproConn) Prepare(string) (driver.Stmt, error) {
	return nil, errors.New("prepare not supported")
}

func (c *heartbeatReproConn) Close() error { return nil }

func (c *heartbeatReproConn) Begin() (driver.Tx, error) { return heartbeatReproTx{}, nil }

func (c *heartbeatReproConn) BeginTx(ctx context.Context, _ driver.TxOptions) (driver.Tx, error) {
	if err := ctx.Err(); err != nil {
		c.state.recordCanceled()
		return nil, err
	}
	return heartbeatReproTx{}, nil
}

func (c *heartbeatReproConn) ExecContext(ctx context.Context, query string, args []driver.NamedValue) (driver.Result, error) {
	if err := ctx.Err(); err != nil {
		c.state.recordCanceled()
		return nil, err
	}
	if strings.Contains(strings.ToLower(query), "insert into workers") && len(args) >= 7 {
		if payload, ok := args[6].Value.(string); ok {
			c.state.storePayload([]byte(payload))
		}
	}
	return heartbeatReproResult(1), nil
}

func (c *heartbeatReproConn) QueryContext(ctx context.Context, _ string, _ []driver.NamedValue) (driver.Rows, error) {
	if err := ctx.Err(); err != nil {
		c.state.recordCanceled()
		return nil, err
	}
	payload := c.state.loadPayload()
	if len(payload) == 0 {
		return &heartbeatReproRows{columns: []string{"payload"}}, nil
	}
	return &heartbeatReproRows{
		columns: []string{"payload"},
		rows:    [][]driver.Value{{string(payload)}},
	}, nil
}

func (heartbeatReproTx) Commit() error   { return nil }
func (heartbeatReproTx) Rollback() error { return nil }

func (heartbeatReproResult) LastInsertId() (int64, error) { return 0, nil }
func (heartbeatReproResult) RowsAffected() (int64, error) { return 1, nil }

func (r *heartbeatReproRows) Columns() []string { return r.columns }
func (r *heartbeatReproRows) Close() error      { return nil }

func (r *heartbeatReproRows) Next(dest []driver.Value) error {
	if r.index >= len(r.rows) {
		return io.EOF
	}
	copy(dest, r.rows[r.index])
	r.index++
	return nil
}
