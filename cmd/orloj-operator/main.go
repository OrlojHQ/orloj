package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrllog "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/OrlojHQ/orloj/crds"
	"github.com/OrlojHQ/orloj/internal/version"
	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/startup"
	_ "github.com/jackc/pgx/v5/stdlib"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(crds.AddToScheme(scheme))
}

func main() {
	env := startup.EnvOrDefault

	showVersion := flag.Bool("version", false, "print version and exit")

	storageBackend := flag.String("storage-backend", env("ORLOJ_STORAGE_BACKEND", "postgres"), "storage backend (must be postgres)")
	postgresDSN := flag.String("postgres-dsn", env("ORLOJ_POSTGRES_DSN", ""), "PostgreSQL connection string")
	sqlDriver := flag.String("sql-driver", env("ORLOJ_SQL_DRIVER", "pgx"), "SQL driver name")
	secretKey := flag.String("secret-encryption-key", env("ORLOJ_SECRET_ENCRYPTION_KEY", ""), "AES key for secret encryption (hex)")

	metricsAddr := flag.String("metrics-addr", ":8080", "Prometheus metrics listen address")
	healthAddr := flag.String("healthz-addr", ":8081", "health probe listen address")
	leaderElect := flag.Bool("leader-elect", true, "enable leader election for HA")
	leaderNS := flag.String("leader-election-namespace", env("ORLOJ_OPERATOR_NAMESPACE", "default"), "namespace for leader election Lease")
	statusInterval := flag.Duration("status-sync-interval", 5*time.Second, "interval for status writeback to CRDs")

	flag.Parse()

	if *showVersion {
		fmt.Println(version.String())
		return
	}

	logger := log.New(os.Stderr, "[orloj-operator] ", log.LstdFlags)

	if *storageBackend != "postgres" {
		logger.Fatalf("--storage-backend must be 'postgres' for orloj-operator")
	}

	encKey, err := startup.ParseSecretEncryptionKey(*secretKey)
	if err != nil {
		logger.Fatalf("invalid --secret-encryption-key: %v", err)
	}

	stores, err := startup.OpenStores(startup.StoreConfig{
		Backend:               *storageBackend,
		PostgresDSN:           *postgresDSN,
		SQLDriver:             *sqlDriver,
		SecretEncryptionKey:   encKey,
		IncludeScheduleStores: false,
	}, logger)
	if err != nil {
		logger.Fatalf("failed to open stores: %v", err)
	}
	defer stores.Close()

	ctrl.SetLogger(ctrllog.New())

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                  scheme,
		LeaderElection:          *leaderElect,
		LeaderElectionID:        "orloj-operator-leader",
		LeaderElectionNamespace: *leaderNS,
		HealthProbeBindAddress:  *healthAddr,
		Metrics:                 metricsserver.Options{BindAddress: *metricsAddr},
	})
	if err != nil {
		logger.Fatalf("unable to create manager: %v", err)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Fatalf("unable to add healthz check: %v", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Fatalf("unable to add readyz check: %v", err)
	}

	registerReconcilers(mgr, stores)

	sw := crds.NewStatusWriter(mgr.GetClient(), stores, *statusInterval)
	if err := mgr.Add(sw); err != nil {
		logger.Fatalf("unable to add status writer: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	logger.Printf("starting orloj-operator %s", version.String())
	if err := mgr.Start(ctx); err != nil {
		logger.Fatalf("operator exited with error: %v", err)
	}
}

func registerReconcilers(mgr ctrl.Manager, stores *startup.StoreSet) {
	must := func(err error) {
		if err != nil {
			log.Fatalf("unable to register reconciler: %v", err)
		}
	}

	must((&crds.SyncReconciler[*crds.Agent, resources.Agent]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.Agent { return &crds.Agent{} },
		Syncer: crds.ResourceSyncer[*crds.Agent, resources.Agent]{
			Kind:       "Agent",
			ToOrloj:    crds.AgentToOrloj,
			Upsert:     stores.Agents.Upsert,
			Delete:     stores.Agents.Delete,
			ScopedName: func(a *crds.Agent) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.AgentSystem, resources.AgentSystem]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.AgentSystem { return &crds.AgentSystem{} },
		Syncer: crds.ResourceSyncer[*crds.AgentSystem, resources.AgentSystem]{
			Kind:       "AgentSystem",
			ToOrloj:    crds.AgentSystemToOrloj,
			Upsert:     stores.AgentSystems.Upsert,
			Delete:     stores.AgentSystems.Delete,
			ScopedName: func(a *crds.AgentSystem) string { return crds.ScopedName(a.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.Tool, resources.Tool]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.Tool { return &crds.Tool{} },
		Syncer: crds.ResourceSyncer[*crds.Tool, resources.Tool]{
			Kind:       "Tool",
			ToOrloj:    crds.ToolToOrloj,
			Upsert:     stores.Tools.Upsert,
			Delete:     stores.Tools.Delete,
			ScopedName: func(t *crds.Tool) string { return crds.ScopedName(t.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.McpServer, resources.McpServer]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.McpServer { return &crds.McpServer{} },
		Syncer: crds.ResourceSyncer[*crds.McpServer, resources.McpServer]{
			Kind:       "McpServer",
			ToOrloj:    crds.McpServerToOrloj,
			Upsert:     stores.McpServers.Upsert,
			Delete:     stores.McpServers.Delete,
			ScopedName: func(m *crds.McpServer) string { return crds.ScopedName(m.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.ModelEndpoint, resources.ModelEndpoint]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.ModelEndpoint { return &crds.ModelEndpoint{} },
		Syncer: crds.ResourceSyncer[*crds.ModelEndpoint, resources.ModelEndpoint]{
			Kind:       "ModelEndpoint",
			ToOrloj:    crds.ModelEndpointToOrloj,
			Upsert:     stores.ModelEPs.Upsert,
			Delete:     stores.ModelEPs.Delete,
			ScopedName: func(m *crds.ModelEndpoint) string { return crds.ScopedName(m.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.Memory, resources.Memory]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.Memory { return &crds.Memory{} },
		Syncer: crds.ResourceSyncer[*crds.Memory, resources.Memory]{
			Kind:       "Memory",
			ToOrloj:    crds.MemoryToOrloj,
			Upsert:     stores.Memories.Upsert,
			Delete:     stores.Memories.Delete,
			ScopedName: func(m *crds.Memory) string { return crds.ScopedName(m.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.AgentPolicy, resources.AgentPolicy]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.AgentPolicy { return &crds.AgentPolicy{} },
		Syncer: crds.ResourceSyncer[*crds.AgentPolicy, resources.AgentPolicy]{
			Kind:       "AgentPolicy",
			ToOrloj:    crds.AgentPolicyToOrloj,
			Upsert:     stores.Policies.Upsert,
			Delete:     stores.Policies.Delete,
			ScopedName: func(p *crds.AgentPolicy) string { return crds.ScopedName(p.ObjectMeta) },
		},
	}).SetupWithManager(mgr))

	must((&crds.SyncReconciler[*crds.Secret, resources.Secret]{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
		NewCRD: func() *crds.Secret { return &crds.Secret{} },
		Syncer: crds.ResourceSyncer[*crds.Secret, resources.Secret]{
			Kind:       "Secret",
			ToOrloj:    crds.SecretToOrloj,
			Upsert:     stores.Secrets.Upsert,
			Delete:     stores.Secrets.Delete,
			ScopedName: func(s *crds.Secret) string { return crds.ScopedName(s.ObjectMeta) },
		},
	}).SetupWithManager(mgr))
}
