package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/OrlojHQ/orloj/api"
	"github.com/OrlojHQ/orloj/controllers"
	"github.com/OrlojHQ/orloj/eventbus"
	"github.com/OrlojHQ/orloj/internal/version"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/startup"
	"github.com/OrlojHQ/orloj/store"
	"github.com/OrlojHQ/orloj/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	env := startup.EnvOrDefault
	envDuration := startup.EnvDurationOrDefault
	envBool := startup.EnvBoolOrDefault
	envInt := startup.EnvIntOrDefault
	envInt64 := startup.EnvInt64OrDefault
	envUint64 := startup.EnvUint64OrDefault

	showVersion := flag.Bool("version", false, "print version and exit")
	addr := flag.String("addr", ":8080", "server listen address")
	uiPath := flag.String("ui-path", env("ORLOJ_UI_PATH", "/"), "base URL path for the web console (env: ORLOJ_UI_PATH)")
	apiKey := flag.String("api-key", env("ORLOJ_API_TOKEN", ""), "API key for bearer token auth (empty disables auth; env fallback: ORLOJ_API_TOKEN or ORLOJ_API_TOKENS)")
	authModeRaw := flag.String("auth-mode", env("ORLOJ_AUTH_MODE", "off"), "API auth mode: off|native|sso (sso is not available in this distribution)")
	authSessionTTL := flag.Duration("auth-session-ttl", envDuration("ORLOJ_AUTH_SESSION_TTL", 24*time.Hour), "session TTL for local auth mode")
	authResetAdminUsername := flag.String("auth-reset-admin-username", env("ORLOJ_AUTH_RESET_ADMIN_USERNAME", ""), "optional username for one-shot local admin password reset")
	authResetAdminPassword := flag.String("auth-reset-admin-password", env("ORLOJ_AUTH_RESET_ADMIN_PASSWORD", ""), "one-shot local admin password reset value; when set, reset password and exit")
	trustedProxies := flag.String("trusted-proxies", env("ORLOJ_TRUSTED_PROXIES", ""), "comma-separated CIDRs of reverse proxies whose X-Forwarded-For/X-Real-IP headers are trusted (env: ORLOJ_TRUSTED_PROXIES)")
	reconcile := flag.Duration("reconcile-interval", 2*time.Second, "agent reconcile interval")
	runTaskWorker := flag.Bool("run-task-worker", false, "run embedded task worker in orlojd process")
	embeddedWorker := flag.Bool("embedded-worker", false, "alias for --run-task-worker")
	taskWorkerID := flag.String("task-worker-id", "embedded-worker", "worker id for embedded task worker")
	taskLeaseDuration := flag.Duration("task-lease-duration", 30*time.Second, "task lease duration for embedded worker")
	taskHeartbeatInterval := flag.Duration("task-heartbeat-interval", 10*time.Second, "task lease heartbeat interval for embedded worker")
	embeddedWorkerMaxConcurrentTasks := flag.Int("embedded-worker-max-concurrent-tasks", envInt("ORLOJ_EMBEDDED_WORKER_MAX_CONCURRENT_TASKS", 1), "max concurrent tasks for embedded worker (same semantics as orlojworker --max-concurrent-tasks; env: ORLOJ_EMBEDDED_WORKER_MAX_CONCURRENT_TASKS)")
	taskWorkerRegion := flag.String("task-worker-region", env("ORLOJ_TASK_WORKER_REGION", "default"), "region for embedded task worker")
	taskExecutionMode := flag.String("task-execution-mode", env("ORLOJ_TASK_EXECUTION_MODE", "sequential"), "task execution mode: sequential|message-driven")
	modelSecretEnvPrefix := flag.String("model-secret-env-prefix", env("ORLOJ_MODEL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve ModelEndpoint.spec.auth.secretRef")
	toolIsolationBackend := flag.String("tool-isolation-backend", env("ORLOJ_TOOL_ISOLATION_BACKEND", "none"), "isolated tool executor backend for container sandboxing: none|container")
	toolContainerRuntime := flag.String("tool-container-runtime", env("ORLOJ_TOOL_CONTAINER_RUNTIME", "docker"), "container runtime binary for isolated tool execution")
	toolContainerImage := flag.String("tool-container-image", env("ORLOJ_TOOL_CONTAINER_IMAGE", "curlimages/curl:8.8.0"), "container image used by isolated tool execution")
	toolContainerNetwork := flag.String("tool-container-network", env("ORLOJ_TOOL_CONTAINER_NETWORK", "none"), "container network mode for isolated tools")
	toolContainerMemory := flag.String("tool-container-memory", env("ORLOJ_TOOL_CONTAINER_MEMORY", "128m"), "container memory limit for isolated tools")
	toolContainerCPUs := flag.String("tool-container-cpus", env("ORLOJ_TOOL_CONTAINER_CPUS", "0.50"), "container CPU limit for isolated tools")
	toolContainerPidsLimit := flag.Int("tool-container-pids-limit", 64, "container pids limit for isolated tools")
	toolContainerUser := flag.String("tool-container-user", env("ORLOJ_TOOL_CONTAINER_USER", "65532:65532"), "container user for isolated tools")
	toolContainerMaxMemory := flag.String("tool-container-max-memory", env("ORLOJ_TOOL_CONTAINER_MAX_MEMORY", ""), "operator ceiling for per-tool/McpServer resources.memory (empty = unbounded)")
	toolContainerMaxCPUs := flag.String("tool-container-max-cpus", env("ORLOJ_TOOL_CONTAINER_MAX_CPUS", ""), "operator ceiling for per-tool/McpServer resources.cpus (empty = unbounded)")
	toolContainerMaxPidsLimit := flag.Int("tool-container-max-pids-limit", envInt("ORLOJ_TOOL_CONTAINER_MAX_PIDS_LIMIT", 0), "operator ceiling for per-tool/McpServer resources.pids_limit (0 = unbounded)")
	toolSecretEnvPrefix := flag.String("tool-secret-env-prefix", env("ORLOJ_TOOL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve Tool.spec.auth.secretRef")
	toolWASMModule := flag.String("tool-wasm-module", env("ORLOJ_TOOL_WASM_MODULE", ""), "default wasm module path (per-tool spec.wasm.module takes precedence)")
	toolWASMEntrypoint := flag.String("tool-wasm-entrypoint", env("ORLOJ_TOOL_WASM_ENTRYPOINT", "run"), "default wasm entrypoint function")
	toolWASMMemoryBytes := flag.Int64("tool-wasm-memory-bytes", envInt64("ORLOJ_TOOL_WASM_MEMORY_BYTES", 64*1024*1024), "default max wasm runtime memory bytes")
	toolWASMFuel := flag.Uint64("tool-wasm-fuel", envUint64("ORLOJ_TOOL_WASM_FUEL", 1000000), "default wasm execution fuel limit")
	toolWASMWASI := flag.Bool("tool-wasm-wasi", envBool("ORLOJ_TOOL_WASM_WASI", true), "default: enable WASI host functions for wasm tools")
	toolWASMCacheDir := flag.String("tool-wasm-cache-dir", env("ORLOJ_TOOL_WASM_CACHE_DIR", ""), "disk cache directory for remote WASM modules (default: ~/.orloj/wasm-cache)")
	cliToolAllowedCommands := flag.String("cli-tool-allowed-commands", env("ORLOJ_CLI_TOOL_ALLOWED_COMMANDS", ""), "comma-separated allowlist of commands for CLI tools (empty allows all)")
	cliToolMaxArgvLength := flag.Int("cli-tool-max-argv-length", envInt("ORLOJ_CLI_TOOL_MAX_ARGV_LENGTH", 4096), "max total argv byte length for CLI tool invocations")
	eventBusBackend := flag.String("event-bus-backend", env("ORLOJ_EVENT_BUS_BACKEND", "memory"), "event bus backend: memory|nats")
	natsURL := flag.String("nats-url", env("ORLOJ_NATS_URL", "nats://127.0.0.1:4222"), "NATS server URL used when --event-bus-backend=nats")
	natsSubjectPrefix := flag.String("nats-subject-prefix", env("ORLOJ_NATS_SUBJECT_PREFIX", "orloj.controlplane"), "NATS subject prefix used when --event-bus-backend=nats")
	agentMessageBusBackend := flag.String("agent-message-bus-backend", env("ORLOJ_AGENT_MESSAGE_BUS_BACKEND", "none"), "runtime agent message bus backend: none|memory|nats-jetstream")
	agentMessageNATSURL := flag.String("agent-message-nats-url", env("ORLOJ_AGENT_MESSAGE_NATS_URL", env("ORLOJ_NATS_URL", "nats://127.0.0.1:4222")), "NATS server URL used when --agent-message-bus-backend=nats-jetstream")
	agentMessageSubjectPrefix := flag.String("agent-message-subject-prefix", env("ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX", "orloj.agentmsg"), "runtime agent message subject prefix")
	agentMessageStreamName := flag.String("agent-message-stream-name", env("ORLOJ_AGENT_MESSAGE_STREAM", "ORLOJ_AGENT_MESSAGES"), "JetStream stream name for runtime agent messages")
	agentMessageHistoryMax := flag.Int("agent-message-history-max", 2048, "in-memory runtime agent message history capacity")
	agentMessageDedupeWindow := flag.Duration("agent-message-dedupe-window", 2*time.Minute, "in-memory runtime agent message dedupe window")
	secretEncryptionKeyRaw := flag.String("secret-encryption-key", env("ORLOJ_SECRET_ENCRYPTION_KEY", ""), "256-bit AES key (hex or base64) for encrypting Secret resource data at rest")
	storageBackend := flag.String("storage-backend", "memory", "state backend: memory|postgres")
	postgresDSN := flag.String("postgres-dsn", os.Getenv("ORLOJ_POSTGRES_DSN"), "postgres DSN (required when --storage-backend=postgres)")
	sqlDriver := flag.String("sql-driver", "pgx", "database/sql driver name used for --storage-backend=postgres")
	postgresMaxOpenConns := flag.Int("postgres-max-open-conns", 20, "max open postgres connections")
	postgresMaxIdleConns := flag.Int("postgres-max-idle-conns", 10, "max idle postgres connections")
	postgresConnMaxLifetime := flag.Duration("postgres-conn-max-lifetime", 30*time.Minute, "max lifetime of postgres connections")
	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		os.Exit(0)
	}

	authMode, authModeErr := parseAuthMode(*authModeRaw)
	if authModeErr != nil {
		logger := telemetry.NewBridgeLogger(telemetry.NewLogger("orlojd"))
		logger.Fatalf("%v", authModeErr)
	}

	slogger := telemetry.NewLogger("orlojd")
	logger := telemetry.NewBridgeLogger(slogger)

	if authMode == api.AuthModeNative && strings.TrimSpace(os.Getenv("ORLOJ_SETUP_TOKEN")) == "" {
		logger.Printf("WARNING: auth mode is native but ORLOJ_SETUP_TOKEN is not set; the first POST to /v1/auth/setup will create the admin account without a setup secret")
	}

	otelShutdown, otelErr := telemetry.Init(context.Background(), telemetry.Config{
		ServiceName: "orlojd",
	})
	if otelErr != nil {
		logger.Printf("opentelemetry init failed (tracing disabled): %v", otelErr)
	} else {
		defer func() {
			shutdownCtx, c := context.WithTimeout(context.Background(), 5*time.Second)
			defer c()
			_ = otelShutdown(shutdownCtx)
		}()
	}

	secretEncryptionKey, err := startup.ParseSecretEncryptionKey(*secretEncryptionKeyRaw)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	startup.LogSecretEncryption(logger, secretEncryptionKey)

	stores, err := startup.OpenStores(startup.StoreConfig{
		Backend:               strings.ToLower(strings.TrimSpace(*storageBackend)),
		PostgresDSN:           strings.TrimSpace(*postgresDSN),
		SQLDriver:             *sqlDriver,
		MaxOpenConns:          *postgresMaxOpenConns,
		MaxIdleConns:          *postgresMaxIdleConns,
		ConnMaxLifetime:       *postgresConnMaxLifetime,
		SecretEncryptionKey:   secretEncryptionKey,
		IncludeScheduleStores: true,
	}, logger)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	defer stores.Close()
	if len(secretEncryptionKey) > 0 {
		activeSealingKey, err := stores.SealingKeys.EnsureActive(context.Background())
		if err != nil {
			logger.Printf("WARNING: failed to initialize active sealing key: %v", err)
		} else {
			logger.Printf("sealed secret key active key_id=%s", activeSealingKey.KeyID)
		}
	} else {
		logger.Printf("sealed secret key initialization skipped: ORLOJ_SECRET_ENCRYPTION_KEY is not set")
	}
	if strings.TrimSpace(*authResetAdminPassword) != "" {
		if err := runLocalAdminPasswordReset(stores, strings.TrimSpace(*authResetAdminUsername), strings.TrimSpace(*authResetAdminPassword)); err != nil {
			logger.Fatalf("admin password reset failed: %v", err)
		}
		logger.Printf("admin password reset completed")
		return
	}

	modelGateway := agentruntime.NewModelRouter(agentruntime.ModelRouterConfig{
		Endpoints:       stores.ModelEPs,
		Secrets:         stores.Secrets,
		SecretEnvPrefix: *modelSecretEnvPrefix,
	})
	taskExecutor := agentruntime.NewTaskExecutorWithRuntime(logger, nil, modelGateway, nil)
	extensions := agentruntime.DefaultExtensions()
	logger.Printf("model routing: endpoint-driven secret_env_prefix=%s", *modelSecretEnvPrefix)

	runtime := agentruntime.NewManager(logger)
	agentController := controllers.NewAgentController(stores.Agents, runtime, logger, *reconcile)
	agentSystemController := controllers.NewAgentSystemController(stores.AgentSystems, logger, *reconcile)
	modelEndpointController := controllers.NewModelEndpointController(stores.ModelEPs, logger, 5*time.Second)
	toolController := controllers.NewToolController(stores.Tools, logger, 5*time.Second)
	mcpServerController := controllers.NewMcpServerController(stores.McpServers, stores.Tools, logger, 10*time.Second)
	mcpSecretResolver := agentruntime.NewChainSecretResolver(
		agentruntime.NewStoreSecretResolver(stores.Secrets, "value"),
		agentruntime.NewEnvSecretResolver("ORLOJ_SECRET_"),
	)
	mcpSessionManager := agentruntime.NewMcpSessionManager(mcpSecretResolver)
	mcpSessionManager.SetContainerConfig(agentruntime.ContainerToolRuntimeConfig{
		RuntimeBinary: *toolContainerRuntime,
		Network:       "bridge",
		Memory:        *toolContainerMemory,
		CPUs:          *toolContainerCPUs,
		PidsLimit:     *toolContainerPidsLimit,
	})
	mcpServerController.SetSessionManager(mcpSessionManager)
	memoryBackendRegistry := agentruntime.NewPersistentMemoryBackendRegistry()
	memoryController := controllers.NewMemoryController(stores.Memories, logger, 5*time.Second)
	memoryController.SetBackendRegistry(memoryBackendRegistry)
	memoryController.SetSecretStore(stores.Secrets)
	memoryController.SetModelEndpointStore(stores.ModelEPs)
	policyController := controllers.NewPolicyController(stores.Policies, logger, 5*time.Second)
	secretController := controllers.NewSecretController(stores.Secrets, logger, 5*time.Second)
	sealedSecretController := controllers.NewSealedSecretController(stores.SealedSecrets, stores.Secrets, stores.SealingKeys, logger, 5*time.Second, 60*time.Second)
	taskController := controllers.NewTaskController(
		stores.Tasks, stores.AgentSystems, stores.Agents, stores.Tools,
		stores.Memories, stores.Policies, stores.Workers, logger, *reconcile,
	)
	taskSchedulerController := controllers.NewTaskSchedulerController(stores.Tasks, stores.Workers, logger, *reconcile, 20*time.Second)
	taskScheduleController := controllers.NewTaskScheduleController(stores.TaskSchedules, stores.Tasks, logger, *reconcile)
	workerController := controllers.NewWorkerController(stores.Workers, logger, *reconcile, 20*time.Second)
	taskController.ConfigureWorker(*taskWorkerID, *taskLeaseDuration, *taskHeartbeatInterval)
	taskController.SetExecutionMode(*taskExecutionMode)
	taskController.SetGovernanceStores(stores.Roles, stores.ToolPerms)
	taskController.SetToolApprovalStore(stores.ToolApprovals)
	taskController.SetTaskApprovalStore(stores.TaskApprovals)
	taskController.SetModelEndpointStore(stores.ModelEPs)
	taskController.SetContextAdapterStore(stores.ContextAdapters)
	taskController.SetExecutor(taskExecutor)
	taskController.SetExtensions(extensions)
	wasmRuntimeCfg := startup.IsolatedToolRuntimeConfig{
		WASMModule:      *toolWASMModule,
		WASMEntrypoint:  *toolWASMEntrypoint,
		WASMMemoryBytes: *toolWASMMemoryBytes,
		WASMFuel:        *toolWASMFuel,
		WASMWASI:        *toolWASMWASI,
		WASMCacheDir:    *toolWASMCacheDir,
		SecretEnvPrefix: *toolSecretEnvPrefix,
		Secrets:         stores.Secrets,
	}
	isolatedToolRuntime, err := startup.NewIsolatedToolRuntime(startup.IsolatedToolRuntimeConfig{
		Backend:          *toolIsolationBackend,
		ContainerRuntime: *toolContainerRuntime,
		ContainerImage:   *toolContainerImage,
		ContainerNetwork: *toolContainerNetwork,
		ContainerMemory:  *toolContainerMemory,
		ContainerCPUs:    *toolContainerCPUs,
		ContainerPids:    *toolContainerPidsLimit,
		ContainerUser:    *toolContainerUser,
		SecretEnvPrefix:  *toolSecretEnvPrefix,
		Secrets:          stores.Secrets,
	}, logger)
	if err != nil {
		logger.Fatalf("failed to configure isolated tool runtime: %v", err)
	}
	wasmToolRuntime, closeWasm, err := startup.NewWASMToolRuntime(wasmRuntimeCfg, logger)
	if err != nil {
		logger.Fatalf("failed to configure wasm tool runtime: %v", err)
	}
	if closeWasm != nil {
		defer closeWasm()
	}
	taskController.SetIsolatedToolRuntime(isolatedToolRuntime)
	taskController.SetWasmToolRuntime(wasmToolRuntime)
	taskController.SetMcpRuntime(mcpSessionManager, stores.McpServers)

	cliStoreResolver := agentruntime.NewStoreSecretResolver(stores.Secrets, "value")
	cliEnvResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(*toolSecretEnvPrefix))
	cliSecretResolver := agentruntime.NewChainSecretResolver(cliStoreResolver, cliEnvResolver)
	taskController.SetCliToolRuntime(agentruntime.CLIToolRuntimeConfig{
		AllowedCommands: startup.ParseCSV(*cliToolAllowedCommands),
		MaxArgvLength:   *cliToolMaxArgvLength,
	}, cliSecretResolver)

	var requestAuthorizer api.RequestAuthorizer
	if strings.TrimSpace(*apiKey) != "" {
		requestAuthorizer = api.NewAPIKeyAuthorizer(*apiKey)
	}
	server := api.NewServerWithOptions(api.Stores{
		Agents:          stores.Agents,
		AgentSystems:    stores.AgentSystems,
		ModelEPs:        stores.ModelEPs,
		Tools:           stores.Tools,
		Secrets:         stores.Secrets,
		SealedSecrets:   stores.SealedSecrets,
		SealingKeys:     stores.SealingKeys,
		Memories:        stores.Memories,
		ContextAdapters: stores.ContextAdapters,
		Policies:        stores.Policies,
		AgentRoles:      stores.Roles,
		ToolPerms:       stores.ToolPerms,
		ToolApprovals:   stores.ToolApprovals,
		TaskApprovals:   stores.TaskApprovals,
		Tasks:           stores.Tasks,
		TaskSchedules:   stores.TaskSchedules,
		TaskWebhooks:    stores.TaskWebhooks,
		WebhookDedupe:   stores.WebhookDedupe,
		Workers:         stores.Workers,
		McpServers:      stores.McpServers,
		LocalAdmins:     stores.LocalAdmins,
		APITokens:       stores.APITokens,
		AuthSessions:    stores.AuthSessions,
	}, runtime, logger, api.ServerOptions{
		Authorizer:     requestAuthorizer,
		Extensions:     extensions,
		AuthMode:       authMode,
		SessionTTL:     *authSessionTTL,
		UIBasePath:     *uiPath,
		TrustedProxies: *trustedProxies,
		ContainerResourceCeiling: resources.ContainerResourceCeiling{
			MaxMemory:    *toolContainerMaxMemory,
			MaxCPUs:      *toolContainerMaxCPUs,
			MaxPidsLimit: *toolContainerMaxPidsLimit,
		},
	})
	bus, closeBus := newEventBus(logger, *eventBusBackend, *natsURL, *natsSubjectPrefix)
	if closeBus != nil {
		defer closeBus()
	}
	agentMessageBus, closeAgentMessageBus := startup.NewAgentMessageBus(
		logger, *agentMessageBusBackend, *agentMessageNATSURL,
		*agentMessageSubjectPrefix, *agentMessageStreamName,
		*agentMessageHistoryMax, *agentMessageDedupeWindow,
	)
	if closeAgentMessageBus != nil {
		defer closeAgentMessageBus()
	}
	server.SetEventBus(bus)
	server.SetMemoryBackends(memoryBackendRegistry)
	taskController.SetEventBus(bus)
	taskController.SetAgentMessageBus(agentMessageBus)
	taskSchedulerController.SetEventBus(bus)
	taskScheduleController.SetEventBus(bus)
	workerController.SetEventBus(bus)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	var wg sync.WaitGroup

	startBackground := func(fn func()) {
		wg.Add(1)
		go func() {
			defer wg.Done()
			fn()
		}()
	}

	if strings.EqualFold(strings.TrimSpace(*taskExecutionMode), "message-driven") {
		logger.Printf("agent runtime reconciliation disabled in message-driven mode")
	} else {
		startBackground(func() { agentController.Start(ctx) })
	}
	startBackground(func() { agentSystemController.Start(ctx) })
	startBackground(func() { modelEndpointController.Start(ctx) })
	startBackground(func() { toolController.Start(ctx) })
	startBackground(func() { memoryController.Start(ctx) })
	startBackground(func() { policyController.Start(ctx) })
	startBackground(func() { secretController.Start(ctx) })
	startBackground(func() { sealedSecretController.Start(ctx) })
	startBackground(func() { taskSchedulerController.Start(ctx) })
	startBackground(func() { taskScheduleController.Start(ctx) })
	startBackground(func() { workerController.Start(ctx) })
	startBackground(func() { mcpServerController.Start(ctx) })
	startBackground(func() { mcpSessionManager.StartReaper(ctx, 30*time.Second) })
	startBackground(func() {
		ticker := time.NewTicker(60 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_ = stores.WebhookDedupe.PruneExpired(ctx, time.Now())
			}
		}
	})
	if *runTaskWorker || *embeddedWorker {
		startBackground(func() {
			startup.HeartbeatWorkerRegistration(ctx, stores.Workers, logger, *taskWorkerID, resources.WorkerSpec{
				Region:             *taskWorkerRegion,
				MaxConcurrentTasks: *embeddedWorkerMaxConcurrentTasks,
			}, *taskHeartbeatInterval)
		})
		startBackground(func() { taskController.Start(ctx) })
		if strings.EqualFold(strings.TrimSpace(*taskExecutionMode), "message-driven") {
			if agentMessageBus == nil {
				logger.Printf("embedded runtime inbox consumer disabled: agent message bus backend is none")
			} else {
				consumer := agentruntime.NewAgentMessageConsumerManager(
					agentMessageBus, stores.Agents, stores.AgentSystems, stores.Tasks, logger,
					agentruntime.AgentMessageConsumerOptions{
						WorkerID:            *taskWorkerID,
						RefreshEvery:        10 * time.Second,
						DedupeWindow:        10 * time.Minute,
						LeaseExtendDuration: *taskLeaseDuration,
						Executor:            taskExecutor,
						Tools:               stores.Tools,
						Roles:               stores.Roles,
						ToolPermissions:     stores.ToolPerms,
						IsolatedToolRuntime: isolatedToolRuntime,
						WasmToolRuntime:     wasmToolRuntime,
						McpSessionManager:   mcpSessionManager,
						McpServerStore:      stores.McpServers,
						SecretResolver:      cliSecretResolver,
						Extensions:          extensions,
						Memories:            stores.Memories,
						MemoryBackends:      memoryBackendRegistry,
						ModelEndpoints:      stores.ModelEPs,
						ToolApprovals:       stores.ToolApprovals,
						TaskApprovals:       stores.TaskApprovals,
						Policies:            stores.Policies,
						ContextAdapters:     stores.ContextAdapters,
					},
				)
				startBackground(func() { consumer.Start(ctx) })
				logger.Printf("embedded runtime inbox consumers enabled refresh=%s dedupe=%s", (10 * time.Second).String(), (10 * time.Minute).String())
			}
		}
		logger.Printf("embedded task worker enabled id=%s lease=%s max_concurrent_tasks=%d", *taskWorkerID, taskLeaseDuration.String(), *embeddedWorkerMaxConcurrentTasks)
	}

	httpServer := &http.Server{
		Addr:              *addr,
		Handler:           telemetry.RequestIDMiddleware(server.Handler()),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       120 * time.Second,
		// WriteTimeout is deliberately omitted: watch/SSE endpoints stream
		// indefinitely and a global write deadline would break them. Per-
		// handler timeouts via http.TimeoutHandler should be used for non-
		// streaming routes instead.
	}

	go func() {
		<-ctx.Done()
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer shutdownCancel()
		_ = httpServer.Shutdown(shutdownCtx)
	}()

	logger.Printf("API server listening on %s", *addr)
	if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		logger.Fatalf("server error: %v", err)
	}
	wg.Wait()
}

