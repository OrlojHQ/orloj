package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
)

// AcquireLeasedSlot reserves capacity under a unique token. Expired reservations
// are discarded while holding the worker lock, so an old owner cannot free a
// successor's slot. A missing worker returns an empty token.
func (s *WorkerStore) AcquireLeasedSlot(ctx context.Context, name string, lease time.Duration) (resources.Worker, string, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	name = normalizeLookupName(name)
	token := newUUID()
	if s.db != nil {
		return s.acquireLeasedSlotSQL(ctx, name, token, lease)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	worker, ok := s.items[name]
	if !ok {
		return resources.Worker{}, "", false, nil
	}
	if s.slotLeases == nil {
		s.slotLeases = make(map[string]map[string]time.Time)
	}
	if s.slotLeases[name] == nil {
		s.slotLeases[name] = make(map[string]time.Time)
	}
	now := time.Now()
	for id, expiry := range s.slotLeases[name] {
		if !expiry.After(now) {
			delete(s.slotLeases[name], id)
		}
	}
	worker.Status.CurrentTasks = len(s.slotLeases[name])
	phase := strings.ToLower(strings.TrimSpace(worker.Status.Phase))
	max := worker.Spec.MaxConcurrentTasks
	if max <= 0 {
		max = 1
	}
	acquired := (phase == "ready" || phase == "pending") && worker.Status.CurrentTasks < max
	if acquired {
		s.slotLeases[name][token] = now.Add(lease)
		worker.Status.CurrentTasks++
	} else {
		token = ""
	}
	old := worker.Metadata
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, old, false); err != nil {
		return resources.Worker{}, "", false, err
	}
	s.items[name] = worker
	return worker, token, acquired, nil
}

func (s *WorkerStore) acquireLeasedSlotSQL(ctx context.Context, name, token string, lease time.Duration) (resources.Worker, string, bool, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return resources.Worker{}, "", false, err
	}
	defer tx.Rollback()
	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT payload FROM workers WHERE name=$1 FOR UPDATE`, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return resources.Worker{}, "", false, nil
	}
	if err != nil {
		return resources.Worker{}, "", false, err
	}
	var worker resources.Worker
	if err = json.Unmarshal(payload, &worker); err != nil {
		return resources.Worker{}, "", false, err
	}
	if _, err = tx.ExecContext(ctx, `DELETE FROM worker_slot_leases WHERE worker_name=$1 AND expires_at <= NOW()`, name); err != nil {
		return resources.Worker{}, "", false, err
	}
	if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_slot_leases WHERE worker_name=$1`, name).Scan(&worker.Status.CurrentTasks); err != nil {
		return resources.Worker{}, "", false, err
	}
	phase := strings.ToLower(strings.TrimSpace(worker.Status.Phase))
	max := worker.Spec.MaxConcurrentTasks
	if max <= 0 {
		max = 1
	}
	acquired := (phase == "ready" || phase == "pending") && worker.Status.CurrentTasks < max
	if acquired {
		if _, err = tx.ExecContext(ctx, `INSERT INTO worker_slot_leases(worker_name,token,expires_at) VALUES($1,$2,NOW()+($3 * INTERVAL '1 microsecond'))`, name, token, lease.Microseconds()); err != nil {
			return resources.Worker{}, "", false, err
		}
		worker.Status.CurrentTasks++
	} else {
		token = ""
	}
	old := worker.Metadata
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err = initializeUpdateMetadata("Worker", &worker.Metadata, old, false); err != nil {
		return resources.Worker{}, "", false, err
	}
	if err = updateWorkerInTx(ctx, tx, name, worker); err != nil {
		return resources.Worker{}, "", false, err
	}
	if err = tx.Commit(); err != nil {
		return resources.Worker{}, "", false, err
	}
	return worker, token, acquired, nil
}

// RenewLeasedSlot never revives an expired reservation.
func (s *WorkerStore) RenewLeasedSlot(ctx context.Context, name, token string, lease time.Duration) error {
	if token == "" {
		return nil
	}
	if lease <= 0 {
		lease = 30 * time.Second
	}
	name = normalizeLookupName(name)
	if s.db != nil {
		result, err := s.db.ExecContext(ctx, `UPDATE worker_slot_leases SET expires_at=NOW()+($3 * INTERVAL '1 microsecond') WHERE worker_name=$1 AND token=$2 AND expires_at > NOW()`, name, token, lease.Microseconds())
		if err != nil {
			return err
		}
		n, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if n == 0 {
			return fmt.Errorf("worker %q slot lease lost", name)
		}
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	expiry, ok := s.slotLeases[name][token]
	if !ok || !expiry.After(time.Now()) {
		return fmt.Errorf("worker %q slot lease lost", name)
	}
	s.slotLeases[name][token] = time.Now().Add(lease)
	return nil
}

// ReleaseLeasedSlot affects only the reservation named by token.
func (s *WorkerStore) ReleaseLeasedSlot(ctx context.Context, name, token string) error {
	if token == "" {
		return nil
	}
	name = normalizeLookupName(name)
	if s.db != nil {
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var payload []byte
		err = tx.QueryRowContext(ctx, `SELECT payload FROM workers WHERE name=$1 FOR UPDATE`, name).Scan(&payload)
		if err == sql.ErrNoRows {
			_, err = tx.ExecContext(ctx, `DELETE FROM worker_slot_leases WHERE worker_name=$1 AND token=$2`, name, token)
			if err != nil {
				return err
			}
			return tx.Commit()
		}
		if err != nil {
			return err
		}
		var worker resources.Worker
		if err = json.Unmarshal(payload, &worker); err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `DELETE FROM worker_slot_leases WHERE worker_name=$1 AND (token=$2 OR expires_at <= NOW())`, name, token); err != nil {
			return err
		}
		if err = tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM worker_slot_leases WHERE worker_name=$1`, name).Scan(&worker.Status.CurrentTasks); err != nil {
			return err
		}
		old := worker.Metadata
		worker.Status.ObservedGeneration = worker.Metadata.Generation
		if err = initializeUpdateMetadata("Worker", &worker.Metadata, old, false); err != nil {
			return err
		}
		if err = updateWorkerInTx(ctx, tx, name, worker); err != nil {
			return err
		}
		return tx.Commit()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.slotLeases[name], token)
	worker, ok := s.items[name]
	if !ok {
		return nil
	}
	now := time.Now()
	for id, expiry := range s.slotLeases[name] {
		if !expiry.After(now) {
			delete(s.slotLeases[name], id)
		}
	}
	old := worker.Metadata
	worker.Status.CurrentTasks = len(s.slotLeases[name])
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, old, false); err != nil {
		return err
	}
	s.items[name] = worker
	return nil
}
