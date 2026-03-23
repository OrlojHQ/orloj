package store

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
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
	tableMcpServers      = "mcp_servers"
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

// dbExecer abstracts *sql.DB and *sql.Tx so that helpers like getFromTable and
// the per-type upsert functions can run inside or outside a transaction.
type dbExecer interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

func getFromTable[T any](ctx context.Context, db dbExecer, table, name string) (T, bool, error) {
	var zero T
	var payload []byte
	err := db.QueryRowContext(ctx, fmt.Sprintf(`SELECT payload FROM %s WHERE name = $1`, table), name).Scan(&payload)
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

// getFromTableForUpdate is like getFromTable but acquires a row-level lock
// within an existing transaction, preventing concurrent upserts from reading
// stale generation/resourceVersion values.
func getFromTableForUpdate[T any](ctx context.Context, tx *sql.Tx, table, name string) (T, bool, error) {
	var zero T
	var payload []byte
	err := tx.QueryRowContext(ctx, fmt.Sprintf(`SELECT payload FROM %s WHERE name = $1 FOR UPDATE`, table), name).Scan(&payload)
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

// upsertMeta holds the minimal metadata needed for the upsert read-modify-write
// cycle. Fetching only these fields avoids deserializing the full JSONB payload.
type upsertMeta struct {
	Generation      int64
	ResourceVersion string
	CreatedAt       string
	SpecHash        string
}

// getUpsertMetaForUpdate reads only generation, resourceVersion, createdAt, and
// spec_hash under a FOR UPDATE lock. This is O(1) in the row size because it
// avoids deserializing the JSONB payload column.
func getUpsertMetaForUpdate(ctx context.Context, tx *sql.Tx, table, name string) (upsertMeta, bool, error) {
	var m upsertMeta
	err := tx.QueryRowContext(ctx,
		fmt.Sprintf(`SELECT
			(payload->'metadata'->>'generation')::bigint,
			COALESCE(payload->'metadata'->>'resourceVersion', '0'),
			COALESCE(payload->'metadata'->>'createdAt', ''),
			COALESCE(spec_hash, '')
		FROM %s WHERE name = $1 FOR UPDATE`, table),
		name,
	).Scan(&m.Generation, &m.ResourceVersion, &m.CreatedAt, &m.SpecHash)
	if err == sql.ErrNoRows {
		return upsertMeta{}, false, nil
	}
	if err != nil {
		return upsertMeta{}, false, err
	}
	return m, true, nil
}

// specHash computes a SHA-256 hex digest of the JSON-serialized spec.
// The hash is deterministic for the same Go struct because json.Marshal
// outputs struct fields in declaration order.
func specHash(spec any) string {
	data, err := json.Marshal(spec)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// defaultListLimit is a safety cap for all list queries. Callers that need
// more results should implement cursor-based pagination; for now this prevents
// unbounded full-table scans from a single API call.
const defaultListLimit = 1000

func listFromTable[T any](ctx context.Context, db dbExecer, table string) ([]T, error) {
	return listFromTableFiltered[T](ctx, db, table, defaultListLimit, 0, "")
}

func listFromTablePaged[T any](ctx context.Context, db dbExecer, table string, limit, offset int) ([]T, error) {
	return listFromTableFiltered[T](ctx, db, table, limit, offset, "")
}

// listFromTableFiltered is the core list helper. When namespace is non-empty
// the WHERE clause uses the per-table namespace index so LIMIT/OFFSET operate
// on the correct subset of rows.
func listFromTableFiltered[T any](ctx context.Context, db dbExecer, table string, limit, offset int, namespace string) ([]T, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}
	var rows *sql.Rows
	var err error
	if namespace != "" {
		rows, err = db.QueryContext(ctx,
			fmt.Sprintf(`SELECT payload FROM %s WHERE namespace = $1 ORDER BY name ASC LIMIT $2 OFFSET $3`, table),
			namespace, limit, offset,
		)
	} else {
		rows, err = db.QueryContext(ctx,
			fmt.Sprintf(`SELECT payload FROM %s ORDER BY name ASC LIMIT $1 OFFSET $2`, table),
			limit, offset,
		)
	}
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

// listFromTableCursor implements keyset/cursor-based pagination. It returns
// rows with name > afterName (the cursor), ordered by name ASC, limited to
// `limit` rows. This is O(limit) regardless of depth, unlike OFFSET which is
// O(offset+limit). When namespace is non-empty, only rows in that namespace
// are considered.
func listFromTableCursor[T any](ctx context.Context, db dbExecer, table string, limit int, afterName, namespace string) ([]T, error) {
	if limit <= 0 {
		limit = defaultListLimit
	}

	var rows *sql.Rows
	var err error
	switch {
	case namespace != "" && afterName != "":
		rows, err = db.QueryContext(ctx,
			fmt.Sprintf(`SELECT payload FROM %s WHERE namespace = $1 AND name > $2 ORDER BY name ASC LIMIT $3`, table),
			namespace, afterName, limit,
		)
	case namespace != "":
		rows, err = db.QueryContext(ctx,
			fmt.Sprintf(`SELECT payload FROM %s WHERE namespace = $1 ORDER BY name ASC LIMIT $2`, table),
			namespace, limit,
		)
	case afterName != "":
		rows, err = db.QueryContext(ctx,
			fmt.Sprintf(`SELECT payload FROM %s WHERE name > $1 ORDER BY name ASC LIMIT $2`, table),
			afterName, limit,
		)
	default:
		rows, err = db.QueryContext(ctx,
			fmt.Sprintf(`SELECT payload FROM %s ORDER BY name ASC LIMIT $1`, table),
			limit,
		)
	}
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

func deleteFromTable(ctx context.Context, db *sql.DB, table, name string) (bool, error) {
	result, err := db.ExecContext(ctx, fmt.Sprintf(`DELETE FROM %s WHERE name = $1`, table), name)
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

func upsertAgentSQL(ctx context.Context, db dbExecer, name string, item resources.Agent) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertAgentSystemSQL(ctx context.Context, db dbExecer, name string, item resources.AgentSystem) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertModelEndpointSQL(ctx context.Context, db dbExecer, name string, item resources.ModelEndpoint) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertToolSQL(ctx context.Context, db dbExecer, name string, item resources.Tool) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertSecretSQL(ctx context.Context, db dbExecer, name string, item resources.Secret) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertMemorySQL(ctx context.Context, db dbExecer, name string, item resources.Memory) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertAgentPolicySQL(ctx context.Context, db dbExecer, name string, item resources.AgentPolicy) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertAgentRoleSQL(ctx context.Context, db dbExecer, name string, item resources.AgentRole) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertToolPermissionSQL(ctx context.Context, db dbExecer, name string, item resources.ToolPermission) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertToolApprovalSQL(ctx context.Context, db dbExecer, name string, item resources.ToolApproval) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertTaskSQL(ctx context.Context, db dbExecer, name string, item resources.Task) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	leaseUntil := parseTimestampPtr(item.Status.LeaseUntil)
	nextAttemptAt := parseTimestampPtr(item.Status.NextAttemptAt)
	_, err = db.ExecContext(ctx,
		`INSERT INTO tasks(name, namespace, system_ref, mode, status_phase, assigned_worker,
		     claimed_by, lease_until, next_attempt_at, priority, spec_hash, payload, updated_at)
		 VALUES($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12::jsonb, NOW())
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
		     spec_hash = EXCLUDED.spec_hash,
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
		specHash(item.Spec),
		string(payload),
	)
	return err
}

func upsertTaskScheduleSQL(ctx context.Context, db dbExecer, name string, item resources.TaskSchedule) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertTaskWebhookSQL(ctx context.Context, db dbExecer, name string, item resources.TaskWebhook) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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

func upsertWorkerSQL(ctx context.Context, db dbExecer, name string, item resources.Worker) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
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
func updateTaskInTx(ctx context.Context, tx *sql.Tx, name string, task resources.Task) error {
	payload, err := json.Marshal(task)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
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

func claimTaskSQL(ctx context.Context, db *sql.DB, name, workerID string, lease time.Duration) (resources.Task, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return resources.Task{}, false, err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT payload FROM tasks WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
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
	if err := updateTaskInTx(ctx, tx, name, task); err != nil {
		return resources.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.Task{}, false, err
	}
	return task, true, nil
}

// WorkerClaimHints carries worker capability filters that are pushed into the
// SQL WHERE clause so a worker never fetches tasks it cannot run.
// Zero-value fields mean "no filter".
type WorkerClaimHints struct {
	AssignedWorker  string // match tasks assigned to this worker (or unassigned)
	Region          string // match tasks with this region requirement (case-insensitive), or no requirement
	RequiresGPU     bool   // when false, skip GPU filter; when true only tasks with gpu=true
	SupportedModels []string // if non-empty, only tasks whose model requirement is in this list (or unset)
}

func claimNextDueTaskSQL(ctx context.Context, db *sql.DB, workerID string, lease time.Duration, hints WorkerClaimHints, matches func(resources.Task) bool) (resources.Task, bool, error) {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return resources.Task{}, false, err
	}
	defer tx.Rollback()

	// Build extra WHERE clauses from hints to avoid fetching tasks the worker
	// cannot execute, which would starve workers with specialized requirements.
	extraWhere := ""
	args := []any{}
	argIdx := 1

	if hints.Region != "" {
		argIdx++
		extraWhere += fmt.Sprintf(` AND (
			payload->'spec'->'requirements'->>'region' IS NULL
			OR payload->'spec'->'requirements'->>'region' = ''
			OR LOWER(payload->'spec'->'requirements'->>'region') = LOWER($%d)
		)`, argIdx)
		args = append(args, hints.Region)
	}
	if hints.AssignedWorker != "" {
		argIdx++
		extraWhere += fmt.Sprintf(` AND (
			assigned_worker = ''
			OR LOWER(assigned_worker) = LOWER($%d)
		)`, argIdx)
		args = append(args, hints.AssignedWorker)
	}

	query := fmt.Sprintf(
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
		   %s
		 ORDER BY updated_at ASC
		 FOR UPDATE SKIP LOCKED
		 LIMIT 64`, extraWhere)

	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return resources.Task{}, false, err
	}

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
			rows.Close()
			return resources.Task{}, false, err
		}
		var task resources.Task
		if err := json.Unmarshal(payload, &task); err != nil {
			rows.Close()
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
	rowsErr := rows.Err()
	rows.Close()
	if rowsErr != nil {
		return resources.Task{}, false, rowsErr
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
	if err := updateTaskInTx(ctx, tx, selectedName, task); err != nil {
		return resources.Task{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.Task{}, false, err
	}
	return task, true, nil
}

// renewTaskLeaseSQL updates only the lease and heartbeat fields without
// re-serializing the entire JSONB payload. This avoids write amplification on
// the hot heartbeat path (called every ~15-30s per running task).
func renewTaskLeaseSQL(ctx context.Context, db *sql.DB, name, workerID string, lease time.Duration) error {
	if lease <= 0 {
		lease = 30 * time.Second
	}
	now := time.Now().UTC()
	leaseUntilTS := now.Add(lease)
	leaseUntilStr := leaseUntilTS.Format(time.RFC3339Nano)
	heartbeatStr := now.Format(time.RFC3339Nano)

	result, err := db.ExecContext(ctx,
		`UPDATE tasks SET
		     lease_until = $2,
		     payload = jsonb_set(
		         jsonb_set(
		             jsonb_set(
		                 jsonb_set(
		                     payload,
		                     '{status,leaseUntil}', to_jsonb($3::text)
		                 ),
		                 '{status,lastHeartbeat}', to_jsonb($4::text)
		             ),
		             '{status,observedGeneration}',
		             to_jsonb((payload->'metadata'->>'generation')::bigint)
		         ),
		         '{metadata,resourceVersion}',
		         to_jsonb(((payload->'metadata'->>'resourceVersion')::bigint + 1)::text)
		     ),
		     updated_at = NOW()
		 WHERE name = $1
		   AND status_phase = 'running'
		   AND LOWER(TRIM(claimed_by)) = LOWER(TRIM($5))`,
		name, &leaseUntilTS, leaseUntilStr, heartbeatStr, workerID,
	)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if affected == 0 {
		var phase, claimedBy string
		scanErr := db.QueryRowContext(ctx,
			`SELECT status_phase, claimed_by FROM tasks WHERE name = $1`, name,
		).Scan(&phase, &claimedBy)
		if scanErr == sql.ErrNoRows {
			return fmt.Errorf("task %q not found", name)
		}
		if !strings.EqualFold(strings.TrimSpace(phase), "running") {
			return fmt.Errorf("task %q is not running", name)
		}
		return fmt.Errorf("task %q is claimed by %q, not %q", name, claimedBy, workerID)
	}
	return nil
}

// ---------------------------------------------------------------------------
// Task logs
// ---------------------------------------------------------------------------

func appendTaskLogSQL(ctx context.Context, db *sql.DB, taskName, entry string) error {
	_, err := db.ExecContext(ctx, `INSERT INTO task_logs(task_name, entry, created_at) VALUES($1, $2, NOW())`, taskName, entry)
	return err
}

const maxTaskLogEntries = 500

func listTaskLogsSQL(ctx context.Context, db *sql.DB, taskName string) ([]string, error) {
	rows, err := db.QueryContext(ctx, `SELECT entry FROM task_logs WHERE task_name = $1 ORDER BY created_at ASC, id ASC LIMIT $2`, taskName, maxTaskLogEntries)
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

func deleteTaskLogsSQL(ctx context.Context, db *sql.DB, taskName string) error {
	_, err := db.ExecContext(ctx, `DELETE FROM task_logs WHERE task_name = $1`, taskName)
	return err
}

// ---------------------------------------------------------------------------
// Worker slot management
// ---------------------------------------------------------------------------

func updateWorkerInTx(ctx context.Context, tx *sql.Tx, name string, worker resources.Worker) error {
	payload, err := json.Marshal(worker)
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx,
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

func tryAcquireWorkerSlotSQL(ctx context.Context, db *sql.DB, name string) (resources.Worker, bool, error) {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return resources.Worker{}, false, err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT payload FROM workers WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
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

	if err := updateWorkerInTx(ctx, tx, name, worker); err != nil {
		return resources.Worker{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return resources.Worker{}, false, err
	}
	return worker, true, nil
}

func releaseWorkerSlotSQL(ctx context.Context, db *sql.DB, name string) error {
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	var payload []byte
	err = tx.QueryRowContext(ctx, `SELECT payload FROM workers WHERE name = $1 FOR UPDATE`, name).Scan(&payload)
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

	if err := updateWorkerInTx(ctx, tx, name, worker); err != nil {
		return err
	}
	return tx.Commit()
}

// ---------------------------------------------------------------------------
// Webhook deduplication
// ---------------------------------------------------------------------------

func upsertWebhookDedupeSQL(ctx context.Context, db *sql.DB, endpointID, eventID, taskName string, expiresAt time.Time) error {
	_, err := db.ExecContext(ctx,
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

// tryInsertWebhookDedupeSQL atomically inserts a dedup entry only if one
// does not already exist (or has expired). Returns (taskName, true) if a
// live duplicate was found, or ("", false) if the insert succeeded.
func tryInsertWebhookDedupeSQL(ctx context.Context, db *sql.DB, endpointID, eventID, taskName string, expiresAt, now time.Time) (string, bool, error) {
	var existingTask string
	err := db.QueryRowContext(ctx,
		`WITH pruned AS (
			DELETE FROM webhook_dedupe WHERE endpoint_id = $1 AND event_id = $2 AND expires_at <= $5
		), ins AS (
			INSERT INTO webhook_dedupe(endpoint_id, event_id, task_name, expires_at, created_at)
			VALUES($1, $2, $3, $4, NOW())
			ON CONFLICT(endpoint_id, event_id) DO NOTHING
			RETURNING task_name
		)
		SELECT COALESCE(
			(SELECT task_name FROM ins),
			(SELECT task_name FROM webhook_dedupe WHERE endpoint_id = $1 AND event_id = $2 AND expires_at > $5)
		)`,
		endpointID, eventID, taskName, expiresAt.UTC(), now.UTC(),
	).Scan(&existingTask)
	if err == sql.ErrNoRows {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if existingTask == taskName {
		return "", false, nil
	}
	return existingTask, true, nil
}

func getWebhookDedupeSQL(ctx context.Context, db *sql.DB, endpointID, eventID string, now time.Time) (string, bool, error) {
	var taskName string
	err := db.QueryRowContext(ctx,
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

func pruneWebhookDedupeSQL(ctx context.Context, db *sql.DB, now time.Time) error {
	_, err := db.ExecContext(ctx, `DELETE FROM webhook_dedupe WHERE expires_at <= $1`, now.UTC())
	return err
}

func upsertMcpServerSQL(ctx context.Context, db dbExecer, name string, item resources.McpServer) error {
	payload, err := json.Marshal(item)
	if err != nil {
		return err
	}
	_, err = db.ExecContext(ctx,
		`INSERT INTO mcp_servers(name, namespace, status_phase, payload, updated_at)
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
		// Returning nil here clears lease/retry timestamps which can cause
		// unexpected task re-scheduling. Bad timestamp values indicate corrupt
		// task state; emit a stderr line for diagnostics rather than silently
		// proceeding.
		fmt.Printf("store: WARNING: parseTimestampPtr: ignoring unparseable timestamp %q: %v\n", v, err)
		return nil
	}
	return &t
}
