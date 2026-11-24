package controller

import (
	"context"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

func TestTargetClusterReconciler_Heartbeat(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = protemv1alpha1.AddToScheme(scheme)

	now := metav1.Now()
	expiredTime := metav1.NewTime(time.Now().Add(-120 * time.Second))

	tests := []struct {
		name          string
		cluster       *protemv1alpha1.TargetCluster
		expectedPhase protemv1alpha1.ClusterPhase
	}{
		{
			name: "Active Heartbeat -> Ready",
			cluster: &protemv1alpha1.TargetCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "c-active"},
				Status: protemv1alpha1.TargetClusterStatus{
					LastHeartbeat: &now,
				},
			},
			expectedPhase: protemv1alpha1.ClusterPhaseReady,
		},
		{
			name: "Expired Heartbeat -> Disconnected",
			cluster: &protemv1alpha1.TargetCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "c-expired"},
				Status: protemv1alpha1.TargetClusterStatus{
					LastHeartbeat: &expiredTime,
				},
			},
			expectedPhase: protemv1alpha1.ClusterPhaseDisconnected,
		},
		{
			name: "No Heartbeat -> Disconnected",
			cluster: &protemv1alpha1.TargetCluster{
				ObjectMeta: metav1.ObjectMeta{Name: "c-never"},
			},
			expectedPhase: protemv1alpha1.ClusterPhaseDisconnected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			client := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&protemv1alpha1.TargetCluster{}).WithObjects(tt.cluster).Build()
			reconciler := &TargetClusterReconciler{Client: client, Scheme: scheme}

			_, err := reconciler.Reconcile(context.Background(), ctrl.Request{
				NamespacedName: types.NamespacedName{Name: tt.cluster.Name},
			})
			if err != nil {
				t.Fatalf("unexpected reconcile error: %v", err)
			}

			var updated protemv1alpha1.TargetCluster
			_ = client.Get(context.Background(), types.NamespacedName{Name: tt.cluster.Name}, &updated)
			if updated.Status.Phase != tt.expectedPhase {
				t.Errorf("expected phase %q, got %q", tt.expectedPhase, updated.Status.Phase)
			}
		})
	}
}

func reconcileCluster(t *testing.T, r *TargetClusterReconciler, name string) {
	t.Helper()
	if _, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Name: name}}); err != nil {
		t.Fatalf("reconcile %s: %v", name, err)
	}
}

func TestTargetClusterReconciler_AgentIdentityLifecycle(t *testing.T) {
	ctx := context.Background()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	_ = protemv1alpha1.AddToScheme(scheme)
	edge := &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "edge"}}
	hub := &protemv1alpha1.TargetCluster{ObjectMeta: metav1.ObjectMeta{Name: "local"}}
	ns := fleet.ClusterNamespace(edge.Name)
	lease := &coordinationv1.Lease{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: fleet.AgentLeaseName}}
	leftover := &protemv1alpha1.ClusterGrant{ObjectMeta: metav1.ObjectMeta{Namespace: ns, Name: "team-a.g1"}}
	elsewhere := &protemv1alpha1.ClusterGrant{ObjectMeta: metav1.ObjectMeta{Namespace: "protem-cluster-other", Name: "team-a.g1"}}
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&protemv1alpha1.TargetCluster{}).
		WithObjects(edge, hub, lease, leftover, elsewhere).Build()
	r := &TargetClusterReconciler{Client: c, Scheme: scheme, HubClusterName: "local"}

	reconcileCluster(t, r, edge.Name)
	reconcileCluster(t, r, hub.Name)
	sa := client.ObjectKey{Namespace: ns, Name: fleet.AgentName}
	for _, obj := range []client.Object{&corev1.ServiceAccount{}, &rbacv1.Role{}, &rbacv1.RoleBinding{}} {
		if err := c.Get(ctx, sa, obj); err != nil {
			t.Errorf("%T for edge: %v", obj, err)
		}
	}
	if err := c.Get(ctx, client.ObjectKey{Namespace: fleet.ClusterNamespace(hub.Name), Name: fleet.AgentName}, &corev1.ServiceAccount{}); !apierrors.IsNotFound(err) {
		t.Errorf("hub has an agent ServiceAccount: %v", err)
	}
	_ = c.Get(ctx, client.ObjectKeyFromObject(hub), hub)
	if len(hub.Finalizers) != 0 {
		t.Errorf("hub finalizers = %v, want none", hub.Finalizers)
	}

	if err := c.Delete(ctx, edge); err != nil {
		t.Fatal(err)
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(edge), edge); err != nil {
		t.Fatalf("edge was deleted without its finalizer running: %v", err)
	}
	reconcileCluster(t, r, edge.Name)
	for _, obj := range []client.Object{&corev1.ServiceAccount{}, &rbacv1.RoleBinding{}} {
		if err := c.Get(ctx, sa, obj); !apierrors.IsNotFound(err) {
			t.Errorf("%T after deletion: err = %v, want not found", obj, err)
		}
	}
	if err := c.Get(ctx, sa, &rbacv1.Role{}); err != nil {
		t.Errorf("Role after deletion: %v, want it left in place", err)
	}
	for _, obj := range []client.Object{lease, leftover, edge} {
		if err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj); !apierrors.IsNotFound(err) {
			t.Errorf("%T %s after deletion: err = %v, want not found", obj, obj.GetName(), err)
		}
	}
	if err := c.Get(ctx, client.ObjectKeyFromObject(elsewhere), elsewhere); err != nil {
		t.Errorf("ClusterGrant of another cluster: %v", err)
	}
	if err := c.Get(ctx, client.ObjectKey{Name: ns}, &corev1.Namespace{}); err != nil {
		t.Errorf("namespace %s: %v, want it left in place", ns, err)
	}
}
