package startup

import (
	"context"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/store"
)

const (
	// DefaultAuditBufferSize is how many audit events may wait for the database
	// before new events are dropped.
	DefaultAuditBufferSize = 1024
	// DefaultAuditRetention is how long persisted audit events are kept.
	DefaultAuditRetention = 90 * 24 * time.Hour
	// DefaultAuditPruneInterval is how often expired audit events are deleted.
	DefaultAuditPruneInterval = time.Hour
	// DefaultAuditDrainTimeout bounds how long Close waits to flush queued
	// events, so shutdown does not stall when the database is unreachable.
	DefaultAuditDrainTimeout = 10 * time.Second

	auditInsertTimeout = 5 * time.Second
)

type auditRecordInserter interface {
	Insert(ctx context.Context, rec store.AuditRecord) error
}

type auditRecordPruner interface {
	PruneBefore(ctx context.Context, cutoff time.Time) (int64, error)
}

// PostgresAuditSink is the built-in durable AuditSink. RecordAudit only
// enqueues; a single background goroutine writes events to the audit_events
// table. When the queue is full, or the sink is closed, events are dropped and
// counted rather than blocking the caller, per the AuditSink contract.
type PostgresAuditSink struct {
	inserter      auditRecordInserter
	logger        *log.Logger
	insertTimeout time.Duration
	drainTimeout  time.Duration
	events        chan store.AuditRecord
	stop          chan struct{}
	done          chan struct{}

	// writeCtx bounds every insert; Close cancels it once drainTimeout has
	// passed so shutdown cannot hang on an unreachable database.
	writeCtx    context.Context
	cancelWrite context.CancelFunc

	// mu guards closed. RecordAudit holds the read lock while sending so that
	// no event can enter the queue after Close starts draining it.
	mu     sync.RWMutex
	closed bool

	dropped atomic.Int64
	failed  atomic.Int64
}

// NewPostgresAuditSink starts the background writer. Call Close to flush
// queued events and stop it; Close waits at most DefaultAuditDrainTimeout.
func NewPostgresAuditSink(inserter auditRecordInserter, bufferSize int, logger *log.Logger) *PostgresAuditSink {
	return newPostgresAuditSink(inserter, bufferSize, auditInsertTimeout, DefaultAuditDrainTimeout, logger)
}

func newPostgresAuditSink(inserter auditRecordInserter, bufferSize int, insertTimeout, drainTimeout time.Duration, logger *log.Logger) *PostgresAuditSink {
	if bufferSize <= 0 {
		bufferSize = DefaultAuditBufferSize
	}
	writeCtx, cancelWrite := context.WithCancel(context.Background())
	s := &PostgresAuditSink{
		inserter:      inserter,
		logger:        logger,
		insertTimeout: insertTimeout,
		drainTimeout:  drainTimeout,
		events:        make(chan store.AuditRecord, bufferSize),
		stop:          make(chan struct{}),
		done:          make(chan struct{}),
		writeCtx:      writeCtx,
		cancelWrite:   cancelWrite,
	}
	go s.run()
	return s
}

// RecordAudit implements agentruntime.AuditSink.
func (s *PostgresAuditSink) RecordAudit(_ context.Context, event agentruntime.AuditEvent) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if s.closed {
		s.drop(1)
		return
	}
	select {
	case s.events <- auditRecordFromEvent(event, time.Now().UTC()):
	default:
		s.drop(1)
	}
}

// Dropped returns how many events were discarded: queue full, sent after
// Close, or still queued when the shutdown drain deadline passed.
func (s *PostgresAuditSink) Dropped() int64 {
	return s.dropped.Load()
}

// Close writes queued events and stops the background writer, giving up after
// the drain timeout and counting what is left as dropped. It is safe to call
// more than once.
func (s *PostgresAuditSink) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.stop)
		time.AfterFunc(s.drainTimeout, s.cancelWrite)
	}
	s.mu.Unlock()
	<-s.done
	s.cancelWrite()
}

func (s *PostgresAuditSink) run() {
	defer close(s.done)
	for {
		select {
		case rec := <-s.events:
			s.writeOrDrop(rec)
		case <-s.stop:
			// No new events can be queued once stop is closed (see Close), so
			// draining until the queue is empty is final.
			for {
				select {
				case rec := <-s.events:
					s.writeOrDrop(rec)
				default:
					return
				}
			}
		}
	}
}

