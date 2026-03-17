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

const (
	tableAgents          = "agents"
	tableAgentSystems    = "agent_systems"
	tableModelEndpoints  = "model_endpoints"
	tableTools           = "tools"
	tableSecrets         = "secrets"
	tableMemories        = "memories"
	tableAgentPolicies   = "agent_policies"
	tableAgentRoles      = "agent_roles"
	tableToolPermissions = "tool_permissions"
	tableTasks           = "tasks"
	tableTaskSchedules   = "task_schedules"
	tableTaskWebhooks    = "task_webhooks"
	tableWorkers         = "workers"
	tableToolApprovals   = "tool_approvals"
)

// EnsurePostgresSchema runs all pending database migrations. New schema changes
// should be added as numbered SQL files in store/migrations/ (e.g.,
// 002_add_foo.up.sql). Migrations are tracked in a schema_migrations table and
// applied exactly once, in lexicographic order.
func EnsurePostgresSchema(db *sql.DB) error {
	return Migrate(db)
}

// ---------------------------------------------------------------------------
// Generic helpers -- table names are compile-time constants, no injection risk.
// ---------------------------------------------------------------------------

func getFromTable[T any](db *sql.DB, table, name string) (T, bool, error) {
	var zero T
	var payload []byte
	err := db.QueryRow(fmt.Sprintf(`SELECT payload FROM %s WHERE name = $1`, table), name).Scan(&payload)
	if err == sql.ErrNoRows {
		return zero, false, nil
	}
	if err != nil {
		return zero, false, err
	}
	var item T
	if err := json.Unmarshal(payload, &item); err != nil {
		return zero, false, err
	}
	return item, true, nil
}

