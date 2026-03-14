package main

import (
	"context"
	"database/sql"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/AnonJon/orloj/api"
	"github.com/AnonJon/orloj/controllers"
	"github.com/AnonJon/orloj/eventbus"
	"github.com/AnonJon/orloj/runtime"
	"github.com/AnonJon/orloj/store"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	addr := flag.String("addr", ":8080", "server listen address")
	reconcile := flag.Duration("reconcile-interval", 2*time.Second, "agent reconcile interval")
	runTaskWorker := flag.Bool("run-task-worker", false, "run embedded task worker in orlojd process")
	taskWorkerID := flag.String("task-worker-id", "embedded-worker", "worker id for embedded task worker")
	taskLeaseDuration := flag.Duration("task-lease-duration", 30*time.Second, "task lease duration for embedded worker")
	taskHeartbeatInterval := flag.Duration("task-heartbeat-interval", 10*time.Second, "task lease heartbeat interval for embedded worker")
	taskExecutionMode := flag.String("task-execution-mode", envOrDefault("ORLOJ_TASK_EXECUTION_MODE", "sequential"), "task execution mode: sequential|message-driven")
	modelGatewayProvider := flag.String("model-gateway-provider", envOrDefault("ORLOJ_MODEL_GATEWAY_PROVIDER", "mock"), "task model gateway provider: mock|openai|anthropic|azure-openai|ollama")
	modelGatewayAPIKey := flag.String("model-gateway-api-key", envOrDefault("ORLOJ_MODEL_GATEWAY_API_KEY", ""), "API key used by task model gateway provider")
	modelGatewayBaseURL := flag.String("model-gateway-base-url", envOrDefault("ORLOJ_MODEL_GATEWAY_BASE_URL", ""), "base URL used by task model gateway provider (provider defaults applied when empty)")
	modelGatewayTimeout := flag.Duration("model-gateway-timeout", envDurationOrDefault("ORLOJ_MODEL_GATEWAY_TIMEOUT", 30*time.Second), "HTTP timeout for task model gateway requests")
	modelGatewayDefaultModel := flag.String("model-gateway-default-model", envOrDefault("ORLOJ_MODEL_GATEWAY_DEFAULT_MODEL", ""), "default model used when agent spec.model is empty (provider defaults applied when empty)")
	modelSecretEnvPrefix := flag.String("model-secret-env-prefix", envOrDefault("ORLOJ_MODEL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve ModelEndpoint.spec.auth.secretRef")
	toolIsolationBackend := flag.String("tool-isolation-backend", envOrDefault("ORLOJ_TOOL_ISOLATION_BACKEND", "none"), "isolated tool executor backend: none|container|wasm")
	toolContainerRuntime := flag.String("tool-container-runtime", envOrDefault("ORLOJ_TOOL_CONTAINER_RUNTIME", "docker"), "container runtime binary for isolated tool execution")
	toolContainerImage := flag.String("tool-container-image", envOrDefault("ORLOJ_TOOL_CONTAINER_IMAGE", "curlimages/curl:8.8.0"), "container image used by isolated tool execution")
	toolContainerNetwork := flag.String("tool-container-network", envOrDefault("ORLOJ_TOOL_CONTAINER_NETWORK", "none"), "container network mode for isolated tools")
	toolContainerMemory := flag.String("tool-container-memory", envOrDefault("ORLOJ_TOOL_CONTAINER_MEMORY", "128m"), "container memory limit for isolated tools")
	toolContainerCPUs := flag.String("tool-container-cpus", envOrDefault("ORLOJ_TOOL_CONTAINER_CPUS", "0.50"), "container CPU limit for isolated tools")
	toolContainerPidsLimit := flag.Int("tool-container-pids-limit", 64, "container pids limit for isolated tools")
	toolContainerUser := flag.String("tool-container-user", envOrDefault("ORLOJ_TOOL_CONTAINER_USER", "65532:65532"), "container user for isolated tools")
	toolSecretEnvPrefix := flag.String("tool-secret-env-prefix", envOrDefault("ORLOJ_TOOL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve Tool.spec.auth.secretRef")
	toolWASMModule := flag.String("tool-wasm-module", envOrDefault("ORLOJ_TOOL_WASM_MODULE", ""), "wasm module path or identifier for wasm tool isolation runtime")
	toolWASMEntrypoint := flag.String("tool-wasm-entrypoint", envOrDefault("ORLOJ_TOOL_WASM_ENTRYPOINT", "run"), "wasm entrypoint function for wasm tool isolation runtime")
	toolWASMRuntimeBinary := flag.String("tool-wasm-runtime-binary", envOrDefault("ORLOJ_TOOL_WASM_RUNTIME_BINARY", "wasmtime"), "wasm runtime binary used by command-backed wasm executor")
	toolWASMRuntimeArgs := flag.String("tool-wasm-runtime-args", envOrDefault("ORLOJ_TOOL_WASM_RUNTIME_ARGS", ""), "comma-separated extra args passed to wasm runtime binary")
	toolWASMMemoryBytes := flag.Int64("tool-wasm-memory-bytes", envInt64OrDefault("ORLOJ_TOOL_WASM_MEMORY_BYTES", 64*1024*1024), "max wasm runtime memory bytes for tool isolation runtime")
	toolWASMFuel := flag.Uint64("tool-wasm-fuel", envUint64OrDefault("ORLOJ_TOOL_WASM_FUEL", 0), "optional wasm execution fuel limit (0 disables fuel limiting)")
	toolWASMWASI := flag.Bool("tool-wasm-wasi", envBoolOrDefault("ORLOJ_TOOL_WASM_WASI", true), "enable WASI host functions for wasm tool isolation runtime")
	eventBusBackend := flag.String("event-bus-backend", envOrDefault("ORLOJ_EVENT_BUS_BACKEND", "memory"), "event bus backend: memory|nats")
	natsURL := flag.String("nats-url", envOrDefault("ORLOJ_NATS_URL", "nats://127.0.0.1:4222"), "NATS server URL used when --event-bus-backend=nats")
	natsSubjectPrefix := flag.String("nats-subject-prefix", envOrDefault("ORLOJ_NATS_SUBJECT_PREFIX", "orloj.controlplane"), "NATS subject prefix used when --event-bus-backend=nats")
	agentMessageBusBackend := flag.String("agent-message-bus-backend", envOrDefault("ORLOJ_AGENT_MESSAGE_BUS_BACKEND", "none"), "runtime agent message bus backend: none|memory|nats-jetstream")
	agentMessageNATSURL := flag.String("agent-message-nats-url", envOrDefault("ORLOJ_AGENT_MESSAGE_NATS_URL", envOrDefault("ORLOJ_NATS_URL", "nats://127.0.0.1:4222")), "NATS server URL used when --agent-message-bus-backend=nats-jetstream")
	agentMessageSubjectPrefix := flag.String("agent-message-subject-prefix", envOrDefault("ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX", "orloj.agentmsg"), "runtime agent message subject prefix")
	agentMessageStreamName := flag.String("agent-message-stream-name", envOrDefault("ORLOJ_AGENT_MESSAGE_STREAM", "ORLOJ_AGENT_MESSAGES"), "JetStream stream name for runtime agent messages")
	agentMessageHistoryMax := flag.Int("agent-message-history-max", 2048, "in-memory runtime agent message history capacity")
	agentMessageDedupeWindow := flag.Duration("agent-message-dedupe-window", 2*time.Minute, "in-memory runtime agent message dedupe window")
	storageBackend := flag.String("storage-backend", "memory", "state backend: memory|postgres")
	postgresDSN := flag.String("postgres-dsn", os.Getenv("ORLOJ_POSTGRES_DSN"), "postgres DSN (required when --storage-backend=postgres)")
	sqlDriver := flag.String("sql-driver", "pgx", "database/sql driver name used for --storage-backend=postgres")
	postgresMaxOpenConns := flag.Int("postgres-max-open-conns", 20, "max open postgres connections")
	postgresMaxIdleConns := flag.Int("postgres-max-idle-conns", 5, "max idle postgres connections")
	postgresConnMaxLifetime := flag.Duration("postgres-conn-max-lifetime", 30*time.Minute, "max lifetime of postgres connections")
	flag.Parse()

	logger := log.New(os.Stdout, "orlojd ", log.LstdFlags|log.Lmicroseconds)
	backend := strings.ToLower(strings.TrimSpace(*storageBackend))

	var db *sql.DB
	var err error
	agentStore := store.NewAgentStore()
	agentSystemStore := store.NewAgentSystemStore()
	modelEndpointStore := store.NewModelEndpointStore()
	toolStore := store.NewToolStore()
	secretStore := store.NewSecretStore()
	memoryStore := store.NewMemoryStore()
	policyStore := store.NewAgentPolicyStore()
	roleStore := store.NewAgentRoleStore()
	toolPermStore := store.NewToolPermissionStore()
	taskStore := store.NewTaskStore()
	taskScheduleStore := store.NewTaskScheduleStore()
	taskWebhookStore := store.NewTaskWebhookStore()
	webhookDedupeStore := store.NewWebhookDedupeStore()
	workerStore := store.NewWorkerStore()

	switch backend {
	case "memory":
		logger.Printf("using storage backend=%s", backend)
	case "postgres":
		dsn := strings.TrimSpace(*postgresDSN)
		if dsn == "" {
			logger.Fatal("postgres backend selected but --postgres-dsn is empty (or ORLOJ_POSTGRES_DSN is unset)")
		}
		db, err = sql.Open(*sqlDriver, dsn)
		if err != nil {
			logger.Fatalf("failed to open postgres with sql driver %q: %v (ensure a matching database/sql driver is linked)", *sqlDriver, err)
		}
		db.SetMaxOpenConns(*postgresMaxOpenConns)
		db.SetMaxIdleConns(*postgresMaxIdleConns)
		db.SetConnMaxLifetime(*postgresConnMaxLifetime)

		pingCtx, pingCancel := context.WithTimeout(context.Background(), 5*time.Second)
		if err := db.PingContext(pingCtx); err != nil {
			pingCancel()
			_ = db.Close()
			logger.Fatalf("failed to connect to postgres: %v", err)
		}
		pingCancel()

		if err := store.EnsurePostgresSchema(db); err != nil {
			_ = db.Close()
			logger.Fatalf("failed to ensure postgres schema: %v", err)
		}

		agentStore = store.NewAgentStoreWithDB(db)
		agentSystemStore = store.NewAgentSystemStoreWithDB(db)
		modelEndpointStore = store.NewModelEndpointStoreWithDB(db)
		toolStore = store.NewToolStoreWithDB(db)
		secretStore = store.NewSecretStoreWithDB(db)
		memoryStore = store.NewMemoryStoreWithDB(db)
		policyStore = store.NewAgentPolicyStoreWithDB(db)
		roleStore = store.NewAgentRoleStoreWithDB(db)
		toolPermStore = store.NewToolPermissionStoreWithDB(db)
		taskStore = store.NewTaskStoreWithDB(db)
		taskScheduleStore = store.NewTaskScheduleStoreWithDB(db)
		taskWebhookStore = store.NewTaskWebhookStoreWithDB(db)
		webhookDedupeStore = store.NewWebhookDedupeStoreWithDB(db)
		workerStore = store.NewWorkerStoreWithDB(db)
		logger.Printf("using storage backend=%s driver=%s", backend, *sqlDriver)
	default:
		logger.Fatalf("unsupported storage backend %q; expected memory or postgres", *storageBackend)
	}
	if db != nil {
		defer db.Close()
	}

	resolvedModelGatewayAPIKey := resolveModelGatewayAPIKey(*modelGatewayProvider, *modelGatewayAPIKey)
	baseModelGateway, err := newModelGateway(
		*modelGatewayProvider,
		resolvedModelGatewayAPIKey,
		*modelGatewayBaseURL,
		*modelGatewayDefaultModel,
		*modelGatewayTimeout,
	)
	if err != nil {
		logger.Fatalf("failed to configure model gateway: %v", err)
	}
	modelGateway := agentruntime.NewModelRouter(agentruntime.ModelRouterConfig{
		Fallback:        baseModelGateway,
		Endpoints:       modelEndpointStore,
		Secrets:         secretStore,
		FallbackAPIKey:  resolvedModelGatewayAPIKey,
		SecretEnvPrefix: *modelSecretEnvPrefix,
	})
	taskExecutor := agentruntime.NewTaskExecutorWithRuntime(logger, nil, modelGateway, nil)
	extensions := agentruntime.DefaultExtensions()
	logger.Printf(
		"task model gateway provider=%s timeout=%s base_url=%s default_model=%s model_secret_env_prefix=%s",
		strings.ToLower(strings.TrimSpace(*modelGatewayProvider)),
		modelGatewayTimeout.String(),
		strings.TrimSpace(*modelGatewayBaseURL),
		strings.TrimSpace(*modelGatewayDefaultModel),
		strings.TrimSpace(*modelSecretEnvPrefix),
	)

	runtime := agentruntime.NewManager(logger)
	agentController := controllers.NewAgentController(agentStore, runtime, logger, *reconcile)
	agentSystemController := controllers.NewAgentSystemController(agentSystemStore, logger, *reconcile)
	modelEndpointController := controllers.NewModelEndpointController(modelEndpointStore, logger, 5*time.Second)
	toolController := controllers.NewToolController(toolStore, logger, 5*time.Second)
	memoryController := controllers.NewMemoryController(memoryStore, logger, 5*time.Second)
	policyController := controllers.NewPolicyController(policyStore, logger, 5*time.Second)
	taskController := controllers.NewTaskController(
		taskStore,
		agentSystemStore,
		agentStore,
		toolStore,
		memoryStore,
		policyStore,
		workerStore,
		logger,
		*reconcile,
	)
	taskSchedulerController := controllers.NewTaskSchedulerController(taskStore, workerStore, logger, *reconcile, 20*time.Second)
	taskScheduleController := controllers.NewTaskScheduleController(taskScheduleStore, taskStore, logger, *reconcile)
	workerController := controllers.NewWorkerController(workerStore, logger, *reconcile, 20*time.Second)
	taskController.ConfigureWorker(*taskWorkerID, *taskLeaseDuration, *taskHeartbeatInterval)
	taskController.SetExecutionMode(*taskExecutionMode)
	taskController.SetGovernanceStores(roleStore, toolPermStore)
	taskController.SetModelEndpointStore(modelEndpointStore)
	taskController.SetExecutor(taskExecutor)
	taskController.SetExtensions(extensions)
	isolatedToolRuntime, err := newIsolatedToolRuntime(
		logger,
		*toolIsolationBackend,
		*toolContainerRuntime,
		*toolContainerImage,
		*toolContainerNetwork,
		*toolContainerMemory,
		*toolContainerCPUs,
		*toolContainerPidsLimit,
		*toolContainerUser,
		*toolSecretEnvPrefix,
		*toolWASMModule,
		*toolWASMEntrypoint,
		*toolWASMRuntimeBinary,
		parseCSV(*toolWASMRuntimeArgs),
		*toolWASMMemoryBytes,
		*toolWASMFuel,
		*toolWASMWASI,
		secretStore,
	)
	if err != nil {
		logger.Fatalf("failed to configure isolated tool runtime: %v", err)
	}
	taskController.SetIsolatedToolRuntime(isolatedToolRuntime)

	server := api.NewServerWithOptions(api.Stores{
		Agents:        agentStore,
		AgentSystems:  agentSystemStore,
		ModelEPs:      modelEndpointStore,
		Tools:         toolStore,
		Secrets:       secretStore,
		Memories:      memoryStore,
		Policies:      policyStore,
		AgentRoles:    roleStore,
		ToolPerms:     toolPermStore,
		Tasks:         taskStore,
		TaskSchedules: taskScheduleStore,
		TaskWebhooks:  taskWebhookStore,
		WebhookDedupe: webhookDedupeStore,
		Workers:       workerStore,
	}, runtime, logger, api.ServerOptions{
		Extensions: extensions,
	})
	bus, closeBus := newEventBus(logger, *eventBusBackend, *natsURL, *natsSubjectPrefix)
	if closeBus != nil {
		defer closeBus()
	}
	agentMessageBus, closeAgentMessageBus := newAgentMessageBus(
		logger,
		*agentMessageBusBackend,
		*agentMessageNATSURL,
		*agentMessageSubjectPrefix,
		*agentMessageStreamName,
		*agentMessageHistoryMax,
		*agentMessageDedupeWindow,
	)
	if closeAgentMessageBus != nil {
		defer closeAgentMessageBus()
	}
	server.SetEventBus(bus)
	taskController.SetEventBus(bus)
	taskController.SetAgentMessageBus(agentMessageBus)
	taskSchedulerController.SetEventBus(bus)
	taskScheduleController.SetEventBus(bus)
	workerController.SetEventBus(bus)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if strings.EqualFold(strings.TrimSpace(*taskExecutionMode), "message-driven") {
		logger.Printf("agent runtime reconciliation disabled in message-driven mode")
	} else {
		go agentController.Start(ctx)
	}
	go agentSystemController.Start(ctx)
	go modelEndpointController.Start(ctx)
	go toolController.Start(ctx)
	go memoryController.Start(ctx)
	go policyController.Start(ctx)
	go taskSchedulerController.Start(ctx)
	go taskScheduleController.Start(ctx)
	go workerController.Start(ctx)
	if *runTaskWorker {
		go taskController.Start(ctx)
		if strings.EqualFold(strings.TrimSpace(*taskExecutionMode), "message-driven") {
			if agentMessageBus == nil {
				logger.Printf("embedded runtime inbox consumer disabled: agent message bus backend is none")
			} else {
				consumer := agentruntime.NewAgentMessageConsumerManager(
					agentMessageBus,
					agentStore,
					agentSystemStore,
					taskStore,
					logger,
					agentruntime.AgentMessageConsumerOptions{
						WorkerID:            *taskWorkerID,
						RefreshEvery:        10 * time.Second,
						DedupeWindow:        10 * time.Minute,
						LeaseExtendDuration: *taskLeaseDuration,
						Executor:            taskExecutor,
						Tools:               toolStore,
						Roles:               roleStore,
						ToolPermissions:     toolPermStore,
						IsolatedToolRuntime: isolatedToolRuntime,
						Extensions:          extensions,
					},
				)
				go consumer.Start(ctx)
				logger.Printf("embedded runtime inbox consumers enabled refresh=%s dedupe=%s", (10 * time.Second).String(), (10 * time.Minute).String())
			}
		}
		logger.Printf("embedded task worker enabled id=%s lease=%s", *taskWorkerID, taskLeaseDuration.String())
	}

	httpServer := &http.Server{
		Addr:    *addr,
		Handler: server.Handler(),
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

func newAgentMessageBus(
	logger *log.Logger,
	backend string,
	natsURL string,
	subjectPrefix string,
	streamName string,
	historyMax int,
	dedupeWindow time.Duration,
) (agentruntime.AgentMessageBus, func()) {
	mode := strings.ToLower(strings.TrimSpace(backend))
	switch mode {
	case "", "none":
		if logger != nil {
			logger.Printf("runtime agent message bus backend=%s", "none")
		}
		return nil, nil
	case "memory":
		bus := agentruntime.NewMemoryAgentMessageBus(subjectPrefix, historyMax, dedupeWindow)
		if logger != nil {
			logger.Printf("runtime agent message bus backend=%s prefix=%s history_max=%d dedupe_window=%s", "memory", subjectPrefix, historyMax, dedupeWindow)
		}
		return bus, func() { _ = bus.Close() }
	case "nats", "nats-jetstream":
		bus, err := agentruntime.NewNATSJetStreamAgentMessageBus(natsURL, subjectPrefix, streamName, logger)
		if err != nil && logger != nil {
			logger.Fatalf("failed to initialize runtime agent message bus: %v", err)
		}
		return bus, func() { _ = bus.Close() }
	default:
		if logger != nil {
			logger.Fatalf("unsupported runtime agent message bus backend %q; expected none, memory, or nats-jetstream", backend)
		}
		return nil, nil
	}
}

func newIsolatedToolRuntime(
	logger *log.Logger,
	backend string,
	runtimeBinary string,
	image string,
	network string,
	memory string,
	cpus string,
	pidsLimit int,
	user string,
	secretEnvPrefix string,
	wasmModule string,
	wasmEntrypoint string,
	wasmRuntimeBinary string,
	wasmRuntimeArgs []string,
	wasmMemoryBytes int64,
	wasmFuel uint64,
	wasmWASI bool,
	secrets agentruntime.SecretResourceLookup,
) (agentruntime.ToolRuntime, error) {
	mode := strings.ToLower(strings.TrimSpace(backend))
	if mode == "" {
		mode = "none"
	}
	containerCfg := agentruntime.DefaultContainerToolRuntimeConfig()
	containerCfg.RuntimeBinary = strings.TrimSpace(runtimeBinary)
	containerCfg.Image = strings.TrimSpace(image)
	containerCfg.Network = strings.TrimSpace(network)
	containerCfg.Memory = strings.TrimSpace(memory)
	containerCfg.CPUs = strings.TrimSpace(cpus)
	containerCfg.PidsLimit = pidsLimit
	containerCfg.User = strings.TrimSpace(user)

	storeResolver := agentruntime.NewStoreSecretResolver(secrets, "value")
	envResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(secretEnvPrefix))
	resolver := agentruntime.NewChainSecretResolver(storeResolver, envResolver)

	wasmCfg := agentruntime.WASMToolRuntimeConfig{
		ModulePath:     strings.TrimSpace(wasmModule),
		Entrypoint:     strings.TrimSpace(wasmEntrypoint),
		RuntimeBinary:  strings.TrimSpace(wasmRuntimeBinary),
		RuntimeArgs:    append([]string(nil), wasmRuntimeArgs...),
		MaxMemoryBytes: wasmMemoryBytes,
		Fuel:           wasmFuel,
		EnableWASI:     wasmWASI,
	}
	wasmFactory := agentruntime.NewWASMCommandExecutorFactory()
	runtime, err := agentruntime.BuildToolIsolationRuntime(agentruntime.ToolIsolationBackendOptions{
		Mode:                mode,
		ContainerConfig:     containerCfg,
		SecretResolver:      resolver,
		WASMConfig:          wasmCfg,
		WASMExecutorFactory: wasmFactory,
	})
	if err != nil {
		return nil, err
	}
	if logger != nil {
		switch mode {
		case "none":
			logger.Printf("tool isolation backend=%s", "none")
		case "container":
			logger.Printf(
				"tool isolation backend=%s runtime=%s image=%s network=%s",
				"container",
				containerCfg.RuntimeBinary,
				containerCfg.Image,
				containerCfg.Network,
			)
		case "wasm":
			logger.Printf(
				"tool isolation backend=%s module=%s entrypoint=%s runtime=%s runtime_args=%d wasi=%t memory_bytes=%d fuel=%d",
				"wasm",
				wasmCfg.ModulePath,
				wasmCfg.Entrypoint,
				wasmCfg.RuntimeBinary,
				len(wasmCfg.RuntimeArgs),
				wasmCfg.EnableWASI,
				wasmCfg.MaxMemoryBytes,
				wasmCfg.Fuel,
			)
		}
	}
	return runtime, nil
}

func newModelGateway(
	provider string,
	apiKey string,
	baseURL string,
	defaultModel string,
	timeout time.Duration,
) (agentruntime.ModelGateway, error) {
	cfg := agentruntime.DefaultModelGatewayConfig()
	cfg.Provider = strings.TrimSpace(provider)
	cfg.APIKey = strings.TrimSpace(apiKey)
	cfg.BaseURL = strings.TrimSpace(baseURL)
	cfg.DefaultModel = strings.TrimSpace(defaultModel)
	cfg.Timeout = timeout
	return agentruntime.NewModelGatewayFromConfig(cfg)
}

func resolveModelGatewayAPIKey(provider string, explicit string) string {
	key := strings.TrimSpace(explicit)
	if key != "" {
		return key
	}
	switch strings.ToLower(strings.TrimSpace(provider)) {
	case "openai", "openai-compatible", "openai_compatible":
		return envOrDefault("OPENAI_API_KEY", "")
	case "anthropic":
		return envOrDefault("ANTHROPIC_API_KEY", "")
	case "azure-openai", "azure_openai", "azure":
		key := envOrDefault("AZURE_OPENAI_API_KEY", "")
		if key != "" {
			return key
		}
		return envOrDefault("OPENAI_API_KEY", "")
	default:
		return ""
	}
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func envDurationOrDefault(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envBoolOrDefault(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func envInt64OrDefault(key string, fallback int64) int64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func envUint64OrDefault(key string, fallback uint64) uint64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseUint(value, 10, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func parseCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		out = append(out, part)
	}
	return out
}
