package main

import (
	"context"
	"encoding/json"
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
	"github.com/OrlojHQ/orloj/runtime/a2a"
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
	logLevelRaw := flag.String("log-level", env("ORLOJ_LOG_LEVEL", "info"), "minimum log level: debug|info|warn|error (env: ORLOJ_LOG_LEVEL)")
	debugLogs := flag.Bool("debug", false, "enable debug logging (equivalent to --log-level=debug)")
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
	crdConflictPolicy := flag.String("crd-conflict-policy", env("ORLOJ_CRD_CONFLICT_POLICY", "warn"), "CRD conflict guard: off|warn|reject (env: ORLOJ_CRD_CONFLICT_POLICY)")
	toolSecretEnvPrefix := flag.String("tool-secret-env-prefix", env("ORLOJ_TOOL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve Tool.spec.auth.secretRef")
	toolWASMModule := flag.String("tool-wasm-module", env("ORLOJ_TOOL_WASM_MODULE", ""), "default wasm module path (per-tool spec.wasm.module takes precedence)")
	toolWASMEntrypoint := flag.String("tool-wasm-entrypoint", env("ORLOJ_TOOL_WASM_ENTRYPOINT", "run"), "default wasm entrypoint function")
	toolWASMMemoryBytes := flag.Int64("tool-wasm-memory-bytes", envInt64("ORLOJ_TOOL_WASM_MEMORY_BYTES", 64*1024*1024), "default max wasm runtime memory bytes")
	toolWASMFuel := flag.Uint64("tool-wasm-fuel", envUint64("ORLOJ_TOOL_WASM_FUEL", 1000000), "default wasm execution fuel limit")
	toolWASMWASI := flag.Bool("tool-wasm-wasi", envBool("ORLOJ_TOOL_WASM_WASI", true), "default: enable WASI host functions for wasm tools")
	toolWASMCacheDir := flag.String("tool-wasm-cache-dir", env("ORLOJ_TOOL_WASM_CACHE_DIR", ""), "disk cache directory for remote WASM modules (default: ~/.orloj/wasm-cache)")
	cliToolAllowedCommands := flag.String("cli-tool-allowed-commands", env("ORLOJ_CLI_TOOL_ALLOWED_COMMANDS", ""), "comma-separated allowlist of commands for CLI tools (empty allows all)")
	cliToolMaxArgvLength := flag.Int("cli-tool-max-argv-length", envInt("ORLOJ_CLI_TOOL_MAX_ARGV_LENGTH", 4096), "max total argv byte length for CLI tool invocations")
	a2aPublicBaseURL := flag.String("a2a-public-base-url", env("ORLOJ_A2A_PUBLIC_BASE_URL", ""), "public base URL for A2A Agent Cards (env: ORLOJ_A2A_PUBLIC_BASE_URL)")
	a2aProtocolVersion := flag.String("a2a-protocol-version", env("ORLOJ_A2A_PROTOCOL_VERSION", ""), "A2A protocol version to advertise (env: ORLOJ_A2A_PROTOCOL_VERSION)")
	a2aCardCacheTTL := flag.Duration("a2a-card-cache-ttl", envDuration("ORLOJ_A2A_CARD_CACHE_TTL", 5*time.Minute), "TTL for cached remote Agent Cards (env: ORLOJ_A2A_CARD_CACHE_TTL)")
	a2aAllowPrivateEndpoints := flag.Bool("a2a-allow-private-endpoints", envBool("ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS", false), "allow outbound A2A requests to private/RFC1918 addresses (env: ORLOJ_A2A_ALLOW_PRIVATE_ENDPOINTS)")
	a2aRemoteAgents := flag.String("a2a-remote-agents", env("ORLOJ_A2A_REMOTE_AGENTS", ""), "JSON-encoded array of remote A2A agent configs (env: ORLOJ_A2A_REMOTE_AGENTS)")
	a2aRateLimitEnabled := flag.Bool("a2a-rate-limit-enabled", envBool("ORLOJ_A2A_RATE_LIMIT_ENABLED", true), "enable per-IP rate limiting on A2A endpoints (env: ORLOJ_A2A_RATE_LIMIT_ENABLED)")
	a2aRateLimitRPM := flag.Int("a2a-rate-limit-rpm", envInt("ORLOJ_A2A_RATE_LIMIT_RPM", 30), "A2A requests per minute per IP (env: ORLOJ_A2A_RATE_LIMIT_RPM)")
	a2aRateLimitMaxSubscribe := flag.Int("a2a-rate-limit-max-subscribe", envInt("ORLOJ_A2A_RATE_LIMIT_MAX_SUBSCRIBE", 10), "max concurrent A2A SSE subscriptions globally (env: ORLOJ_A2A_RATE_LIMIT_MAX_SUBSCRIBE)")
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
	eventBusBackend := flag.String("event-bus-backend", env("ORLOJ_EVENT_BUS_BACKEND", "memory"), "event bus backend: memory|nats")
	natsURL := flag.String("nats-url", env("ORLOJ_NATS_URL", "nats://127.0.0.1:4222"), "NATS server URL used when --event-bus-backend=nats")
	natsSubjectPrefix := flag.String("nats-subject-prefix", env("ORLOJ_NATS_SUBJECT_PREFIX", "orloj.controlplane"), "NATS subject prefix used when --event-bus-backend=nats")
	agentMessageBusBackend := flag.String("agent-message-bus-backend", env("ORLOJ_AGENT_MESSAGE_BUS_BACKEND", "none"), "runtime agent message bus backend: none|memory|nats-jetstream")
	agentMessageNATSURL := flag.String("agent-message-nats-url", env("ORLOJ_AGENT_MESSAGE_NATS_URL", env("ORLOJ_NATS_URL", "nats://127.0.0.1:4222")), "NATS server URL used when --agent-message-bus-backend=nats-jetstream")
	agentMessageSubjectPrefix := flag.String("agent-message-subject-prefix", env("ORLOJ_AGENT_MESSAGE_SUBJECT_PREFIX", "orloj.agentmsg"), "runtime agent message subject prefix")
	agentMessageStreamName := flag.String("agent-message-stream-name", env("ORLOJ_AGENT_MESSAGE_STREAM", "ORLOJ_AGENT_MESSAGES"), "JetStream stream name for runtime agent messages")
	agentMessageHistoryMax := flag.Int("agent-message-history-max", 2048, "in-memory runtime agent message history capacity")
	agentMessageDedupeWindow := flag.Duration("agent-message-dedupe-window", 2*time.Minute, "in-memory runtime agent message dedupe window")
	secretEncryptionKeyRaw := flag.String("secret-encryption-key", env("ORLOJ_SECRET_ENCRYPTION_KEY", ""), "256-bit AES key (hex or base64) for encrypting Secret resource data at rest (prefer ORLOJ_SECRET_ENCRYPTION_KEY env var)")
	corsAllowedOrigins := flag.String("cors-allowed-origins", env("ORLOJ_CORS_ALLOWED_ORIGINS", ""), "comma-separated CORS allowed origins (empty = same-origin only; env: ORLOJ_CORS_ALLOWED_ORIGINS)")
	tlsCertFile := flag.String("tls-cert-file", env("ORLOJ_TLS_CERT_FILE", ""), "TLS certificate file for HTTPS (env: ORLOJ_TLS_CERT_FILE)")
	tlsKeyFile := flag.String("tls-key-file", env("ORLOJ_TLS_KEY_FILE", ""), "TLS private key file for HTTPS (env: ORLOJ_TLS_KEY_FILE)")
	storageBackend := flag.String("storage-backend", "memory", "state backend: memory|postgres")
	postgresDSN := flag.String("postgres-dsn", os.Getenv("ORLOJ_POSTGRES_DSN"), "postgres DSN (required when --storage-backend=postgres)")
	sqlDriver := flag.String("sql-driver", "pgx", "database/sql driver name used for --storage-backend=postgres")
	postgresMaxOpenConns := flag.Int("postgres-max-open-conns", 20, "max open postgres connections")
	postgresMaxIdleConns := flag.Int("postgres-max-idle-conns", 10, "max idle postgres connections")
	postgresConnMaxLifetime := flag.Duration("postgres-conn-max-lifetime", 30*time.Minute, "max lifetime of postgres connections")
	flag.Parse()

	warnIfSecretFlagSet("api-key", "ORLOJ_API_TOKEN")
	warnIfSecretFlagSet("secret-encryption-key", "ORLOJ_SECRET_ENCRYPTION_KEY")
	warnIfSecretFlagSet("auth-reset-admin-password", "ORLOJ_AUTH_RESET_ADMIN_PASSWORD")

	if *showVersion {
		fmt.Println(version.String())
		os.Exit(0)
	}

	if v := strings.TrimSpace(*apiKey); v != "" && os.Getenv("ORLOJ_API_TOKEN") == "" {
		os.Setenv("ORLOJ_API_TOKEN", v)
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

	slogger := telemetry.NewLoggerWithLevel("orlojd", parsedLogLevel)
	logger := telemetry.NewBridgeLogger(slogger)
	debugLogger := telemetry.NewDebugBridgeLogger(slogger)
	fatalLogger := telemetry.NewErrorBridgeLogger(slogger)
	resolvedLogLevelLabel := strings.ToLower(strings.TrimSpace(resolvedLogLevel))
	if resolvedLogLevelLabel == "" {
		resolvedLogLevelLabel = "info"
	}

	authMode, authModeErr := parseAuthMode(*authModeRaw)
	if authModeErr != nil {
		fatalLogger.Fatalf("%v", authModeErr)
	}

	debugLogger.Printf(
		"startup config addr=%s ui_path=%s log_level=%s auth_mode=%s api_token_configured=%t trusted_proxies_configured=%t storage_backend=%s postgres_dsn_configured=%t task_execution_mode=%s embedded_worker=%t event_bus_backend=%s agent_message_bus_backend=%s tool_isolation_backend=%s tool_container_runtime=%s tool_container_network=%s wasm_module_configured=%t",
		*addr,
		*uiPath,
		resolvedLogLevelLabel,
		*authModeRaw,
		strings.TrimSpace(*apiKey) != "",
		strings.TrimSpace(*trustedProxies) != "",
		*storageBackend,
		strings.TrimSpace(*postgresDSN) != "",
		*taskExecutionMode,
		*runTaskWorker || *embeddedWorker,
		*eventBusBackend,
		*agentMessageBusBackend,
		*toolIsolationBackend,
		*toolContainerRuntime,
		*toolContainerNetwork,
		strings.TrimSpace(*toolWASMModule) != "",
	)

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
		IncludeScheduleStores: true,
	}, logger)
	if err != nil {
		fatalLogger.Fatalf("%v", err)
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
			fatalLogger.Fatalf("admin password reset failed: %v", err)
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
	mcpSessionManager := startup.NewMcpSessionManager(startup.McpRuntimeConfig{
		ContainerRuntime: *toolContainerRuntime,
		ContainerNetwork: "bridge",
		ContainerMemory:  *toolContainerMemory,
		ContainerCPUs:    *toolContainerCPUs,
		ContainerPids:    *toolContainerPidsLimit,
		SecretEnvPrefix:  "ORLOJ_SECRET_",
		Secrets:          stores.Secrets,
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
	taskController.SetDebugLogger(debugLogger)
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
		fatalLogger.Fatalf("failed to configure isolated tool runtime: %v", err)
	}
	wasmToolRuntime, closeWasm, err := startup.NewWASMToolRuntime(wasmRuntimeCfg, logger)
	if err != nil {
		fatalLogger.Fatalf("failed to configure wasm tool runtime: %v", err)
	}
	if closeWasm != nil {
		defer closeWasm()
	}
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
	if *agentK8sEnabled {
		agentK8sRT, agentK8sErr := startup.NewKubernetesAgentRuntime(startup.KubernetesAgentRuntimeConfig{
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

	cliStoreResolver := agentruntime.NewStoreSecretResolver(stores.Secrets, "value")
	cliEnvResolver := agentruntime.NewEnvSecretResolver(strings.TrimSpace(*toolSecretEnvPrefix))
	cliSecretResolver := agentruntime.NewChainSecretResolver(cliStoreResolver, cliEnvResolver)
	taskController.SetCliToolRuntime(agentruntime.CLIToolRuntimeConfig{
		AllowedCommands: startup.ParseCSV(*cliToolAllowedCommands),
		MaxArgvLength:   *cliToolMaxArgvLength,
	}, cliSecretResolver)

	// A2A outbound tool runtime — always available so type=a2a tools work
	// regardless of whether the inbound A2A API is enabled.
	a2aClient := a2a.NewClient(a2a.ClientConfig{
		AllowPrivate: *a2aAllowPrivateEndpoints,
		CardCacheTTL: *a2aCardCacheTTL,
	})
	a2aToolRT := a2a.NewToolRuntime(a2aClient, nil, cliSecretResolver)
	taskController.SetA2AToolRuntime(a2aToolRT)

	// A2A inbound protocol (API endpoints, registry, agent cards)
	var a2aRegistry *a2a.Registry
	var remoteAgentConfigs []a2a.RemoteAgentConfig
	if raw := strings.TrimSpace(*a2aRemoteAgents); raw != "" {
		if err := json.Unmarshal([]byte(raw), &remoteAgentConfigs); err != nil {
			logger.Printf("WARNING: failed to parse ORLOJ_A2A_REMOTE_AGENTS: %v", err)
		}
	}

	if len(remoteAgentConfigs) > 0 {
		a2aRegistry = a2a.NewRegistry(a2aClient, remoteAgentConfigs, *a2aCardCacheTTL, logger)
	}

	authSchemes := []string{}
	if authMode != api.AuthModeOff || strings.TrimSpace(*apiKey) != "" || strings.TrimSpace(os.Getenv("ORLOJ_API_TOKENS")) != "" {
		authSchemes = append(authSchemes, "bearer")
	}

	rateLimitRPM := 0
	maxConcurrentSubscribe := 0
	if *a2aRateLimitEnabled {
		rateLimitRPM = *a2aRateLimitRPM
		maxConcurrentSubscribe = *a2aRateLimitMaxSubscribe
	}
	a2aConfig := &api.A2AConfig{
		PublicBaseURL:          strings.TrimSpace(*a2aPublicBaseURL),
		ProtocolVersion:        strings.TrimSpace(*a2aProtocolVersion),
		StreamingEnabled:       true,
		AuthSchemes:            authSchemes,
		Registry:               a2aRegistry,
		RateLimitRPM:           rateLimitRPM,
		MaxConcurrentSubscribe: maxConcurrentSubscribe,
	}

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
		len(startup.ParseCSV(*cliToolAllowedCommands)),
		*cliToolMaxArgvLength,
		*toolWASMEntrypoint,
		*toolWASMMemoryBytes,
		*toolWASMFuel,
		*toolWASMWASI,
		strings.TrimSpace(*toolWASMCacheDir) != "",
		"bridge",
	)

	// Token auth (ORLOJ_API_TOKEN, ORLOJ_API_TOKENS, DB minted tokens) is wired
	// via newTokenAuthorizerWithStoreFromEnv inside NewServerWithOptions.
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
		EvalDatasets:    stores.EvalDatasets,
		EvalRuns:        stores.EvalRuns,
		LocalAdmins:     stores.LocalAdmins,
		APITokens:       stores.APITokens,
		AuthSessions:    stores.AuthSessions,
	}, runtime, logger, api.ServerOptions{
		Extensions:     extensions,
		AuthMode:       authMode,
		SessionTTL:     *authSessionTTL,
		UIBasePath:     *uiPath,
		TrustedProxies: *trustedProxies,
		CORSAllowedOrigins: parseCSVList(*corsAllowedOrigins),
		ContainerResourceCeiling: resources.ContainerResourceCeiling{
			MaxMemory:    *toolContainerMaxMemory,
			MaxCPUs:      *toolContainerMaxCPUs,
			MaxPidsLimit: *toolContainerMaxPidsLimit,
		},
		CRDConflictPolicy: *crdConflictPolicy,
	})
	bus, closeBus := newEventBus(logger, fatalLogger, *eventBusBackend, *natsURL, *natsSubjectPrefix)
	if closeBus != nil {
		defer closeBus()
	}
	agentMessageBus, closeAgentMessageBus := startup.NewAgentMessageBus(
		logger, *agentMessageBusBackend, *agentMessageNATSURL,
		*agentMessageSubjectPrefix, *agentMessageStreamName,
		*agentMessageHistoryMax, *agentMessageDedupeWindow,
		fatalLogger,
	)
	if closeAgentMessageBus != nil {
		defer closeAgentMessageBus()
	}
	server.SetEventBus(bus)
	server.SetMemoryBackends(memoryBackendRegistry)
	server.SetA2AConfig(a2aConfig)
	logger.Printf("A2A protocol configured base_url=%s protocol_version=%s", a2aConfig.PublicBaseURL, a2aConfig.ProtocolVersion)
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
	if a2aRegistry != nil {
		startBackground(func() { a2aRegistry.Start(ctx) })
	}
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
		evalRunController := controllers.NewEvalRunController(
			stores.EvalRuns, stores.EvalDatasets, stores.Tasks,
			&agentruntime.EvalScorer{Gateway: modelGateway},
			logger,
		)
		startBackground(func() { evalRunController.Start(ctx) })
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
						A2AToolRuntime:      a2aToolRT,
						DebugLogger:         debugLogger,
						OnStepEvent: func(taskName, namespace string, evt agentruntime.AgentStepEvent) {
							if bus != nil {
								bus.Publish(eventbus.Event{
									Source:    "task-controller",
									Type:      "task.trace",
									Kind:      "Task",
									Name:      taskName,
									Namespace: namespace,
									Data:      evt,
								})
							}
						},
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
	if strings.TrimSpace(*tlsCertFile) != "" || strings.TrimSpace(*tlsKeyFile) != "" {
		if strings.TrimSpace(*tlsCertFile) == "" || strings.TrimSpace(*tlsKeyFile) == "" {
			fatalLogger.Fatalf("both --tls-cert-file and --tls-key-file are required for TLS")
		}
		if err := httpServer.ListenAndServeTLS(*tlsCertFile, *tlsKeyFile); err != nil && err != http.ErrServerClosed {
			fatalLogger.Fatalf("server error: %v", err)
		}
	} else if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		fatalLogger.Fatalf("server error: %v", err)
	}
	wg.Wait()
}

func warnIfSecretFlagSet(flagName, envName string) {
	set := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == flagName {
			set = true
		}
	})
	if set {
		log.Printf("WARNING: secret %q was passed via CLI flag; prefer env var %s to avoid exposure in process listings", flagName, envName)
	}
}

func parseCSVList(raw string) []string {
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
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

func newEventBus(logger *log.Logger, fatalLogger *log.Logger, backend, natsURL, subjectPrefix string) (eventbus.Bus, func()) {
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
			if fatalLogger != nil {
				fatalLogger.Fatalf("failed to initialize nats event bus: %v", err)
			}
		}
		return bus, func() { _ = bus.Close() }
	default:
		if fatalLogger != nil {
			fatalLogger.Fatalf("unsupported event bus backend %q; expected memory or nats", backend)
		}
		return eventbus.NewMemoryBus(8192), nil
	}
}
