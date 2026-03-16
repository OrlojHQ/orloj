package store

import (
	"database/sql"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.up.sql
var migrationFS embed.FS

// Migrate runs all pending database migrations in order. It creates a
// schema_migrations tracking table if it does not already exist and skips
// migrations that have already been applied.
func Migrate(db *sql.DB) error {
	if db == nil {
		return fmt.Errorf("db is required")
	}

	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version TEXT PRIMARY KEY,
		applied_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
	)`)
	if err != nil {
		return fmt.Errorf("create schema_migrations table: %w", err)
	}

	entries, err := fs.ReadDir(migrationFS, "migrations")
	if err != nil {
		return fmt.Errorf("read migration entries: %w", err)
	}

	var files []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".up.sql") {
			continue
		}
		files = append(files, e.Name())
	}
	sort.Strings(files)

	for _, name := range files {
		version := strings.TrimSuffix(name, ".up.sql")

		var exists int
		err := db.QueryRow(`SELECT 1 FROM schema_migrations WHERE version = $1`, version).Scan(&exists)
		if err == nil {
			continue
		}
		if err != sql.ErrNoRows {
			return fmt.Errorf("check migration %s: %w", version, err)
		}

		content, err := migrationFS.ReadFile("migrations/" + name)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", name, err)
		}

		tx, err := db.Begin()
		if err != nil {
			return fmt.Errorf("begin migration %s: %w", version, err)
		}

		if _, err := tx.Exec(string(content)); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("apply migration %s: %w", version, err)
		}

		if _, err := tx.Exec(`INSERT INTO schema_migrations(version) VALUES($1)`, version); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("record migration %s: %w", version, err)
		}

		if err := tx.Commit(); err != nil {
			return fmt.Errorf("commit migration %s: %w", version, err)
		}
	}

	return nil
}
