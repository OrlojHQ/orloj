package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5/pgxpool"
	pgvector "github.com/pgvector/pgvector-go"
)

// PgvectorOptions configures optional behaviour of the pgvector backend.
type PgvectorOptions struct {
	Table     string // table name, default "orloj_memory"
	Dimension int    // vector dimension override; auto-detected when 0
}

// PgvectorBackend implements PersistentMemoryBackend using PostgreSQL with the
// pgvector extension. Entries are stored with their vector embeddings, enabling
// cosine-similarity search.
type PgvectorBackend struct {
	pool      *pgxpool.Pool
	embedder  EmbeddingProvider
	table     string
	dimension int
}

// NewPgvectorBackend connects to the database, detects the embedding dimension
// (if not overridden), installs the vector extension, and creates the table
// with an HNSW index.
func NewPgvectorBackend(dsn string, embedder EmbeddingProvider, opts PgvectorOptions) (*PgvectorBackend, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("pgvector: connection string (spec.endpoint) is required")
	}
	if embedder == nil {
		return nil, fmt.Errorf("pgvector: embedding provider (spec.embedding_model → ModelEndpoint) is required")
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgvector: connect: %w", err)
	}

	table := strings.TrimSpace(opts.Table)
	if table == "" {
		table = "orloj_memory"
	}

	dim := opts.Dimension
	if dim <= 0 {
		vecs, err := embedder.Embed(ctx, []string{"dimension probe"})
		if err != nil {
			pool.Close()
			return nil, fmt.Errorf("pgvector: detect embedding dimension: %w", err)
		}
		if len(vecs) == 0 || len(vecs[0]) == 0 {
			pool.Close()
			return nil, fmt.Errorf("pgvector: embedding provider returned empty vector")
		}
		dim = len(vecs[0])
	}

	if err := initSchema(ctx, pool, table, dim); err != nil {
		pool.Close()
		return nil, err
	}

	return &PgvectorBackend{
		pool:      pool,
		embedder:  embedder,
		table:     table,
		dimension: dim,
	}, nil
}

func initSchema(ctx context.Context, pool *pgxpool.Pool, table string, dim int) error {
	stmts := []string{
		`CREATE EXTENSION IF NOT EXISTS vector`,
		fmt.Sprintf(`CREATE TABLE IF NOT EXISTS %s (
			key        TEXT PRIMARY KEY,
			value      TEXT NOT NULL,
			embedding  vector(%d),
			created_at TIMESTAMPTZ DEFAULT now(),
			updated_at TIMESTAMPTZ DEFAULT now()
		)`, table, dim),
		fmt.Sprintf(`CREATE INDEX IF NOT EXISTS %s_embedding_idx ON %s USING hnsw (embedding vector_cosine_ops)`, table, table),
	}
	for _, stmt := range stmts {
		if _, err := pool.Exec(ctx, stmt); err != nil {
			return fmt.Errorf("pgvector: schema init: %w", err)
		}
	}
	return nil
}

func (b *PgvectorBackend) Put(ctx context.Context, key, value string) error {
	vecs, err := b.embedder.Embed(ctx, []string{value})
	if err != nil {
		return fmt.Errorf("pgvector put: embed: %w", err)
	}
	if len(vecs) == 0 {
		return fmt.Errorf("pgvector put: embedding provider returned no vectors")
	}
	embedding := pgvector.NewVector(vecs[0])

	query := fmt.Sprintf(`INSERT INTO %s (key, value, embedding, updated_at)
		VALUES ($1, $2, $3, now())
		ON CONFLICT (key) DO UPDATE SET value = $2, embedding = $3, updated_at = now()`, b.table)

	_, err = b.pool.Exec(ctx, query, key, value, embedding)
	if err != nil {
		return fmt.Errorf("pgvector put: %w", err)
	}
	return nil
}

func (b *PgvectorBackend) Get(ctx context.Context, key string) (string, bool, error) {
	query := fmt.Sprintf(`SELECT value FROM %s WHERE key = $1`, b.table)
	var value string
	err := b.pool.QueryRow(ctx, query, key).Scan(&value)
	if err != nil {
		if err.Error() == "no rows in result set" {
			return "", false, nil
		}
		return "", false, fmt.Errorf("pgvector get: %w", err)
	}
	return value, true, nil
}

func (b *PgvectorBackend) Search(ctx context.Context, queryText string, topK int) ([]MemorySearchResult, error) {
	if topK <= 0 {
		topK = 10
	}

	vecs, err := b.embedder.Embed(ctx, []string{queryText})
	if err != nil {
		return nil, fmt.Errorf("pgvector search: embed query: %w", err)
	}
	if len(vecs) == 0 {
		return nil, fmt.Errorf("pgvector search: embedding provider returned no vectors")
	}
	embedding := pgvector.NewVector(vecs[0])

	query := fmt.Sprintf(
		`SELECT key, value, 1 - (embedding <=> $1) AS score
		 FROM %s
		 WHERE embedding IS NOT NULL
		 ORDER BY embedding <=> $1
		 LIMIT $2`, b.table)

	rows, err := b.pool.Query(ctx, query, embedding, topK)
	if err != nil {
		return nil, fmt.Errorf("pgvector search: %w", err)
	}
	defer rows.Close()

	var results []MemorySearchResult
	for rows.Next() {
		var r MemorySearchResult
		if err := rows.Scan(&r.Key, &r.Value, &r.Score); err != nil {
			return nil, fmt.Errorf("pgvector search: scan: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (b *PgvectorBackend) List(ctx context.Context, prefix string) ([]MemorySearchResult, error) {
	var query string
	var args []any

	if prefix == "" {
		query = fmt.Sprintf(`SELECT key, value FROM %s ORDER BY key`, b.table)
	} else {
		query = fmt.Sprintf(`SELECT key, value FROM %s WHERE key LIKE $1 ORDER BY key`, b.table)
		args = append(args, prefix+"%")
	}

	rows, err := b.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("pgvector list: %w", err)
	}
	defer rows.Close()

	var results []MemorySearchResult
	for rows.Next() {
		var r MemorySearchResult
		if err := rows.Scan(&r.Key, &r.Value); err != nil {
			return nil, fmt.Errorf("pgvector list: scan: %w", err)
		}
		results = append(results, r)
	}
	return results, rows.Err()
}

func (b *PgvectorBackend) Ping(ctx context.Context) error {
	return b.pool.Ping(ctx)
}
