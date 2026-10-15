package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type auditExporter interface {
	Export(ctx context.Context, w io.Writer, since, until time.Time) (int, error)
}

// runAuditExport writes persisted audit events in [since, until) to path as
// JSON lines. since and until are RFC3339 timestamps; empty means unbounded.
func runAuditExport(ctx context.Context, exporter auditExporter, path, sinceRaw, untilRaw string) (int, error) {
	if exporter == nil {
		return 0, fmt.Errorf("audit export requires --storage-backend=postgres")
	}
	since, err := parseAuditExportTime("audit-export-since", sinceRaw)
	if err != nil {
		return 0, err
	}
	until, err := parseAuditExportTime("audit-export-until", untilRaw)
	if err != nil {
		return 0, err
	}
	if !since.IsZero() && !until.IsZero() && !since.Before(until) {
		return 0, fmt.Errorf("--audit-export-since must be before --audit-export-until")
	}

	// Write to a 0600 temp file in the same directory and rename it into place
	// only on success, so the target always ends up 0600 and a failed export
	// never truncates an existing file.
	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".tmp-*")
	if err != nil {
		return 0, fmt.Errorf("create audit export file: %w", err)
	}
	tmpPath := tmp.Name()
	fail := func(n int, err error) (int, error) {
		_ = tmp.Close()
		_ = os.Remove(tmpPath)
		return n, err
	}
	w := bufio.NewWriter(tmp)
	n, err := exporter.Export(ctx, w, since, until)
	if err != nil {
		return fail(n, fmt.Errorf("export audit events: %w", err))
	}
	if err := w.Flush(); err != nil {
		return fail(n, fmt.Errorf("write audit export file: %w", err))
	}
	if err := tmp.Chmod(0o600); err != nil {
		return fail(n, fmt.Errorf("chmod audit export file: %w", err))
	}
	if err := tmp.Sync(); err != nil {
		return fail(n, fmt.Errorf("sync audit export file: %w", err))
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return n, fmt.Errorf("close audit export file: %w", err)
	}
	// An interrupt during the export cancels ctx; never publish a possibly
	// incomplete file in that case.
	if err := ctx.Err(); err != nil {
		_ = os.Remove(tmpPath)
		return n, fmt.Errorf("audit export interrupted: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return n, fmt.Errorf("rename audit export file: %w", err)
	}
	return n, nil
}

func parseAuditExportTime(flagName, raw string) (time.Time, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return time.Time{}, nil
	}
	t, err := time.Parse(time.RFC3339Nano, raw)
	if err != nil {
		return time.Time{}, fmt.Errorf("--%s must be an RFC3339 timestamp: %w", flagName, err)
	}
	return t.UTC(), nil
}

// validateOneShotFlags rejects combining one-shot modes, which would
// otherwise silently run only the first one.
func validateOneShotFlags(resetAdminPassword, auditExportFile string) error {
	if strings.TrimSpace(resetAdminPassword) != "" && strings.TrimSpace(auditExportFile) != "" {
		return fmt.Errorf("--auth-reset-admin-password and --audit-export-file cannot be used together")
	}
	return nil
}