func runLocalAdminPasswordReset(stores *startup.StoreSet, username, password string) error {
	if stores == nil || stores.LocalAdmins == nil || stores.AuthSessions == nil {
		return fmt.Errorf("auth stores are not initialized")
	}
	if err := store.ValidatePasswordPolicy(password, 12); err != nil {
		return err
	}

	current, hasAdmin, err := stores.LocalAdmins.Get()
	if err != nil {
		return err
	}
	username = strings.TrimSpace(username)
	if username == "" {
		if hasAdmin {
			username = current.Username
		} else {
			return fmt.Errorf("auth-reset-admin-username is required when no admin account exists")
		}
	}

	hash, err := store.GeneratePasswordHash(password)
	if err != nil {
		return err
	}
	existing, found, err := stores.LocalAdmins.GetByUsername(username)
	if err != nil {
		return err
	}
	if found {
		if err := stores.LocalAdmins.SetPassword(existing.Username, hash); err != nil {
			return err
		}
	} else {
		userCount, err := stores.LocalAdmins.CountUsers()
		if err != nil {
			return err
		}
		if userCount > 0 {
			return fmt.Errorf("user %q not found", username)
		}
		if _, err := stores.LocalAdmins.UpsertUser(username, hash, "admin"); err != nil {
			return err
		}
	}
	if hasAdmin {
		_ = stores.AuthSessions.DeleteByUsername(current.Username)
	}
	_ = stores.AuthSessions.DeleteByUsername(username)
	return nil
}