func listFromTable[T any](db *sql.DB, table string) ([]T, error) {
	rows, err := db.Query(fmt.Sprintf(`SELECT payload FROM %s ORDER BY name ASC`, table))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]T, 0)
	for rows.Next() {
		var payload []byte
		if err := rows.Scan(&payload); err != nil {
			return nil, err
		}
		var item T
		if err := json.Unmarshal(payload, &item); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func deleteFromTable(db *sql.DB, table, name string) (bool, error) {
	result, err := db.Exec(fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, table), name)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

// ---------------------------------------------------------------------------
// Per-type upsert functions -- extract typed columns for indexing/filtering.
// ---------------------------------------------------------------------------

func upsertAgentSQL(db *sql.DB, name string, item resources.Agent) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO agents(name, namespace, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertAgentSystemSQL(db *sql.DB, name string, item resources.AgentSystem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO agent_systems(name, namespace, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertModelEndpointSQL(db *sql.DB, name string, item resources.ModelEndpoint) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO model_endpoints(name, namespace, provider, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     provider = EXCLUDED.provider,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Spec.Provider)),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertToolSQL(db *sql.DB, name string, item resources.Tool) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO tools(name, namespace, risk_level, isolation_mode, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     risk_level = EXCLUDED.risk_level,
		     isolation_mode = EXCLUDED.isolation_mode,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Spec.RiskLevel)),
		strings.ToLower(strings.TrimSpace(item.Spec.Runtime.IsolationMode)),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertSecretSQL(db *sql.DB, name string, item resources.Secret) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO secrets(name, namespace, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertMemorySQL(db *sql.DB, name string, item resources.Memory) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO memories(name, namespace, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertAgentPolicySQL(db *sql.DB, name string, item resources.AgentPolicy) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO agent_policies(name, namespace, apply_mode, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     apply_mode = EXCLUDED.apply_mode,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Spec.ApplyMode)),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertAgentRoleSQL(db *sql.DB, name string, item resources.AgentRole) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO agent_roles(name, namespace, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertToolPermissionSQL(db *sql.DB, name string, item resources.ToolPermission) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO tool_permissions(name, namespace, tool_ref, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     tool_ref = EXCLUDED.tool_ref,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.ToolRef),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertToolApprovalSQL(db *sql.DB, name string, item resources.ToolApproval) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO tool_approvals(name, namespace, task_ref, tool, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     task_ref = EXCLUDED.task_ref,
		     tool = EXCLUDED.tool,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.TaskRef),
		strings.TrimSpace(item.Spec.Tool),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertTaskSQL(db *sql.DB, name string, item resources.Task) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	leaseUntil := parseTimestampPtr(item.Status.LeaseUntil)
	nextAttemptAt := parseTimestampPtr(item.Status.NextAttemptAt)
	_, err = db.Exec(
		`INSERT INTO tasks(name, namespace, system_ref, mode, status_phase, assigned_worker,
		     claimed_by, lease_until, next_attempt_at, priority, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     system_ref = EXCLUDED.system_ref,
		     mode = EXCLUDED.mode,
		     status_phase = EXCLUDED.status_phase,
		     assigned_worker = EXCLUDED.assigned_worker,
		     claimed_by = EXCLUDED.claimed_by,
		     lease_until = EXCLUDED.lease_until,
		     next_attempt_at = EXCLUDED.next_attempt_at,
		     priority = EXCLUDED.priority,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.System),
		strings.ToLower(strings.TrimSpace(item.Spec.Mode)),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		strings.TrimSpace(item.Status.AssignedWorker),
		strings.TrimSpace(item.Status.ClaimedBy),
		leaseUntil,
		nextAttemptAt,
		strings.ToLower(strings.TrimSpace(item.Spec.Priority)),
		string(payload),
	)
	return err
}

func upsertTaskScheduleSQL(db *sql.DB, name string, item resources.TaskSchedule) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO task_schedules(name, namespace, task_ref, schedule, suspend, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6, $7::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     task_ref = EXCLUDED.task_ref,
		     schedule = EXCLUDED.schedule,
		     suspend = EXCLUDED.suspend,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.TaskRef),
		strings.TrimSpace(item.Spec.Schedule),
		item.Spec.Suspend,
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertTaskWebhookSQL(db *sql.DB, name string, item resources.TaskWebhook) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO task_webhooks(name, namespace, task_ref, status_phase, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     task_ref = EXCLUDED.task_ref,
		     status_phase = EXCLUDED.status_phase,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.TaskRef),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		string(payload),
	)
	return err
}

func upsertWorkerSQL(db *sql.DB, name string, item resources.Worker) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO workers(name, namespace, region, status_phase, current_tasks, max_concurrent_tasks, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6, $7::jsonb, NOW())
		 ON CONFLICT(name) DO UPDATE SET
		     namespace = EXCLUDED.namespace,
		     region = EXCLUDED.region,
		     status_phase = EXCLUDED.status_phase,
		     current_tasks = EXCLUDED.current_tasks,
		     max_concurrent_tasks = EXCLUDED.max_concurrent_tasks,
		     payload = EXCLUDED.payload,
		     updated_at = NOW()`,
		name,
		resources.NormalizeNamespace(item.Metadata.Namespace),
		strings.TrimSpace(item.Spec.Region),
		strings.ToLower(strings.TrimSpace(item.Status.Phase)),
		item.Status.CurrentTasks,
		item.Spec.MaxConcurrentTasks,
		string(payload),
	)
	return err
}

// ---------------------------------------------------------------------------
// Task claiming and lease management
// ---------------------------------------------------------------------------

// updateTaskInTx writes both typed columns and payload within an open tx.
func updateTaskInTx(tx *sql.Tx, name string, task resources.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`UPDATE tasks SET
		     status_phase = $2,
		     assigned_worker = $3,
		     claimed_by = $4,
		     lease_until = $5,
		     next_attempt_at = $6,
		     payload = $7::jsonb,
		     updated_at = NOW()
		 WHERE name = $1`,
		name,
		strings.ToLower(strings.TrimSpace(task.Status.Phase)),
		strings.TrimSpace(task.Status.AssignedWorker),
		strings.TrimSpace(task.Status.ClaimedBy),
		parseTimestampPtr(task.Status.LeaseUntil),
		parseTimestampPtr(task.Status.NextAttemptAt),
		string(payload),
	)
	return err
}

func claimTaskSQL(db *sql.DB, name, workerID string, lease time.Duration) (resources.Task, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return resources.Task{}, false, err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM tasks WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return resources.Task{}, false, nil
	}
	if err != nil {
		return resources.Task{}, false, err
	}

	var task resources.Task
	if err := json.Unmarshal(payload, &task); err != nil {
		return resources.Task{}, false, err
	}
	if !isTaskClaimable(task, workerID, now) {
		if err := tx.Commit(); err != nil {
			return resources.Task{}, false, err
		}
		return resources.Task{}, false, nil
	}

	task, err = applyTaskClaim(task, workerID, lease, now)
	if err != nil {
		return resources.Task{}, false, err
	}
	if err := updateTaskInTx(tx, name, task); err != nil {
		return resources.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.Task{}, false, err
	}
	return task, true, nil
}

func claimNextDueTaskSQL(db *sql.DB, workerID string, lease time.Duration, matches func(resources.Task) bool) (resources.Task, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return resources.Task{}, false, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(
		`SELECT name, payload
		 FROM tasks
		 WHERE mode != 'template'
		   AND (
		     (status_phase IN ('', 'pending')
		       AND (next_attempt_at IS NULL OR next_attempt_at <= NOW())
		     )
		     OR (status_phase = 'running'
		       AND (claimed_by = '' OR lease_until IS NULL OR lease_until <= NOW())
		     )
		   )
		 ORDER BY updated_at ASC
		 FOR UPDATE SKIP LOCKED
		 LIMIT 64`,
	)
	if err != nil {
		return resources.Task{}, false, err
	}
	defer rows.Close()

	var (
		selectedName string
		selectedTask resources.Task
		found        bool
	)
	for rows.Next() {
		var (
			rName   string
			payload []byte
		)
		if err := rows.Scan(&rName, &payload); err != nil {
			return resources.Task{}, false, err
		}
		var task resources.Task
		if err := json.Unmarshal(payload, &task); err != nil {
			return resources.Task{}, false, err
		}
		if !isTaskClaimable(task, workerID, now) {
			continue
		}
		if matches != nil && !matches(task) {
			continue
		}
		selectedName = rName
		selectedTask = task
		found = true
		break
	}
	if err := rows.Err(); err != nil {
		return resources.Task{}, false, err
	}
	if !found {
		if err := tx.Commit(); err != nil {
			return resources.Task{}, false, err
		}
		return resources.Task{}, false, nil
	}

	task, err := applyTaskClaim(selectedTask, workerID, lease, now)
	if err != nil {
		return resources.Task{}, false, err
	}
	if err := updateTaskInTx(tx, selectedName, task); err != nil {
		return resources.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.Task{}, false, err
	}
	return task, true, nil
}

func renewTaskLeaseSQL(db *sql.DB, name, workerID string, lease time.Duration) error {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM tasks WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return fmt.Errorf("task %q not found", name)
	}
	if err != nil {
		return err
	}

	var task resources.Task
	if err := json.Unmarshal(payload, &task); err != nil {
		return err
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.ClaimedBy), strings.TrimSpace(workerID)) {
		return fmt.Errorf("task %q is claimed by %q, not %q", name, task.Status.ClaimedBy, workerID)
	}
	if !strings.EqualFold(strings.TrimSpace(task.Status.Phase), "running") {
		return fmt.Errorf("task %q is not running", name)
	}

	currentMeta := task.Metadata
	task.Status.LeaseUntil = now.Add(lease).Format(time.RFC3339Nano)
	task.Status.LastHeartbeat = now.Format(time.RFC3339Nano)
	task.Status.ObservedGeneration = task.Metadata.Generation
	if err := initializeUpdateMetadata("Task", &task.Metadata, currentMeta, false); err != nil {
		return err
	}

	if err := updateTaskInTx(tx, name, task); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Task logs
