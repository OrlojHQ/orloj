package store

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

const (
	kindAgent        = "Agent"
	kindAgentSystem  = "AgentSystem"
	kindModelEP      = "ModelEndpoint"
	kindTool         = "Tool"
	kindSecret       = "Secret"
	kindMemory       = "Memory"
	kindAgentPolicy  = "AgentPolicy"
	kindAgentRole    = "AgentRole"
	kindToolPerm     = "ToolPermission"
	kindTask         = "Task"
	kindTaskSchedule = "TaskSchedule"
	kindTaskWebhook  = "TaskWebhook"
	kindWorker       = "Worker"
)

// EnsurePostgresSchema runs all pending database migrations. New schema changes
// should be added as numbered SQL files in store/migrations/ (e.g.,
// 002_add_foo.up.sql). Migrations are tracked in a schema_migrations table and
// applied exactly once, in lexicographic order.
func EnsurePostgresSchema(db *sql.DB) error {
	return Migrate(db)
}

func upsertResource(db *sql.DB, kind string, name string, value any) error {
	payload, err := json.Marshal(value)
	if err != nil {
		return err
	}
	_, err = db.Exec(
		`INSERT INTO resources(kind, name, payload, updated_at)
		 VALUES($1, $2, $3::jsonb, NOW())
		 ON CONFLICT(kind, name)
		 DO UPDATE SET payload = EXCLUDED.payload, updated_at = NOW()`,
		kind,
		name,
		string(payload),
	)
	return err
}

func getResource(db *sql.DB, kind string, name string, out any) (bool, error) {
	var payload []byte
	err := db.QueryRow(`SELECT payload FROM resources WHERE kind = $1 AND name = $2`, kind, name).Scan(&payload)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if err := json.Unmarshal(payload, out); err != nil {
		return false, err
	}
	return true, nil
}

func listResources[T any](db *sql.DB, kind string) ([]T, error) {
	rows, err := db.Query(`SELECT payload FROM resources WHERE kind = $1 ORDER BY name ASC`, kind)
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

func deleteResource(db *sql.DB, kind string, name string) (bool, error) {
	result, err := db.Exec(`DELETE FROM resources WHERE kind = $1 AND name = $2`, kind, name)
	if err != nil {
		return false, err
	}
	affected, err := result.RowsAffected()
	if err != nil {
		return false, err
	}
	return affected > 0, nil
}

func appendTaskLogSQL(db *sql.DB, taskName, entry string) error {
	var exists int
	err := db.QueryRow(`SELECT 1 FROM resources WHERE kind = $1 AND name = $2`, kindTask, taskName).Scan(&exists)
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
	err := db.QueryRow(`SELECT 1 FROM resources WHERE kind = $1 AND name = $2`, kindTask, taskName).Scan(&exists)
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
