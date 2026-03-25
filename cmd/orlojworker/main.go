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
	"github.com/OrlojHQ/orloj/startup"
	"github.com/OrlojHQ/orloj/telemetry"
	_ "github.com/jackc/pgx/v5/stdlib"
)

func main() {
	env := startup.EnvOrDefault
	envBool := startup.EnvBoolOrDefault
	envInt := startup.EnvIntOrDefault
	envInt64 := startup.EnvInt64OrDefault
	envUint64 := startup.EnvUint64OrDefault

	showVersion := flag.Bool("version", false, "print version and exit")
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
	toolIsolationBackend := flag.String("tool-isolation-backend", env("ORLOJ_TOOL_ISOLATION_BACKEND", "none"), "isolated tool executor backend: none|container|wasm")
	toolContainerRuntime := flag.String("tool-container-runtime", env("ORLOJ_TOOL_CONTAINER_RUNTIME", "docker"), "container runtime binary for isolated tool execution")
	toolContainerImage := flag.String("tool-container-image", env("ORLOJ_TOOL_CONTAINER_IMAGE", "curlimages/curl:8.8.0"), "container image used by isolated tool execution")
	toolContainerNetwork := flag.String("tool-container-network", env("ORLOJ_TOOL_CONTAINER_NETWORK", "none"), "container network mode for isolated tools")
	toolContainerMemory := flag.String("tool-container-memory", env("ORLOJ_TOOL_CONTAINER_MEMORY", "128m"), "container memory limit for isolated tools")
	toolContainerCPUs := flag.String("tool-container-cpus", env("ORLOJ_TOOL_CONTAINER_CPUS", "0.50"), "container CPU limit for isolated tools")
	toolContainerPidsLimit := flag.Int("tool-container-pids-limit", envInt("ORLOJ_TOOL_CONTAINER_PIDS_LIMIT", 64), "container pids limit for isolated tools")
	toolContainerUser := flag.String("tool-container-user", env("ORLOJ_TOOL_CONTAINER_USER", "65532:65532"), "container user for isolated tools")
	toolSecretEnvPrefix := flag.String("tool-secret-env-prefix", env("ORLOJ_TOOL_SECRET_ENV_PREFIX", "ORLOJ_SECRET_"), "environment variable prefix used to resolve Tool.spec.auth.secretRef")
	toolWASMModule := flag.String("tool-wasm-module", env("ORLOJ_TOOL_WASM_MODULE", ""), "wasm module path or identifier for wasm tool isolation runtime")
	toolWASMEntrypoint := flag.String("tool-wasm-entrypoint", env("ORLOJ_TOOL_WASM_ENTRYPOINT", "run"), "wasm entrypoint function for wasm tool isolation runtime")
	toolWASMRuntimeBinary := flag.String("tool-wasm-runtime-binary", env("ORLOJ_TOOL_WASM_RUNTIME_BINARY", "wasmtime"), "wasm runtime binary used by command-backed wasm executor")
	toolWASMRuntimeArgs := flag.String("tool-wasm-runtime-args", env("ORLOJ_TOOL_WASM_RUNTIME_ARGS", ""), "comma-separated extra args passed to wasm runtime binary")
	toolWASMMemoryBytes := flag.Int64("tool-wasm-memory-bytes", envInt64("ORLOJ_TOOL_WASM_MEMORY_BYTES", 64*1024*1024), "max wasm runtime memory bytes for tool isolation runtime")
	toolWASMFuel := flag.Uint64("tool-wasm-fuel", envUint64("ORLOJ_TOOL_WASM_FUEL", 0), "optional wasm execution fuel limit (0 disables fuel limiting)")
	toolWASMWASI := flag.Bool("tool-wasm-wasi", envBool("ORLOJ_TOOL_WASM_WASI", true), "enable WASI host functions for wasm tool isolation runtime")
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

	slogger := telemetry.NewLogger("orlojworker")
	logger := telemetry.NewBridgeLogger(slogger)

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
		IncludeScheduleStores: false,
	}, logger)
	if err != nil {
		logger.Fatalf("%v", err)
	}
	defer stores.Close()

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
	taskController.ConfigureWorker(*workerID, *leaseDuration, *heartbeatInterval)
	taskController.SetExecutionMode(*taskExecutionMode)
	taskController.SetGovernanceStores(stores.Roles, stores.ToolPerms)
	taskController.SetToolApprovalStore(stores.ToolApprovals)
	taskController.SetModelEndpointStore(stores.ModelEPs)
	taskController.SetExecutor(taskExecutor)
	taskController.SetExtensions(extensions)
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
		WASMModule:       *toolWASMModule,
		WASMEntrypoint:   *toolWASMEntrypoint,
		WASMRuntimeBin:   *toolWASMRuntimeBinary,
		WASMRuntimeArgs:  startup.ParseCSV(*toolWASMRuntimeArgs),
		WASMMemoryBytes:  *toolWASMMemoryBytes,
		WASMFuel:         *toolWASMFuel,
		WASMWASI:         *toolWASMWASI,
		Secrets:          stores.Secrets,
	}, logger)
	if err != nil {
		logger.Fatalf("failed to configure isolated tool runtime: %v", err)
	}
	taskController.SetIsolatedToolRuntime(isolatedToolRuntime)
	agentMessageBus, closeAgentMessageBus := startup.NewAgentMessageBus(
		logger, *agentMessageBusBackend, *agentMessageNATSURL,
		*agentMessageSubjectPrefix, *agentMessageStreamName,
		*agentMessageHistoryMax, *agentMessageDedupeWindow,
	)
	if closeAgentMessageBus != nil {
		defer closeAgentMessageBus()
	}
	taskController.SetAgentMessageBus(agentMessageBus)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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
					Extensions:          extensions,
					Memories:            stores.Memories,
					MemoryBackends:      memoryBackendRegistry,
					ModelEndpoints:      stores.ModelEPs,
					ToolApprovals:       stores.ToolApprovals,
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

