package main

import (
	"context"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/OrlojHQ/orloj/controllers"
	"github.com/OrlojHQ/orloj/internal/version"
	"github.com/OrlojHQ/orloj/resources"
	agentruntime "github.com/OrlojHQ/orloj/runtime"
	"github.com/OrlojHQ/orloj/runtime/a2a"
	"github.com/OrlojHQ/orloj/startup"
	"github.com/OrlojHQ/orloj/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	env := startup.EnvOrDefault
	envBool := startup.EnvBoolOrDefault
	envDuration := startup.EnvDurationOrDefault
	envInt := startup.EnvIntOrDefault
	envInt64 := startup.EnvInt64OrDefault
	envUint64 := startup.EnvUint64OrDefault

	showVersion := flag.Bool("version", false, "print version and exit")
	logLevelRaw := flag.String("log-level", env("ORLOJ_LOG_LEVEL", "info"), "minimum log level: debug|info|warn|error (env: ORLOJ_LOG_LEVEL)")
	debugLogs := flag.Bool("debug", false, "enable debug logging (equivalent to --log-level=debug)")
	workerID := flag.String("worker-id", "worker-1", "task worker identity")
	healthzAddr := flag.String("healthz-addr", env("ORLOJ_WORKER_HEALTHZ_ADDR", ""), "optional address for the /healthz liveness probe endpoint (e.g. :8081); empty disables it")
	reconcile := flag.Duration("reconcile-interval", 1*time.Second, "claim/reconcile interval")
	leaseDuration := flag.Duration("lease-duration", 30*time.Second, "task lease duration")
	heartbeatInterval := flag.Duration("heartbeat-interval", 10*time.Second, "task lease heartbeat interval")
	region := flag.String("region", "default", "worker region")
	gpu := flag.Bool("gpu", false, "worker has GPU capability")
	supportedModels := flag.String("supported-models", "", "comma-separated supported model ids")
	maxConcurrentTasks := flag.Int("max-concurrent-tasks", 1, "worker max concurrent task capacity")
	taskExecutionMode := flag.String("task-execution-mode", env("ORLOJ_TASK_EXECUTION_MODE", "sequential"), "task execution mode: sequential|message-driven")
	modelSecretEnvPrefix := flag.String("model-secret-env-prefix", env("ORLOJ_MODEL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve ModelEndpoint.spec.auth.secretRef")
	toolIsolationBackend := flag.String("tool-isolation-backend", env("ORLOJ_TOOL_ISOLATION_BACKEND", "none"), "isolated tool executor backend for container sandboxing: none|container")
	toolContainerRuntime := flag.String("tool-container-runtime", env("ORLOJ_TOOL_CONTAINER_RUNTIME", "docker"), "container runtime binary for isolated tool execution")
	toolContainerImage := flag.String("tool-container-image", env("ORLOJ_TOOL_CONTAINER_IMAGE", "curlimages/curl:8.8.0"), "container image used by isolated tool execution")
	toolContainerNetwork := flag.String("tool-container-network", env("ORLOJ_TOOL_CONTAINER_NETWORK", "none"), "container network mode for isolated tools")
	toolContainerMemory := flag.String("tool-container-memory", env("ORLOJ_TOOL_CONTAINER_MEMORY", "128m"), "container memory limit for isolated tools")
	toolContainerCPUs := flag.String("tool-container-cpus", env("ORLOJ_TOOL_CONTAINER_CPUS", "0.50"), "container CPU limit for isolated tools")
	toolContainerPidsLimit := flag.Int("tool-container-pids-limit", envInt("ORLOJ_TOOL_CONTAINER_PIDS_LIMIT", 64), "container pids limit for isolated tools")
	toolContainerUser := flag.String("tool-container-user", env("ORLOJ_TOOL_CONTAINER_USER", "65532:65532"), "container user for isolated tools")
	toolSecretEnvPrefix := flag.String("tool-secret-env-prefix", env("ORLOJ_TOOL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve Tool.spec.auth.secretRef")
	toolWASMModule := flag.String("tool-wasm-module", env("ORLOJ_TOOL_WASM_MODULE", ""), "default wasm module path (per-tool spec.wasm.module takes precedence)")
	toolWASMEntrypoint := flag.String("tool-wasm-entrypoint", env("ORLOJ_TOOL_WASM_ENTRYPOINT", "run"), "default wasm entrypoint function")
	toolWASMMemoryBytes := flag.Int64("tool-wasm-memory-bytes", envInt64("ORLOJ_TOOL_WASM_MEMORY_BYTES", 64*1024*1024), "default max wasm runtime memory bytes")
	toolWASMFuel := flag.Uint64("tool-wasm-fuel", envUint64("ORLOJ_TOOL_WASM_FUEL", 1000000), "default wasm execution fuel limit")
	toolWASMWASI := flag.Bool("tool-wasm-wasi", envBool("ORLOJ_TOOL_WASM_WASI", true), "default: enable WASI host functions for wasm tools")
	toolWASMCacheDir := flag.String("tool-wasm-cache-dir", env("ORLOJ_TOOL_WASM_CACHE_DIR", ""), "disk cache directory for remote WASM modules (default: ~/.orloj/wasm-cache)")
	cliToolAllowedCommands := flag.String("cli-tool-allowed-commands", env("ORLOJ_CLI_TOOL_ALLOWED_COMMANDS", ""), "comma-separated allowlist of commands for CLI tools (empty allows all)")
	cliToolMaxArgvLength := flag.Int("cli-tool-max-argv-length", envInt("ORLOJ_CLI_TOOL_MAX_ARGV_LENGTH", 4096), "max total argv byte length for CLI tool invocations")
	a2aAllowPrivateEndpoints := flag.Bool("a2a-allow-private-endpoints", envBool("ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS", false), "allow outbound A2A requests to private addresses (env: ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS)")
	a2aCardCacheTTL := flag.Duration("a2a-card-cache-ttl", envDuration("ORLOJ_A2A_CARD_CACHE_TTL", 5*time.Minute), "TTL for cached remote Agent Cards (env: ORLOJ_A2A_CARD_CACHE_TTL)")
	toolK8sEnabled := flag.Bool("tool-k8s-enabled", envBool("ORLOJ_TOOL_K8S_ENABLED", false), "enable kubernetes tool isolation runtime for isolation_mode=kubernetes tools")
	toolK8sNamespace := flag.String("tool-k8s-namespace", env("ORLOJ_TOOL_K8S_NAMESPACE", ""), "namespace for kubernetes tool isolation Jobs (default: pod namespace or 'default')")
	toolK8sServiceAccount := flag.String("tool-k8s-service-account", env("ORLOJ_TOOL_K8S_SERVICE_ACCOUNT", ""), "service account for kubernetes tool isolation Pods")
	toolK8sJobTTL := flag.Int("tool-k8s-job-ttl", envInt("ORLOJ_TOOL_K8S_JOB_TTL", 300), "TTL seconds after kubernetes tool Job finishes (cleanup)")
	toolK8sDefaultImage := flag.String("tool-k8s-default-image", env("ORLOJ_TOOL_K8S_DEFAULT_IMAGE", "curlimages/curl:8.8.0"), "fallback container image for kubernetes tool isolation")
	agentK8sEnabled := flag.Bool("agent-k8s-enabled", envBool("ORLOJ_AGENT_K8S_ENABLED", false), "enable kubernetes agent execution (run agents as ephemeral K8s Jobs)")
	agentK8sNamespace := flag.String("agent-k8s-namespace", env("ORLOJ_AGENT_K8S_NAMESPACE", ""), "namespace for kubernetes agent Jobs (default: pod namespace or 'default')")
	agentK8sServiceAccount := flag.String("agent-k8s-service-account", env("ORLOJ_AGENT_K8S_SERVICE_ACCOUNT", ""), "service account for kubernetes agent Pods")
	agentK8sImage := flag.String("agent-k8s-image", env("ORLOJ_AGENT_K8S_IMAGE", ""), "container image for agent Jobs (default: own image)")
	agentK8sJobTTL := flag.Int("agent-k8s-job-ttl", envInt("ORLOJ_AGENT_K8S_JOB_TTL", 600), "TTL seconds after agent Job finishes (cleanup)")
	agentK8sDefaultMemory := flag.String("agent-k8s-default-memory", env("ORLOJ_AGENT_K8S_DEFAULT_MEMORY", "512Mi"), "default memory limit for agent Pods")
	agentK8sDefaultCPU := flag.String("agent-k8s-default-cpu", env("ORLOJ_AGENT_K8S_DEFAULT_CPU", "500m"), "default CPU limit for agent Pods")
	agentMessageBusBackend := flag.String("agent-message-bus-backend", env("ORLOJ_AGENT_MESSAGE_BUS_BACKEND", "none"), "runtime agent message bus backend: none|memory|nats-jetstream")
	agentMessageNATSURL := flag.String("agent-message-nats-url", env("ORLOJ_AGENT_MESSAGE_NATS_URL", env("ORLOJ_NATS_URL", "nats://127.0.0.1:4222")), "NATS server URL used when --agent-message-bus-backend=nats-jetstream")
	agentMessageSubjectPrefix := flag.String("agent-message-subject-prefix", env("ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX", "orloj.agentmsg"), "runtime agent message subject prefix")
	agentMessageStreamName := flag.String("agent-message-stream-name", env("ORLOJ_AGENT_MESSAGE_STREAM", "ORLOJ_AGENT_MESSAGES"), "JetStream stream name for runtime agent messages")
	agentMessageHistoryMax := flag.Int("agent-message-history-max", 2048, "in-memory runtime agent message history capacity")
	agentMessageDedupeWindow := flag.Duration("agent-message-dedupe-window", 2*time.Minute, "in-memory runtime agent message dedupe window")
	agentMessageConsume := flag.Bool("agent-message-consume", envBool("ORLOJ_AGENT_MESSAGE_CONSUME", false), "enable runtime agent inbox consumers in worker")
	agentMessageConsumerNamespace := flag.String("agent-message-consumer-namespace", env("ORLOJ_AGENT_MESSAGE_CONSUMER_NAMESPACE", ""), "optional namespace filter for runtime inbox consumers")
	agentMessageConsumerRefresh := flag.Duration("agent-message-consumer-refresh", 10*time.Second, "refresh interval for reconciling runtime inbox consumers")
	agentMessageConsumerDedupe := flag.Duration("agent-message-consumer-dedupe-window", 10*time.Minute, "dedupe window for runtime inbox message processing")
	secretEncryptionKeyRaw := flag.String("secret-encryption-key", env("ORLOJ_SECRET_ENCRYPTION_KEY", ""), "256-bit AES key (hex or base64) for encrypting Secret resource data at rest")
	singleAgent := flag.Bool("single-agent", false, "run a single agent from a task and exit (K8s Job mode)")
	singleAgentTaskID := flag.String("task-id", "", "task namespace/name for --single-agent mode")
	singleAgentName := flag.String("agent-name", "", "agent to execute in --single-agent mode")
	singleAgentAttempt := flag.Int("attempt", 0, "task attempt number for --single-agent mode")
	singleAgentMessageID := flag.String("message-id", "", "message ID for --single-agent mode (message-driven)")
	storageBackend := flag.String("storage-backend", "postgres", "state backend: postgres|memory")
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

	resolvedLogLevel := *logLevelRaw
	if *debugLogs {
		resolvedLogLevel = "debug"
	}
	parsedLogLevel, logLevelErr := telemetry.ResolveLogLevel(*logLevelRaw, *debugLogs)
	if logLevelErr != nil {
		fmt.Fprintf(os.Stderr, "%v\n", logLevelErr)
		os.Exit(2)
	}

	slogger := telemetry.NewLoggerWithLevel("orlojworker", parsedLogLevel)
	logger := telemetry.NewBridgeLogger(slogger)
	debugLogger := telemetry.NewDebugBridgeLogger(slogger)
	fatalLogger := telemetry.NewErrorBridgeLogger(slogger)
	resolvedLogLevelLabel := strings.ToLower(strings.TrimSpace(resolvedLogLevel))
	if resolvedLogLevelLabel == "" {
		resolvedLogLevelLabel = "info"
	}

	debugLogger.Printf(
		"startup config worker_id=%s log_level=%s healthz_enabled=%t region=%s gpu=%t supported_models=%d max_concurrent_tasks=%d storage_backend=%s postgres_dsn_configured=%t task_execution_mode=%s agent_message_consume=%t agent_message_bus_backend=%s agent_message_consumer_namespace=%q tool_isolation_backend=%s tool_container_runtime=%s tool_container_network=%s wasm_module_configured=%t",
		*workerID,
		resolvedLogLevelLabel,
		strings.TrimSpace(*healthzAddr) != "",
		*region,
		*gpu,
		len(startup.ParseCSV(*supportedModels)),
		*maxConcurrentTasks,
		*storageBackend,
		strings.TrimSpace(*postgresDSN) != "",
		*taskExecutionMode,
		*agentMessageConsume,
		*agentMessageBusBackend,
		strings.TrimSpace(*agentMessageConsumerNamespace),
		*toolIsolationBackend,
		*toolContainerRuntime,
		*toolContainerNetwork,
		strings.TrimSpace(*toolWASMModule) != "",
	)

	otelShutdown, otelErr := telemetry.Init(context.Background(), telemetry.Config{
		ServiceName: "orlojworker",
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
		fatalLogger.Fatalf("%v", err)
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
		IncludeScheduleStores: false,
	}, logger)
	if err != nil {
		fatalLogger.Fatalf("%v", err)
	}
	defer stores.Close()

	// --single-agent mode: execute one agent from a task, write result, exit.
	if *singleAgent {
		if strings.ToLower(strings.TrimSpace(*storageBackend)) != "postgres" {
			fatalLogger.Fatalf("--single-agent mode requires --storage-backend=postgres")
		}

		ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer cancel()

		toolStoreResolver := agentruntime.NewStoreSecretResolver(stores.Secrets, "value")
		toolEnvResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(*toolSecretEnvPrefix))
		toolSecretResolver := agentruntime.NewChainSecretResolver(toolStoreResolver, toolEnvResolver)

		isolatedToolRuntime, isoErr := startup.NewIsolatedToolRuntime(startup.IsolatedToolRuntimeConfig{
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
		if isoErr != nil {
			fatalLogger.Fatalf("failed to configure isolated tool runtime: %v", isoErr)
		}
		wasmToolRuntime, closeWasm, wasmErr := startup.NewWASMToolRuntime(startup.IsolatedToolRuntimeConfig{
			WASMModule:      *toolWASMModule,
			WASMEntrypoint:  *toolWASMEntrypoint,
			WASMMemoryBytes: *toolWASMMemoryBytes,
			WASMFuel:        *toolWASMFuel,
			WASMWASI:        *toolWASMWASI,
			WASMCacheDir:    *toolWASMCacheDir,
			SecretEnvPrefix: *toolSecretEnvPrefix,
			Secrets:         stores.Secrets,
		}, logger)
		if wasmErr != nil {
			fatalLogger.Fatalf("failed to configure wasm tool runtime: %v", wasmErr)
		}
		if closeWasm != nil {
			defer closeWasm()
		}
		mcpSessionMgr := startup.NewMcpSessionManager(startup.McpRuntimeConfig{
			ContainerRuntime: *toolContainerRuntime,
			ContainerNetwork: "bridge",
			ContainerMemory:  *toolContainerMemory,
			ContainerCPUs:    *toolContainerCPUs,
			ContainerPids:    *toolContainerPidsLimit,
			SecretEnvPrefix:  "ORLOJ_SECRET_",
			Secrets:          stores.Secrets,
		})
		defer mcpSessionMgr.Close()

		var k8sTools agentruntime.ToolRuntime
		if *toolK8sEnabled {
			rt, k8sErr := startup.NewKubernetesToolRuntime(startup.KubernetesToolRuntimeConfig{
				Namespace:       *toolK8sNamespace,
				ServiceAccount:  *toolK8sServiceAccount,
				JobTTLSeconds:   int32(*toolK8sJobTTL),
				DefaultImage:    *toolK8sDefaultImage,
				SecretEnvPrefix: *toolSecretEnvPrefix,
				Secrets:         stores.Secrets,
			}, logger)
			if k8sErr != nil {
				logger.Printf("kubernetes tool runtime unavailable in agent pod: %v", k8sErr)
			} else {
				k8sTools = rt
			}
		}

		saCfg := startup.SingleAgentConfig{
			TaskID:               *singleAgentTaskID,
			AgentName:            *singleAgentName,
			Attempt:              *singleAgentAttempt,
			MessageID:            *singleAgentMessageID,
			ModelSecretEnvPrefix: *modelSecretEnvPrefix,
			ToolSecretEnvPrefix:  *toolSecretEnvPrefix,
			IsolatedToolRuntime:  isolatedToolRuntime,
			WasmToolRuntime:      wasmToolRuntime,
			McpSessionManager:    mcpSessionMgr,
			CliToolConfig: agentruntime.CLIToolRuntimeConfig{
				AllowedCommands: startup.ParseCSV(*cliToolAllowedCommands),
				MaxArgvLength:   *cliToolMaxArgvLength,
			},
			SecretResolver:  toolSecretResolver,
			KubernetesTools: k8sTools,
			A2ATools: a2a.NewToolRuntime(a2a.NewClient(a2a.ClientConfig{
				AllowPrivate: *a2aAllowPrivateEndpoints,
				CardCacheTTL: *a2aCardCacheTTL,
			}), nil, toolSecretResolver),
		}

		if runErr := startup.RunSingleAgent(ctx, stores, saCfg, logger); runErr != nil {
			fatalLogger.Fatalf("single-agent failed: %v", runErr)
		}
		os.Exit(0)
	}

	modelGateway := agentruntime.NewModelRouter(agentruntime.ModelRouterConfig{
		Endpoints:       stores.ModelEPs,
		Secrets:         stores.Secrets,
		SecretEnvPrefix: *modelSecretEnvPrefix,
	})
	taskExecutor := agentruntime.NewTaskExecutorWithRuntime(logger, nil, modelGateway, nil)
	extensions := agentruntime.DefaultExtensions()
	logger.Printf("model routing: endpoint-driven secret_env_prefix=%s", *modelSecretEnvPrefix)

	taskController := controllers.NewTaskController(
		stores.Tasks, stores.AgentSystems, stores.Agents, stores.Tools,
		stores.Memories, stores.Policies, stores.Workers, logger, *reconcile,
	)
	taskController.SetDebugLogger(debugLogger)
	taskController.ConfigureWorker(*workerID, *leaseDuration, *heartbeatInterval)
	taskController.SetExecutionMode(*taskExecutionMode)
	taskController.SetGovernanceStores(stores.Roles, stores.ToolPerms)
	taskController.SetToolApprovalStore(stores.ToolApprovals)
	taskController.SetTaskApprovalStore(stores.TaskApprovals)
	taskController.SetModelEndpointStore(stores.ModelEPs)
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
		fatalLogger.Fatalf("failed to configure isolated tool runtime: %v", err)
	}
	wasmToolRuntime, closeWasm, err := startup.NewWASMToolRuntime(wasmRuntimeCfg, logger)
	if err != nil {
		fatalLogger.Fatalf("failed to configure wasm tool runtime: %v", err)
	}
	if closeWasm != nil {
		defer closeWasm()
	}
	mcpSessionManager := startup.NewMcpSessionManager(startup.McpRuntimeConfig{
		ContainerRuntime: *toolContainerRuntime,
		ContainerNetwork: "bridge",
		ContainerMemory:  *toolContainerMemory,
		ContainerCPUs:    *toolContainerCPUs,
		ContainerPids:    *toolContainerPidsLimit,
		SecretEnvPrefix:  "ORLOJ_SECRET_",
		Secrets:          stores.Secrets,
	})
	defer mcpSessionManager.Close()
	taskController.SetIsolatedToolRuntime(isolatedToolRuntime)
	taskController.SetWasmToolRuntime(wasmToolRuntime)
	taskController.SetMcpRuntime(mcpSessionManager, stores.McpServers)
	if *toolK8sEnabled {
		k8sRT, k8sErr := startup.NewKubernetesToolRuntime(startup.KubernetesToolRuntimeConfig{
			Namespace:       *toolK8sNamespace,
			ServiceAccount:  *toolK8sServiceAccount,
			JobTTLSeconds:   int32(*toolK8sJobTTL),
			DefaultImage:    *toolK8sDefaultImage,
			SecretEnvPrefix: *toolSecretEnvPrefix,
			Secrets:         stores.Secrets,
		}, logger)
		if k8sErr != nil {
			fatalLogger.Fatalf("failed to configure kubernetes tool runtime: %v", k8sErr)
		}
		taskController.SetKubernetesToolRuntime(k8sRT)
	}
	var agentK8sRT *agentruntime.KubernetesAgentRuntime
	if *agentK8sEnabled {
		var agentK8sErr error
		agentK8sRT, agentK8sErr = startup.NewKubernetesAgentRuntime(startup.KubernetesAgentRuntimeConfig{
			Namespace:      *agentK8sNamespace,
			ServiceAccount: *agentK8sServiceAccount,
			Image:          *agentK8sImage,
			JobTTLSeconds:  int32(*agentK8sJobTTL),
			DefaultMemory:  *agentK8sDefaultMemory,
			DefaultCPU:     *agentK8sDefaultCPU,
		}, stores.Tasks, logger)
		if agentK8sErr != nil {
			fatalLogger.Fatalf("failed to configure kubernetes agent runtime: %v", agentK8sErr)
		}
		taskController.SetAgentKubernetesRuntime(agentK8sRT)
	}

	toolStoreResolver := agentruntime.NewStoreSecretResolver(stores.Secrets, "value")
	toolEnvResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(*toolSecretEnvPrefix))
	toolSecretResolver := agentruntime.NewChainSecretResolver(toolStoreResolver, toolEnvResolver)
	cliConfig := agentruntime.CLIToolRuntimeConfig{
		AllowedCommands: startup.ParseCSV(*cliToolAllowedCommands),
		MaxArgvLength:   *cliToolMaxArgvLength,
	}
	taskController.SetCliToolRuntime(cliConfig, toolSecretResolver)

	// A2A tool runtime
	a2aClient := a2a.NewClient(a2a.ClientConfig{
		AllowPrivate: *a2aAllowPrivateEndpoints,
		CardCacheTTL: *a2aCardCacheTTL,
	})
	a2aToolRT := a2a.NewToolRuntime(a2aClient, nil, toolSecretResolver)
	taskController.SetA2AToolRuntime(a2aToolRT)

	taskController.SetContextAdapterStore(stores.ContextAdapters)
	debugLogger.Printf(
		"tool runtime config isolation_backend=%s container_runtime=%s container_image=%s container_network=%s container_memory=%s container_cpus=%s container_pids_limit=%d container_user=%s cli_allowed_commands=%d cli_max_argv_length=%d wasm_entrypoint=%s wasm_memory_bytes=%d wasm_fuel=%d wasm_wasi=%t wasm_cache_configured=%t mcp_container_network=%s",
		*toolIsolationBackend,
		*toolContainerRuntime,
		*toolContainerImage,
		*toolContainerNetwork,
		*toolContainerMemory,
		*toolContainerCPUs,
		*toolContainerPidsLimit,
		*toolContainerUser,
		len(cliConfig.AllowedCommands),
		cliConfig.MaxArgvLength,
		*toolWASMEntrypoint,
		*toolWASMMemoryBytes,
		*toolWASMFuel,
		*toolWASMWASI,
		strings.TrimSpace(*toolWASMCacheDir) != "",
		"bridge",
	)

	agentMessageBus, closeAgentMessageBus := startup.NewAgentMessageBus(
		logger, *agentMessageBusBackend, *agentMessageNATSURL,
		*agentMessageSubjectPrefix, *agentMessageStreamName,
		*agentMessageHistoryMax, *agentMessageDedupeWindow,
		fatalLogger,
	)
	if closeAgentMessageBus != nil {
		defer closeAgentMessageBus()
	}
	taskController.SetAgentMessageBus(agentMessageBus)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	go mcpSessionManager.StartReaper(ctx, 30*time.Second)

	specModels := startup.ParseCSV(*supportedModels)
	go startup.HeartbeatWorkerRegistration(ctx, stores.Workers, logger, *workerID, resources.WorkerSpec{
		Region: *region,
		Capabilities: resources.WorkerCapabilities{
			GPU:             *gpu,
			SupportedModels: specModels,
		},
		MaxConcurrentTasks: *maxConcurrentTasks,
	}, *heartbeatInterval)
	memoryBackendRegistry := agentruntime.NewPersistentMemoryBackendRegistry()
	memoryController := controllers.NewMemoryController(stores.Memories, logger, 5*time.Second)
	memoryController.SetBackendRegistry(memoryBackendRegistry)
	memoryController.SetSecretStore(stores.Secrets)
	memoryController.SetModelEndpointStore(stores.ModelEPs)
	go memoryController.Start(ctx)
	if *agentMessageConsume {
		if agentMessageBus == nil {
			logger.Printf("runtime inbox consumer disabled: agent message bus backend is none")
		} else {
			consumer := agentruntime.NewAgentMessageConsumerManager(
				agentMessageBus, stores.Agents, stores.AgentSystems, stores.Tasks, logger,
				agentruntime.AgentMessageConsumerOptions{
					WorkerID:            *workerID,
					Namespace:           *agentMessageConsumerNamespace,
					RefreshEvery:        *agentMessageConsumerRefresh,
					DedupeWindow:        *agentMessageConsumerDedupe,
					LeaseExtendDuration: *leaseDuration,
					Executor:            taskExecutor,
					Tools:               stores.Tools,
					Roles:               stores.Roles,
					ToolPermissions:     stores.ToolPerms,
					IsolatedToolRuntime: isolatedToolRuntime,
					WasmToolRuntime:     wasmToolRuntime,
					McpSessionManager:   mcpSessionManager,
					McpServerStore:      stores.McpServers,
					CliToolConfig:       cliConfig,
					SecretResolver:      toolSecretResolver,
					Extensions:          extensions,
					Memories:            stores.Memories,
					MemoryBackends:      memoryBackendRegistry,
					ModelEndpoints:      stores.ModelEPs,
					ToolApprovals:       stores.ToolApprovals,
					TaskApprovals:       stores.TaskApprovals,
					Policies:            stores.Policies,
				ContextAdapters:     stores.ContextAdapters,
				A2AToolRuntime:      a2aToolRT,
				AgentK8sRuntime:     agentK8sRT,
				DebugLogger:         debugLogger,
				},
			)
			go consumer.Start(ctx)
			logger.Printf("runtime inbox consumers enabled namespace=%q refresh=%s dedupe=%s",
				strings.TrimSpace(*agentMessageConsumerNamespace),
				agentMessageConsumerRefresh.String(),
				agentMessageConsumerDedupe.String(),
			)
		}
	}

	if addr := strings.TrimSpace(*healthzAddr); addr != "" {
		mux := http.NewServeMux()
		mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "text/plain")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ok"))
		})
		go func() {
			srv := &http.Server{Addr: addr, Handler: mux}
			go func() {
				<-ctx.Done()
				shutCtx, c := context.WithTimeout(context.Background(), 2*time.Second)
				defer c()
				_ = srv.Shutdown(shutCtx)
			}()
			if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				logger.Printf("healthz server error: %v", err)
			}
		}()
		logger.Printf("healthz endpoint listening on %s", addr)
	}

	logger.Printf("task worker starting id=%s lease=%s heartbeat=%s", *workerID, leaseDuration.String(), heartbeatInterval.String())
	taskController.Start(ctx)
}
