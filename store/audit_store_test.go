package store

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

func openPostgresForAuditTest(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ORLOJ_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("ORLOJ_POSTGRES_DSN is not set; skipping Postgres integration test")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatalf("open postgres failed: %v", err)
	}
	if err := db.Ping(); err != nil {
		_ = db.Close()
		t.Skipf("postgres not reachable at ORLOJ_POSTGRES_DSN: %v", err)
	}
	if err := EnsurePostgresSchema(db); err != nil {
		_ = db.Close()
		t.Fatalf("ensure schema failed: %v", err)
	}
	reset := func() {
		if _, err := db.Exec(`TRUNCATE TABLE audit_events`); err != nil {
			t.Fatalf("reset audit_events failed: %v", err)
		}
	}
	reset()
	t.Cleanup(func() {
		reset()
		_ = db.Close()
	})
	return db
}

func seedAuditRecords(t *testing.T, s *AuditStore, base time.Time) {
	t.Helper()
	for i, action := range []string{"first", "second", "third"} {
		err := s.Insert(context.Background(), AuditRecord{
			OccurredAt:   base.Add(time.Duration(i) * time.Hour),
			Component:    "api",
			Action:       action,
			Outcome:      "success",
			Namespace:    "default",
			ResourceKind: "Task",
			ResourceName: "t-" + action,
			Principal:    "admin",
			Metadata:     map[string]string{"index": action},
		})
		if err != nil {
			t.Fatalf("insert %s: %v", action, err)
		}
	}
}

func TestPostgresAuditStoreExportRange(t *testing.T) {
	db := openPostgresForAuditTest(t)

	base := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)
	s := NewAuditStoreWithDB(db)
	seedAuditRecords(t, s, base)

	tests := []struct {
		name        string
		since       time.Time
		until       time.Time
		wantActions []string
	}{
		{name: "unbounded", wantActions: []string{"first", "second", "third"}},
		{name: "since_inclusive", since: base.Add(time.Hour), wantActions: []string{"second", "third"}},
		{name: "until_exclusive", until: base.Add(time.Hour), wantActions: []string{"first"}},
		{name: "bounded", since: base, until: base.Add(2 * time.Hour), wantActions: []string{"first", "second"}},
		{name: "empty", since: base.Add(24 * time.Hour), wantActions: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			n, err := s.Export(context.Background(), &buf, tc.since, tc.until)
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			if n != len(tc.wantActions) {
				t.Fatalf("export count = %d, want %d", n, len(tc.wantActions))
			}
			var got []string
			scanner := bufio.NewScanner(&buf)
			for scanner.Scan() {
				var rec AuditRecord
				if err := json.Unmarshal(scanner.Bytes(), &rec); err != nil {
					t.Fatalf("decode line %q: %v", scanner.Text(), err)
				}
				if rec.ResourceName != "t-"+rec.Action || rec.Metadata["index"] != rec.Action {
					t.Fatalf("unexpected record fields: %+v", rec)
				}
				got = append(got, rec.Action)
			}
			if len(got) != len(tc.wantActions) {
				t.Fatalf("actions = %v, want %v", got, tc.wantActions)
			}
			for i := range got {
				if got[i] != tc.wantActions[i] {
					t.Fatalf("actions = %v, want %v", got, tc.wantActions)
				}
			}
		})
	}
}

func TestPostgresAuditStorePruneBefore(t *testing.T) {
	base := time.Date(2026, 1, 2, 3, 0, 0, 0, time.UTC)

	tests := []struct {
		name        string
		cutoff      time.Time
		wantRemoved int64
		wantLeft    int
	}{
		{name: "nothing_older", cutoff: base, wantRemoved: 0, wantLeft: 3},
		{name: "some_older", cutoff: base.Add(90 * time.Minute), wantRemoved: 2, wantLeft: 1},
		{name: "all_older", cutoff: base.Add(24 * time.Hour), wantRemoved: 3, wantLeft: 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			db := openPostgresForAuditTest(t)
			s := NewAuditStoreWithDB(db)
			seedAuditRecords(t, s, base)

			removed, err := s.PruneBefore(context.Background(), tc.cutoff)
			if err != nil {
				t.Fatalf("prune: %v", err)
			}
			if removed != tc.wantRemoved {
				t.Fatalf("removed = %d, want %d", removed, tc.wantRemoved)
			}
			n, err := s.Export(context.Background(), &bytes.Buffer{}, time.Time{}, time.Time{})
			if err != nil {
				t.Fatalf("export: %v", err)
			}
			if n != tc.wantLeft {
				t.Fatalf("remaining = %d, want %d", n, tc.wantLeft)
			}
		})
	}
}
