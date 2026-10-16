package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"time"
)

// AuditRecord is one persisted audit event. Its JSON shape matches
// agentruntime.AuditEvent so exported lines can be consumed by the same tools.
type AuditRecord struct {
	OccurredAt   time.Time         `json:"timestamp"`
	Component    string            `json:"component,omitempty"`
	Action       string            `json:"action"`
	Outcome      string            `json:"outcome"`
	Namespace    string            `json:"namespace,omitempty"`
	ResourceKind string            `json:"resource_kind,omitempty"`
	ResourceName string            `json:"resource_name,omitempty"`
	Principal    string            `json:"principal,omitempty"`
	Message      string            `json:"message,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// AuditStore persists audit events in the audit_events table. It is Postgres
// only; the memory backend has no durable audit trail.
type AuditStore struct {
	db *sql.DB
}

func NewAuditStoreWithDB(db *sql.DB) *AuditStore {
	return &AuditStore{db: db}
}

// Insert appends one audit record.
func (s *AuditStore) Insert(ctx context.Context, rec AuditRecord) error {
	metadata := rec.Metadata
	if metadata == nil {
		metadata = map[string]string{}
	}
	payload, err := json.Marshal(metadata)
	if err != nil {
		return err
	}
	_, err = s.db.ExecContext(ctx,
		`INSERT INTO audit_events(occurred_at, component, action, outcome, namespace, resource_kind, resource_name, principal, message, metadata)
		 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10::jsonb)`,
		rec.OccurredAt.UTC(),
		rec.Component,
		rec.Action,
		rec.Outcome,
		rec.Namespace,
		rec.ResourceKind,
		rec.ResourceName,
		rec.Principal,
		rec.Message,
		string(payload),
	)
	return err
}

// PruneBefore deletes records that occurred before cutoff and returns how many
// were removed.
func (s *AuditStore) PruneBefore(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := s.db.ExecContext(ctx, `DELETE FROM audit_events WHERE occurred_at < $1`, cutoff.UTC())
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Export writes records with since <= occurred_at < until to w as JSON lines,
// oldest first. A zero since or until leaves that side of the range open.
func (s *AuditStore) Export(ctx context.Context, w io.Writer, since, until time.Time) (int, error) {
	var sinceArg, untilArg any
	if !since.IsZero() {
		sinceArg = since.UTC()
	}
	if !until.IsZero() {
		untilArg = until.UTC()
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT occurred_at, component, action, outcome, namespace, resource_kind, resource_name, principal, message, metadata
		 FROM audit_events
		 WHERE ($1::timestamptz IS NULL OR occurred_at >= $1::timestamptz)
		   AND ($2::timestamptz IS NULL OR occurred_at < $2::timestamptz)
		 ORDER BY occurred_at, id`,
		sinceArg,
		untilArg,
	)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	enc := json.NewEncoder(w)
	count := 0
	for rows.Next() {
		var rec AuditRecord
		var metadata []byte
		if err := rows.Scan(
			&rec.OccurredAt, &rec.Component, &rec.Action, &rec.Outcome, &rec.Namespace,
			&rec.ResourceKind, &rec.ResourceName, &rec.Principal, &rec.Message, &metadata,
		); err != nil {
			return count, err
		}
		if len(metadata) > 0 {
			if err := json.Unmarshal(metadata, &rec.Metadata); err != nil {
				return count, fmt.Errorf("decode audit metadata: %w", err)
			}
		}
		if len(rec.Metadata) == 0 {
			rec.Metadata = nil
		}
		rec.OccurredAt = rec.OccurredAt.UTC()
		if err := enc.Encode(rec); err != nil {
			return count, err
		}
		count++
	}
	return count, rows.Err()
}
