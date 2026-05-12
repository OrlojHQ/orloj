package startup

import (
	"context"
	"database/sql"
	"fmt"
	"log"
	"time"

	"github.com/OrlojHQ/orloj/store"
)

// StoreSet holds all resource stores. Schedule/webhook stores are nil when
// IncludeScheduleStores is false (worker-only mode).
type StoreSet struct {
	Agents          *store.AgentStore
	AgentSystems    *store.AgentSystemStore
	ModelEPs        *store.ModelEndpointStore
	Tools           *store.ToolStore
	Secrets         *store.SecretStore
	SealedSecrets   *store.SealedSecretStore
	SealingKeys     *store.SealingKeyStore
	Memories        *store.MemoryStore
	ContextAdapters *store.ContextAdapterStore
	Policies        *store.AgentPolicyStore
	Roles           *store.AgentRoleStore
	ToolPerms       *store.ToolPermissionStore
	ToolApprovals   *store.ToolApprovalStore
	TaskApprovals   *store.TaskApprovalStore
	Tasks           *store.TaskStore
	TaskSchedules   *store.TaskScheduleStore
	TaskWebhooks    *store.TaskWebhookStore
	WebhookDedupe   *store.WebhookDedupeStore
	Workers         *store.WorkerStore
	McpServers      *store.McpServerStore
	EvalDatasets    *store.EvalDatasetStore
	EvalRuns        *store.EvalRunStore
	LocalAdmins     *store.LocalAdminStore
	APITokens       *store.APITokenStore
	AuthSessions    *store.AuthSessionStore
	DB              *sql.DB
}

// Close closes the database connection if one is open.
func (s *StoreSet) Close() {
	if s.DB != nil {
		_ = s.DB.Close()
	}
}

type StoreConfig struct {
	Backend             string // "memory" or "postgres"
	PostgresDSN         string
	SQLDriver           string
	MaxOpenConns        int
	MaxIdleConns        int
	ConnMaxLifetime     time.Duration
	SecretEncryptionKey []byte

	// IncludeScheduleStores creates TaskSchedule, TaskWebhook, and WebhookDedupe
	// stores. Only needed by orlojd.
	IncludeScheduleStores bool
}