func parseAuthMode(raw string) (api.AuthMode, error) {
	key := strings.ToLower(strings.TrimSpace(raw))
	switch key {
	case "off":
		return api.AuthModeOff, nil
	case "native":
		return api.AuthModeNative, nil
	case "sso":
		return api.AuthModeSSO, fmt.Errorf("auth mode %q is not available in this distribution", key)
	default:
		return "", fmt.Errorf("invalid auth mode %q (expected off, native, sso)", strings.TrimSpace(raw))
	}
}

func newEventBus(logger *log.Logger, backend, natsURL, subjectPrefix string) (eventbus.Bus, func()) {
	mode := strings.ToLower(strings.TrimSpace(backend))
	switch mode {
	case "", "memory":
		if logger != nil {
			logger.Printf("event bus backend=%s", "memory")
		}
		return eventbus.NewMemoryBus(8192), nil
	case "nats":
		bus, err := eventbus.NewNATSBus(natsURL, subjectPrefix, 8192, logger)
		if err != nil {
			if logger != nil {
				logger.Fatalf("failed to initialize nats event bus: %v", err)
			}
		}
		return bus, func() { _ = bus.Close() }
	default:
		if logger != nil {
			logger.Fatalf("unsupported event bus backend %q; expected memory or nats", backend)
		}
		return eventbus.NewMemoryBus(8192), nil
	}
}
