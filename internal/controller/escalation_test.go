package controller

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// 3.4: Request for namespaces outside the profile must be rejected by controller
func TestEscalation_3_4_NamespaceEscalationRejected(t *testing.T) {
	scheme := setupTestScheme(t)
	cluster := makeReadyCluster(hubClusterName)

	profile := &protemv1alpha1.AccessProfile{
		ObjectMeta: metav1.ObjectMeta{Name: "payments-only", Namespace: defaultNs},
		Spec: protemv1alpha1.AccessProfileSpec{
			DisplayName: "Payments Only",
			Scope:       protemv1alpha1.ProfileScopeNamespaced,
			Role:        protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Namespaces:  []string{"payments"}, // ONLY payments namespace!
			Clusters:    protemv1alpha1.ClusterTargeting{Names: []string{hubClusterName}},
			Duration:    protemv1alpha1.ProfileDurationConfig{Default: metav1.Duration{Duration: 1 * time.Hour}, Max: metav1.Duration{Duration: 2 * time.Hour}},
			Requesters:  protemv1alpha1.ProfileRequesters{Groups: []string{devsGroup}},
			Approval:    protemv1alpha1.ProfileApprovalConfig{Rules: []protemv1alpha1.ApprovalRule{{Name: "rule", Mode: protemv1alpha1.ApprovalModeAuto}}},
		},
	}

	// Attacker crafts request asking for kube-system!
	request := &protemv1alpha1.AccessRequest{
		ObjectMeta: metav1.ObjectMeta{Name: reqEscalateNs, Namespace: defaultNs},
		Spec: protemv1alpha1.AccessRequestSpec{
			ProfileRef: "payments-only",
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail, Groups: []string{devsGroup}},
			Namespaces: []string{"kube-system"}, // Unauthorized namespace escalation!
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cluster, profile, request).
		WithStatusSubresource(request).
		Build()

	reconciler := &AccessRequestReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ctx := context.Background()
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: reqEscalateNs}})
	if err != nil {
		t.Fatalf("reconcile error: %v", err)
	}

	var updatedReq protemv1alpha1.AccessRequest
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: defaultNs, Name: reqEscalateNs}, &updatedReq); err != nil {
		t.Fatalf("failed to get request: %v", err)
	}

	if updatedReq.Status.Phase != protemv1alpha1.RequestPhaseRejected {
		t.Fatalf("VULNERABILITY §3.4: Expected request with unauthorized namespace to be Rejected, got %q", updatedReq.Status.Phase)
	}
}

// 3.4: Namespaced scope must NEVER create a ClusterRoleBinding, even if namespaces is empty or contains "*"
func TestEscalation_3_4_ClusterRoleBindingNeverCreatedForNamespacedScope(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: grantNamespacedScope, Namespace: defaultNs},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: clusterRoleKind, Name: adminName},
			Scope:      protemv1alpha1.ProfileScopeNamespaced, // Explicitly Namespaced!
			Namespaces: []string{"*"},                         // Sneaky "*" attempt!
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant).WithStatusSubresource(grant).Build()
	reconciler := &AccessGrantReconciler{Client: fakeClient, Scheme: scheme}
	ctx := context.Background()

	// Reconcile finalizer
	_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantNamespacedScope}})
	// Reconcile bindings
	_, _ = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantNamespacedScope}})

	var updatedGrant protemv1alpha1.AccessGrant
	_ = fakeClient.Get(ctx, client.ObjectKey{Namespace: defaultNs, Name: grantNamespacedScope}, &updatedGrant)

	for _, b := range updatedGrant.Status.ManagedBindings {
		if b.Kind == "ClusterRoleBinding" {
			t.Fatalf("VULNERABILITY §3.4: Created ClusterRoleBinding for profile with Scope=Namespaced!")
		}
	}
}
