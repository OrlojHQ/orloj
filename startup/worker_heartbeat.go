package startup

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/store"
)

// HeartbeatWorkerRegistration runs a background loop that upserts a Worker
// resource on every tick, maintaining a "Ready" heartbeat while the process is
// alive. On context cancellation it sets the worker to "NotReady" and returns.
// This function is intentionally blocking — callers should run it as a goroutine.
func HeartbeatWorkerRegistration(
	ctx context.Context,
	workerStore *store.WorkerStore,
	logger *log.Logger,
	workerID string,
	spec resources.WorkerSpec,
	interval time.Duration,
) {
	if interval <= 0 {
		interval = 10 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		now := time.Now().UTC().Format(time.RFC3339Nano)
		worker := resources.Worker{
			APIVersion: "orloj.dev/v1",
			Kind:       "Worker",
			Metadata:   resources.ObjectMeta{Name: workerID},
			Spec:       spec,
			Status: resources.WorkerStatus{
				Phase:         "Ready",
				LastHeartbeat: now,
				CurrentTasks:  0,
			},
		}
		for attempt := 0; attempt < 3; attempt++ {
		if existing, ok, _ := workerStore.Get(workerID); ok {
			worker.Metadata.ResourceVersion = existing.Metadata.ResourceVersion
			worker.Metadata.Generation = existing.Metadata.Generation
			worker.Metadata.CreatedAt = existing.Metadata.CreatedAt
			worker.Status.ObservedGeneration = existing.Metadata.Generation
			if strings.EqualFold(strings.TrimSpace(existing.Status.Phase), "ready") ||
				strings.EqualFold(strings.TrimSpace(existing.Status.Phase), "pending") {
				worker.Status.CurrentTasks = existing.Status.CurrentTasks
			} else {
				worker.Status.CurrentTasks = 0
			}
		}
			_, err := workerStore.Upsert(worker)
			if err == nil {
				break
			}
			if !store.IsConflict(err) {
				if logger != nil {
					logger.Printf("worker heartbeat upsert failed: %v", err)
				}
				break
			}
		}

		select {
		case <-ctx.Done():
			final := worker
			final.Status.Phase = "NotReady"
			final.Status.LastError = "worker stopped"
		if existing, ok, _ := workerStore.Get(workerID); ok {
			final.Metadata.ResourceVersion = existing.Metadata.ResourceVersion
			final.Metadata.Generation = existing.Metadata.Generation
			final.Metadata.CreatedAt = existing.Metadata.CreatedAt
		}
			_, _ = workerStore.Upsert(final)
			return
		case <-ticker.C:
		}
	}
}
