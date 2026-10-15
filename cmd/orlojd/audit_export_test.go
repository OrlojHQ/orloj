package main

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"
)

type fakeAuditExporter struct {
	since, until time.Time
	lines        string
	err          error
	honorCtx     bool
}

func (f *fakeAuditExporter) Export(ctx context.Context, w io.Writer, since, until time.Time) (int, error) {
	f.since, f.until = since, until
	if f.honorCtx && ctx.Err() != nil {
		return 0, ctx.Err()
	}
	if f.err != nil {
		return 0, f.err
	}
	_, err := io.WriteString(w, f.lines)
	return 1, err
}

func TestRunAuditExport(t *testing.T) {
	since := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	until := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	tests := []struct {
		name      string
		since     string
		until     string
		exportErr error
		wantSince time.Time
		wantUntil time.Time
		wantErr   bool
		wantFile  string
		// preExisting, when non-empty, is written to the target (mode 0644)
		// before the export runs.
		preExisting string
		// cancelled runs the export with an already-cancelled context, as
		// when the operator presses Ctrl-C mid-export.
		cancelled bool
		honorCtx  bool
	}{
		{name: "unbounded", wantFile: "{\"action\":\"a\"}\n"},
		{name: "bounded", since: since.Format(time.RFC3339), until: until.Format(time.RFC3339), wantSince: since, wantUntil: until, wantFile: "{\"action\":\"a\"}\n"},
		{name: "invalid_since", since: "yesterday", wantErr: true},
		{name: "invalid_until", until: "2026-13-01", wantErr: true},
		{name: "since_not_before_until", since: until.Format(time.RFC3339), until: since.Format(time.RFC3339), wantErr: true},
		{name: "export_error", exportErr: errors.New("db down"), wantErr: true},
		{name: "replaces_existing_file_with_0600", preExisting: "old\n", wantFile: "{\"action\":\"a\"}\n"},
		{name: "export_error_leaves_existing_file", preExisting: "old\n", exportErr: errors.New("db down"), wantErr: true},
		{name: "cancelled_exporter_honors_ctx", cancelled: true, honorCtx: true, wantErr: true},
		{name: "cancelled_exporter_ignores_ctx", cancelled: true, wantErr: true},
		{name: "cancelled_leaves_existing_file", preExisting: "old\n", cancelled: true, wantErr: true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "audit.jsonl")
			if tc.preExisting != "" {
				if err := os.WriteFile(path, []byte(tc.preExisting), 0o644); err != nil {
					t.Fatalf("seed existing file: %v", err)
				}
				if err := os.Chmod(path, 0o644); err != nil {
					t.Fatalf("chmod existing file: %v", err)
				}
			}
			exp := &fakeAuditExporter{lines: "{\"action\":\"a\"}\n", err: tc.exportErr, honorCtx: tc.honorCtx}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelled {
				cancel()
			}

			n, err := runAuditExport(ctx, exp, path, tc.since, tc.until)
			entries, readErr := os.ReadDir(dir)
			if readErr != nil {
				t.Fatalf("read dir: %v", readErr)
			}
			wantEntries := 0
			if tc.preExisting != "" || !tc.wantErr {
				wantEntries = 1
			}
			if len(entries) != wantEntries {
				t.Fatalf("dir has %d entries, want %d (temp file left behind?)", len(entries), wantEntries)
			}
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error")
				}
				if tc.preExisting != "" {
					got, err := os.ReadFile(path)
					if err != nil || string(got) != tc.preExisting {
						t.Fatalf("existing file changed: %q err=%v", got, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n != 1 {
				t.Fatalf("count = %d, want 1", n)
			}
			if !exp.since.Equal(tc.wantSince) || !exp.until.Equal(tc.wantUntil) {
				t.Fatalf("range = [%v, %v), want [%v, %v)", exp.since, exp.until, tc.wantSince, tc.wantUntil)
			}
			got, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("read export file: %v", err)
			}
			if string(got) != tc.wantFile {
				t.Fatalf("file = %q, want %q", got, tc.wantFile)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatalf("stat export file: %v", err)
			}
			if mode := info.Mode().Perm(); mode != 0o600 {
				t.Fatalf("file mode = %o, want 600", mode)
			}
		})
	}
}

func TestRunAuditExportRequiresStore(t *testing.T) {
	tests := []struct {
		name string
		exp  auditExporter
	}{
		{name: "nil_exporter", exp: nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := runAuditExport(context.Background(), tc.exp, filepath.Join(t.TempDir(), "x"), "", ""); err == nil {
				t.Fatal("expected error when no audit store is configured")
			}
		})
	}
}

func TestValidateOneShotFlags(t *testing.T) {
	tests := []struct {
		name          string
		resetPassword string
		exportFile    string
		wantErr       bool
	}{
		{name: "neither"},
		{name: "reset_only", resetPassword: "pw"},
		{name: "export_only", exportFile: "audit.jsonl"},
		{name: "both", resetPassword: "pw", exportFile: "audit.jsonl", wantErr: true},
		{name: "whitespace_only_ignored", resetPassword: "  ", exportFile: "audit.jsonl"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := validateOneShotFlags(tc.resetPassword, tc.exportFile)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
		})
	}
}
