package crds

import (
	"context"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/OrlojHQ/orloj/resources"
	"github.com/OrlojHQ/orloj/startup"
)

// StatusWriter periodically reads Orloj store status for CRD-managed
// resources and patches it onto the corresponding CRD .status subresource.
type StatusWriter struct {
	client   client.Client
	stores   *startup.StoreSet
	interval time.Duration
}

func NewStatusWriter(c client.Client, stores *startup.StoreSet, interval time.Duration) *StatusWriter {
	return &StatusWriter{client: c, stores: stores, interval: interval}
}

func (sw *StatusWriter) NeedLeaderElection() bool { return true }

// Start runs the status writeback loop until ctx is cancelled.
// Implements manager.Runnable so it can be added to the controller-runtime Manager.
func (sw *StatusWriter) Start(ctx context.Context) error {
	logger := log.FromContext(ctx).WithName("status-writer")
	ticker := time.NewTicker(sw.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			sw.syncAll(ctx, logger)
		}
	}
}

type statusLogger interface {
	Info(msg string, keysAndValues ...any)
	Error(err error, msg string, keysAndValues ...any)
}

func (sw *StatusWriter) syncAll(ctx context.Context, logger statusLogger) {
	sw.syncKind(ctx, logger, "Agent", sw.syncAgents)
	sw.syncKind(ctx, logger, "AgentSystem", sw.syncAgentSystems)
	sw.syncKind(ctx, logger, "Tool", sw.syncTools)
	sw.syncKind(ctx, logger, "McpServer", sw.syncMcpServers)
	sw.syncKind(ctx, logger, "ModelEndpoint", sw.syncModelEndpoints)
	sw.syncKind(ctx, logger, "Memory", sw.syncMemories)
	sw.syncKind(ctx, logger, "AgentPolicy", sw.syncAgentPolicies)
	sw.syncKind(ctx, logger, "Secret", sw.syncSecrets)
}

func (sw *StatusWriter) syncKind(ctx context.Context, logger statusLogger, kind string, fn func(ctx context.Context) error) {
	if err := fn(ctx); err != nil {
		logger.Error(err, "status writeback failed", "kind", kind)
	}
}

func (sw *StatusWriter) syncAgents(ctx context.Context) error {
	items, err := sw.stores.Agents.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &Agent{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "Agent")
	}
	return nil
}

func (sw *StatusWriter) syncAgentSystems(ctx context.Context) error {
	items, err := sw.stores.AgentSystems.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &AgentSystem{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "AgentSystem")
	}
	return nil
}

func (sw *StatusWriter) syncTools(ctx context.Context) error {
	items, err := sw.stores.Tools.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &Tool{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "Tool")
	}
	return nil
}

func (sw *StatusWriter) syncMcpServers(ctx context.Context) error {
	items, err := sw.stores.McpServers.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &McpServer{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "McpServer")
	}
	return nil
}

func (sw *StatusWriter) syncModelEndpoints(ctx context.Context) error {
	items, err := sw.stores.ModelEPs.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &ModelEndpoint{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "ModelEndpoint")
	}
	return nil
}

func (sw *StatusWriter) syncMemories(ctx context.Context) error {
	items, err := sw.stores.Memories.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &Memory{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "Memory")
	}
	return nil
}

func (sw *StatusWriter) syncAgentPolicies(ctx context.Context) error {
	items, err := sw.stores.Policies.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &AgentPolicy{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus(item.Status.Phase, item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "AgentPolicy")
	}
	return nil
}

func (sw *StatusWriter) syncSecrets(ctx context.Context) error {
	items, err := sw.stores.Secrets.List(ctx)
	if err != nil {
		return err
	}
	for _, item := range items {
		if !IsCRDManaged(item.Metadata) {
			continue
		}
		crd := &Secret{}
		if err := sw.client.Get(ctx, namespacedName(item.Metadata), crd); err != nil {
			continue
		}
		status := buildCRDStatus("Synced", item.Metadata.Generation)
		if crd.Status.Phase == status.Phase && crd.Status.ObservedGeneration == status.ObservedGeneration {
			continue
		}
		crd.Status = status
		sw.updateStatus(ctx, crd, "Secret")
	}
	return nil
}

func (sw *StatusWriter) updateStatus(ctx context.Context, obj client.Object, kind string) {
	if err := sw.client.Status().Update(ctx, obj); err != nil {
		log.FromContext(ctx).V(1).Info("status writeback failed for resource", "kind", kind, "name", obj.GetName(), "error", err)
	}
}

func buildCRDStatus(phase string, generation int64) CRDStatus {
	return CRDStatus{
		Phase:              phase,
		ObservedGeneration: generation,
		LastSyncedAt:       time.Now().UTC().Format(time.RFC3339),
	}
}

// namespacedName returns the K8s ObjectKey for a store resource.
// When orloj.dev/target-namespace is used, the store's Namespace is the
// Orloj namespace (not the K8s namespace). The original K8s namespace is
// preserved in the orloj.dev/source-namespace annotation.
func namespacedName(meta resources.ObjectMeta) client.ObjectKey {
	ns := meta.Namespace
	if src := meta.Annotations[AnnotationSourceNamespace]; src != "" {
		ns = src
	}
	if ns == "" {
		ns = "default"
	}
	return client.ObjectKey{Namespace: ns, Name: meta.Name}
}