// ---------------------------------------------------------------------------

func appendTaskLogSQL(db *sql.DB, taskName, entry string) error {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM tasks WHERE name = $1`, taskName).Scan(&exists)
	if err == sql.ErrNoRows {
		return fmt.Errorf("task %q not found", taskName)
	}
	if err != nil {
		return err
	}
	_, err = db.Exec(`INSERT INTO task_logs(task_name, entry, created_at) VALUES($1, $2, NOW())`, taskName, entry)
	return err
}

func listTaskLogsSQL(db *sql.DB, taskName string) ([]string, error) {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM tasks WHERE name = $1`, taskName).Scan(&exists)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("task %q not found", taskName)
	}
	if err != nil {
		return nil, err
	}

	rows, err := db.Query(`SELECT entry FROM task_logs WHERE task_name = $1 ORDER BY created_at ASC, id ASC`, taskName)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]string, 0)
	for rows.Next() {
		var entry string
		if err := rows.Scan(&entry); err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func deleteTaskLogsSQL(db *sql.DB, taskName string) error {
	_, err := db.Exec(`DELETE FROM task_logs WHERE task_name = $1`, taskName)
	return err
}

// ---------------------------------------------------------------------------
// Worker slot management
// ---------------------------------------------------------------------------

func updateWorkerInTx(tx *sql.Tx, name string, worker resources.Worker) error {
	payload, err := json.Marshal(worker)
	if err != nil {
		return err
	}
	_, err = tx.Exec(
		`UPDATE workers SET
		     status_phase = $2,
		     current_tasks = $3,
		     max_concurrent_tasks = $4,
		     payload = $5::jsonb,
		     updated_at = NOW()
		 WHERE name = $1`,
		name,
		strings.ToLower(strings.TrimSpace(worker.Status.Phase)),
		worker.Status.CurrentTasks,
		worker.Spec.MaxConcurrentTasks,
		string(payload),
	)
	return err
}

func tryAcquireWorkerSlotSQL(db *sql.DB, name string) (resources.Worker, bool, error) {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return resources.Worker{}, false, err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM workers WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return resources.Worker{}, false, nil
	}
	if err != nil {
		return resources.Worker{}, false, err
	}

	var worker resources.Worker
	if err := json.Unmarshal(payload, &worker); err != nil {
		return resources.Worker{}, false, err
	}
	phase := strings.ToLower(strings.TrimSpace(worker.Status.Phase))
	if phase != "ready" && phase != "pending" {
		if err := tx.Commit(); err != nil {
			return resources.Worker{}, false, err
		}
		return worker, false, nil
	}

	maxConcurrent := worker.Spec.MaxConcurrentTasks
	if maxConcurrent <= 0 {
		maxConcurrent = 1
	}
	if worker.Status.CurrentTasks >= maxConcurrent {
		if err := tx.Commit(); err != nil {
			return resources.Worker{}, false, err
		}
		return worker, false, nil
	}

	current := worker.Metadata
	worker.Status.CurrentTasks++
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, current, false); err != nil {
		return resources.Worker{}, false, err
	}

	if err := updateWorkerInTx(tx, name, worker); err != nil {
		return resources.Worker{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.Worker{}, false, err
	}
	return worker, true, nil
}