func OpenStores(cfg StoreConfig, logger *log.Logger) (*StoreSet, error) {
	s := &StoreSet{}

	switch cfg.Backend {
	case "memory":
		s.Agents = store.NewAgentStore()
		s.AgentSystems = store.NewAgentSystemStore()
		s.ModelEPs = store.NewModelEndpointStore()
		s.Tools = store.NewToolStore()
		s.Secrets = store.NewSecretStore()
		s.SealedSecrets = store.NewSealedSecretStore()
		s.SealingKeys = store.NewSealingKeyStore()
		s.SealingKeys.SetEncryptionKey(cfg.SecretEncryptionKey)
		s.Memories = store.NewMemoryStore()
		s.Policies = store.NewAgentPolicyStore()
		s.Roles = store.NewAgentRoleStore()
		s.ToolPerms = store.NewToolPermissionStore()
		s.ToolApprovals = store.NewToolApprovalStore()
		s.TaskApprovals = store.NewTaskApprovalStore()
		s.Tasks = store.NewTaskStore()
		s.Workers = store.NewWorkerStore()
		s.McpServers = store.NewMcpServerStore()
		s.ContextAdapters = store.NewContextAdapterStore()
		s.EvalDatasets = store.NewEvalDatasetStore()
		s.EvalRuns = store.NewEvalRunStore()
		s.LocalAdmins = store.NewLocalAdminStore()
		s.APITokens = store.NewAPITokenStore()
		s.AuthSessions = store.NewAuthSessionStore()
		if cfg.IncludeScheduleStores {
			s.TaskSchedules = store.NewTaskScheduleStore()
			s.TaskWebhooks = store.NewTaskWebhookStore()
			s.WebhookDedupe = store.NewWebhookDedupeStore()
		}
		if logger != nil {
			logger.Printf("using storage backend=%s", cfg.Backend)
		}
		return s, nil

	case "postgres":
		dsn := cfg.PostgresDSN
		if dsn == "" {
			return nil, fmt.Errorf("postgres backend selected but DSN is empty (set --postgres-dsn or ORLOJ_POSTGRES_DSN)")
		}
		driver := cfg.SQLDriver
		if driver == "" {
			driver = "pgx"
		}

		db, err := sql.Open(driver, dsn)
		if err != nil {
			return nil, fmt.Errorf("failed to open postgres with sql driver %q: %w (ensure a matching database/sql driver is linked)", driver, err)
		}
		if cfg.MaxOpenConns > 0 {
			db.SetMaxOpenConns(cfg.MaxOpenConns)
		}
		if cfg.MaxIdleConns > 0 {
			db.SetMaxIdleConns(cfg.MaxIdleConns)
		}
		if cfg.ConnMaxLifetime > 0 {
			db.SetConnMaxLifetime(cfg.ConnMaxLifetime)
		}
		// Evict idle connections after 5 minutes so firewalls/load-balancers
		// that silently close idle TCP connections don't hand us broken conns.
		db.SetConnMaxIdleTime(5 * time.Minute)

		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := db.PingContext(pingCtx); err != nil {
			pingCancel()
			_ = db.Close()
			return nil, fmt.Errorf("failed to connect to postgres: %w", err)
		}
		pingCancel()

		if err := store.EnsurePostgresSchema(db); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to ensure postgres schema: %w", err)
		}

		s.DB = db
		s.Agents = store.NewAgentStoreWithDB(db)
		s.AgentSystems = store.NewAgentSystemStoreWithDB(db)
		s.ModelEPs = store.NewModelEndpointStoreWithDB(db)
		s.Tools = store.NewToolStoreWithDB(db)
		s.Secrets = store.NewSecretStoreWithEncryption(db, cfg.SecretEncryptionKey)
		s.SealedSecrets = store.NewSealedSecretStoreWithDB(db)
		s.SealingKeys = store.NewSealingKeyStoreWithDB(db, cfg.SecretEncryptionKey)
		s.Memories = store.NewMemoryStoreWithDB(db)
		s.Policies = store.NewAgentPolicyStoreWithDB(db)
		s.Roles = store.NewAgentRoleStoreWithDB(db)
		s.ToolPerms = store.NewToolPermissionStoreWithDB(db)
		s.ToolApprovals = store.NewToolApprovalStoreWithDB(db)
		s.TaskApprovals = store.NewTaskApprovalStoreWithDB(db)
		s.Tasks = store.NewTaskStoreWithDB(db)
		s.Workers = store.NewWorkerStoreWithDB(db)
		s.McpServers = store.NewMcpServerStoreWithDB(db)
		s.ContextAdapters = store.NewContextAdapterStoreWithDB(db)
		s.EvalDatasets = store.NewEvalDatasetStoreWithDB(db)
		s.EvalRuns = store.NewEvalRunStoreWithDB(db)
		s.LocalAdmins = store.NewLocalAdminStoreWithDB(db)
		s.APITokens = store.NewAPITokenStoreWithDB(db)
		s.AuthSessions = store.NewAuthSessionStoreWithDB(db)
		if cfg.IncludeScheduleStores {
			s.TaskSchedules = store.NewTaskScheduleStoreWithDB(db)
			s.TaskWebhooks = store.NewTaskWebhookStoreWithDB(db)
			s.WebhookDedupe = store.NewWebhookDedupeStoreWithDB(db)
		}
		if logger != nil {
			logger.Printf("using storage backend=%s driver=%s", cfg.Backend, driver)
		}
		return s, nil

	default:
		return nil, fmt.Errorf("unsupported storage backend %q; expected memory or postgres", cfg.Backend)
	}
}