// writeOrDrop writes rec unless the drain deadline has already cancelled
// writes, in which case rec is counted as dropped instead of attempted.
func (s *PostgresAuditSink) writeOrDrop(rec store.AuditRecord) {
	if s.writeCtx.Err() != nil {
		s.drop(1)
		return
	}
	s.write(rec)
}

func (s *PostgresAuditSink) write(rec store.AuditRecord) {
	ctx, cancel := context.WithTimeout(s.writeCtx, s.insertTimeout)
	defer cancel()
	if err := s.inserter.Insert(ctx, rec); err != nil {
		if n := s.failed.Add(1); (n == 1 || n%1000 == 0) && s.logger != nil {
			s.logger.Printf("audit sink insert failed (%d failures so far) action=%s: %v", n, rec.Action, err)
		}
	}
}

func (s *PostgresAuditSink) drop(n int64) {
	total := s.dropped.Add(n)
	if s.logger == nil {
		return
	}
	// Log the first drop and then roughly every 1000th.
	if total-n == 0 || total/1000 != (total-n)/1000 {
		s.logger.Printf("audit sink dropped %d audit events so far (buffer full or sink closed)", total)
	}
}

// sanitizeAuditString makes s storable in Postgres TEXT/JSONB, which reject
// NUL bytes and invalid UTF-8.
func sanitizeAuditString(s string) string {
	return strings.ReplaceAll(strings.ToValidUTF8(s, "\uFFFD"), "\x00", "")
}

func auditRecordFromEvent(event agentruntime.AuditEvent, now time.Time) store.AuditRecord {
	occurredAt, err := time.Parse(time.RFC3339Nano, event.Timestamp)
	if err != nil {
		occurredAt = now
	}
	var metadata map[string]string
	if event.Metadata != nil {
		metadata = make(map[string]string, len(event.Metadata))
		for k, v := range event.Metadata {
			metadata[sanitizeAuditString(k)] = sanitizeAuditString(v)
		}
	}
	return store.AuditRecord{
		OccurredAt:   occurredAt.UTC(),
		Component:    sanitizeAuditString(event.Component),
		Action:       sanitizeAuditString(event.Action),
		Outcome:      sanitizeAuditString(event.Outcome),
		Namespace:    sanitizeAuditString(event.Namespace),
		ResourceKind: sanitizeAuditString(event.ResourceKind),
		ResourceName: sanitizeAuditString(event.ResourceName),
		Principal:    sanitizeAuditString(event.Principal),
		Message:      sanitizeAuditString(event.Message),
		Metadata:     metadata,
	}
}

// NewAuditSink returns the built-in durable audit sink when the Postgres
// backend is in use and enabled is true. Otherwise it returns a nil sink, which
// agentruntime.NormalizeExtensions turns into the no-op default. The returned
// function flushes and stops the sink and is always safe to call.
func NewAuditSink(stores *StoreSet, enabled bool, logger *log.Logger) (agentruntime.AuditSink, func()) {
	if !enabled || stores == nil || stores.Audit == nil {
		return nil, func() {}
	}
	sink := NewPostgresAuditSink(stores.Audit, DefaultAuditBufferSize, logger)
	if logger != nil {
		logger.Printf("audit sink: postgres (table audit_events)")
	}
	return sink, sink.Close
}

// RunAuditRetention deletes audit events older than maxAge once at start and
// then every interval until ctx is done. A maxAge of zero or less keeps events
// forever and returns immediately.
func RunAuditRetention(ctx context.Context, pruner auditRecordPruner, maxAge, interval time.Duration, logger *log.Logger) {
	if maxAge <= 0 || pruner == nil {
		return
	}
	if interval <= 0 {
		interval = DefaultAuditPruneInterval
	}
	prune := func() {
		removed, err := pruner.PruneBefore(ctx, time.Now().UTC().Add(-maxAge))
		if logger == nil {
			return
		}
		if err != nil {
			if ctx.Err() == nil {
				logger.Printf("audit retention prune failed: %v", err)
			}
			return
		}
		if removed > 0 {
			logger.Printf("audit retention pruned %d events older than %s", removed, maxAge)
		}
	}

	prune()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			prune()
		}
	}
}