func releaseWorkerSlotSQL(db *sql.DB, name string) error {
	tx, err := db.BeginTx(context.Background(), nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRow(`SELECT payload FROM workers WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil
	}
	if err != nil {
		return err
	}

	var worker resources.Worker
	if err := json.Unmarshal(payload, &worker); err != nil {
		return err
	}
	if worker.Status.CurrentTasks <= 0 {
		return tx.Commit()
	}

	current := worker.Metadata
	worker.Status.CurrentTasks--
	if worker.Status.CurrentTasks < 0 {
		worker.Status.CurrentTasks = 0
	}
	worker.Status.ObservedGeneration = worker.Metadata.Generation
	if err := initializeUpdateMetadata("Worker", &worker.Metadata, current, false); err != nil {
		return err
	}

	if err := updateWorkerInTx(tx, name, worker); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Webhook deduplication
// ---------------------------------------------------------------------------

func upsertWebhookDedupeSQL(db *sql.DB, endpointID, eventID, taskName string, expiresAt time.Time) error {
	_, err := db.Exec(
		`INSERT INTO webhook_dedupe(endpoint_id, event_id, task_name, expires_at, created_at)
		 VALUES($1, $2, $3, $4, NOW())
		 ON CONFLICT(endpoint_id, event_id)
		 DO UPDATE SET task_name = EXCLUDED.task_name, expires_at = EXCLUDED.expires_at`,
		endpointID,
		eventID,
		taskName,
		expiresAt.UTC(),
	)
	return err
}

func getWebhookDedupeSQL(db *sql.DB, endpointID, eventID string, now time.Time) (string, bool, error) {
	var taskName string
	err := db.QueryRow(
		`SELECT task_name
		 FROM webhook_dedupe
		 WHERE endpoint_id = $1 AND event_id = $2 AND expires_at > $3`,
		endpointID,
		eventID,
		now.UTC(),
	).Scan(&taskName)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return taskName, true, nil
}

func pruneWebhookDedupeSQL(db *sql.DB, now time.Time) error {
	_, err := db.Exec(`DELETE FROM webhook_dedupe WHERE expires_at <= $1`, now.UTC())
	return err
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func parseTimestampPtr(value string) *time.Time {
	v := strings.TrimSpace(value)
	if v == "" {
		return nil
	}
	t, err := parseTimestamp(v)
	if err != nil {
		return nil
	}
	return &t
}
