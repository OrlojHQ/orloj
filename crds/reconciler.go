package crds

import (
	"context"
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// ResourceSyncer abstracts store operations for one resource kind,
// enabling a single generic reconciler to sync all Orloj CRD types.
type ResourceSyncer[CRD client.Object, Orloj any] struct {
	// ToOrloj converts a CRD object to its Orloj store representation.
	ToOrloj func(crd CRD) Orloj
	// Upsert persists the Orloj resource. Normalize/validate is called
	// internally by the store, so no separate Validate call is needed.
	Upsert func(ctx context.Context, item Orloj) (Orloj, error)
	// Delete removes the resource from the Orloj store by scoped name.
	Delete func(ctx context.Context, name string) error
	// ScopedName returns the store key ("namespace/name" or just "name").
	ScopedName func(crd CRD) string
	// Kind is the human-readable kind name for logging (e.g. "Agent").
	Kind string
}

// SyncReconciler is a generic controller-runtime reconciler that syncs
// a single CRD kind into the Orloj Postgres store.
type SyncReconciler[CRD client.Object, Orloj any] struct {
	client.Client
	Scheme *runtime.Scheme
	Syncer ResourceSyncer[CRD, Orloj]
	// NewCRD returns a new zero-value CRD pointer (e.g. func() *Agent { return &Agent{} }).
	NewCRD func() CRD
}

func (r *SyncReconciler[CRD, Orloj]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.NewCRD()
	if err := r.Get(ctx, req.NamespacedName, obj); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	if !obj.GetDeletionTimestamp().IsZero() {
		return r.handleDelete(ctx, obj)
	}

	return r.handleCreateOrUpdate(ctx, obj)
}

func (r *SyncReconciler[CRD, Orloj]) handleDelete(ctx context.Context, obj CRD) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("kind", r.Syncer.Kind)

	if !controllerutil.ContainsFinalizer(obj, FinalizerSync) {
		return ctrl.Result{}, nil
	}

	scopedName := r.Syncer.ScopedName(obj)
	if err := r.Syncer.Delete(ctx, scopedName); err != nil {
		logger.Error(err, "failed to delete from store", "name", scopedName)
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	controllerutil.RemoveFinalizer(obj, FinalizerSync)
	if err := r.Update(ctx, obj); err != nil {
		return ctrl.Result{}, err
	}

	logger.Info("deleted from store", "name", scopedName)
	return ctrl.Result{}, nil
}

func (r *SyncReconciler[CRD, Orloj]) handleCreateOrUpdate(ctx context.Context, obj CRD) (ctrl.Result, error) {
	logger := log.FromContext(ctx).WithValues("kind", r.Syncer.Kind)
	if !controllerutil.ContainsFinalizer(obj, FinalizerSync) {
		controllerutil.AddFinalizer(obj, FinalizerSync)
		if err := r.Update(ctx, obj); err != nil {
			return ctrl.Result{}, err
		}
	}

	orlojObj := r.Syncer.ToOrloj(obj)
	_, err := r.Syncer.Upsert(ctx, orlojObj)
	if err != nil {
		r.setStatus(ctx, obj, "", err)
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	r.setStatus(ctx, obj, "Synced", nil)
	logger.Info("synced to store", "name", r.Syncer.ScopedName(obj))
	return ctrl.Result{}, nil
}

func (r *SyncReconciler[CRD, Orloj]) setStatus(ctx context.Context, obj CRD, phase string, syncErr error) {
	status := CRDStatus{
		ObservedGeneration: obj.GetGeneration(),
		LastSyncedAt:       time.Now().UTC().Format(time.RFC3339),
	}
	if syncErr != nil {
		status.Phase = "SyncError"
		status.SyncError = syncErr.Error()
	} else {
		status.Phase = phase
	}
	setCRDStatus(obj, status)
	if err := r.Status().Update(ctx, obj); err != nil {
		log.FromContext(ctx).V(1).Info("failed to update CRD status", "kind", r.Syncer.Kind, "error", err)
	}
}

func (r *SyncReconciler[CRD, Orloj]) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(r.NewCRD()).
		Complete(r)
}

func setCRDStatus(obj client.Object, status CRDStatus) {
	switch v := obj.(type) {
	case *Agent:
		v.Status = status
	case *AgentSystem:
		v.Status = status
	case *Tool:
		v.Status = status
	case *McpServer:
		v.Status = status
	case *ModelEndpoint:
		v.Status = status
	case *Memory:
		v.Status = status
	case *AgentPolicy:
		v.Status = status
	case *Secret:
		v.Status = status
	}
}

// ScopedName returns "namespace/name" for store key lookups.
// It respects the orloj.dev/target-namespace annotation if set.
func ScopedName(meta metav1.ObjectMeta) string {
	ns := resolveOrlojNamespace(meta)
	if ns == "" {
		ns = "default"
	}
	return fmt.Sprintf("%s/%s", ns, meta.Name)
}
