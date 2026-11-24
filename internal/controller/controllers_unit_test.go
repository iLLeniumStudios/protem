package controller

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/binding"
)

const (
	clusterRoleKind      = "ClusterRole"
	roleKind             = "Role"
	adminName            = "admin"
	defaultNs            = "default"
	productionNs         = "production"
	hubClusterName       = "hub-cluster"
	devsGroup            = "devs"
	sreLeadsGroup        = "sre-leads"
	aliceUsername        = "alice"
	aliceEmail           = "alice@company.com"
	rbacAPIGroup         = "rbac.authorization.k8s.io"
	reqOne               = "req-1"
	profOne              = "prof-1"
	testProfileReq       = "test-profile-req"
	testCluster          = "test-cluster"
	manualRule           = "manual-rule"
	grantTestOne         = "grant-test-1"
	grantExpiredOne      = "grant-expired-1"
	viewRole             = "view"
	grantDrift           = "grant-drift"
	reqEscalateNs        = "req-escalate-ns"
	grantNamespacedScope = "grant-namespaced-scope"
)

func setupTestScheme(t *testing.T) *runtime.Scheme {
	s := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(s); err != nil {
		t.Fatalf("failed to add clientgo to scheme: %v", err)
	}
	if err := protemv1alpha1.AddToScheme(s); err != nil {
		t.Fatalf("failed to add protemv1alpha1 to scheme: %v", err)
	}
	return s
}

func makeReadyCluster(name string) *protemv1alpha1.TargetCluster {
	return &protemv1alpha1.TargetCluster{
		ObjectMeta: metav1.ObjectMeta{Name: name},
		Spec:       protemv1alpha1.TargetClusterSpec{DisplayName: name},
		Status: protemv1alpha1.TargetClusterStatus{
			Conditions: []metav1.Condition{
				{Type: readyConditionType, Status: metav1.ConditionTrue},
			},
		},
	}
}

func newRequestReconciler(t *testing.T, funcs interceptor.Funcs, objs ...client.Object) (*AccessRequestReconciler, client.Client) {
	scheme := setupTestScheme(t)
	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		WithStatusSubresource(&protemv1alpha1.AccessRequest{}, &protemv1alpha1.AccessGrant{}).
		WithInterceptorFuncs(funcs).
		Build()
	return &AccessRequestReconciler{Client: c, Scheme: scheme}, c
}

func reconcileRequest(t *testing.T, r *AccessRequestReconciler, c client.Client) (protemv1alpha1.AccessRequest, error) {
	t.Helper()
	key := client.ObjectKey{Namespace: defaultNs, Name: reqOne}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: key})
	var got protemv1alpha1.AccessRequest
	if getErr := c.Get(context.Background(), key, &got); getErr != nil {
		t.Fatalf("failed to get request: %v", getErr)
	}
	return got, err
}

func TestAccessRequestReconciler_MissingProfileWritesRejection(t *testing.T) {
	r, c := newRequestReconciler(t, interceptor.Funcs{}, lifecycleRequest())

	got, err := reconcileRequest(t, r, c)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if got.Status.Phase != protemv1alpha1.RequestPhaseRejected {
		t.Fatalf("expected persisted phase Rejected, got %q", got.Status.Phase)
	}
}

func TestAccessRequestReconciler_CreatesOwnedGrantAndRecordsIt(t *testing.T) {
	r, c := newRequestReconciler(t, interceptor.Funcs{}, lifecycleProfile(), approvedRequest())

	got, err := reconcileRequest(t, r, c)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if got.Status.GrantRef != req1Grant {
		t.Fatalf("expected GrantRef grant-req-1, got %q", got.Status.GrantRef)
	}
	var grant protemv1alpha1.AccessGrant
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: defaultNs, Name: req1Grant}, &grant); err != nil {
		t.Fatalf("expected grant-req-1 to exist: %v", err)
	}
	if owner := metav1.GetControllerOf(&grant); owner == nil || owner.Kind != "AccessRequest" || owner.Name != reqOne {
		t.Fatalf("expected grant controlled by AccessRequest req-1, got %+v", grant.OwnerReferences)
	}
}

func TestAccessRequestReconciler_ExistingGrantIsStillRecorded(t *testing.T) {
	req := approvedRequest()
	req.UID = "req-1-uid"
	existing := &protemv1alpha1.AccessGrant{ObjectMeta: metav1.ObjectMeta{Name: req1Grant, Namespace: defaultNs}}
	if err := controllerutil.SetControllerReference(req, existing, setupTestScheme(t)); err != nil {
		t.Fatal(err)
	}
	r, c := newRequestReconciler(t, interceptor.Funcs{}, lifecycleProfile(), req, existing)

	got, err := reconcileRequest(t, r, c)
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}
	if got.Status.GrantRef != req1Grant {
		t.Fatalf("expected GrantRef grant-req-1, got %q", got.Status.GrantRef)
	}
}

func TestAccessRequestReconciler_ForeignExistingGrantIsNotRecorded(t *testing.T) {
	foreign := &protemv1alpha1.AccessGrant{ObjectMeta: metav1.ObjectMeta{Name: req1Grant, Namespace: defaultNs}}
	r, c := newRequestReconciler(t, interceptor.Funcs{}, lifecycleProfile(), approvedRequest(), foreign)

	got, err := reconcileRequest(t, r, c)
	if err == nil {
		t.Fatal("expected an error for a grant-req-1 not controlled by the request")
	}
	if got.Status.GrantRef != "" {
		t.Fatalf("expected no GrantRef, got %q", got.Status.GrantRef)
	}
}

func TestAccessRequestReconciler_ClusterListErrorIsReturned(t *testing.T) {
	listErr := errors.New("apiserver unavailable")
	funcs := interceptor.Funcs{List: func(context.Context, client.WithWatch, client.ObjectList, ...client.ListOption) error { return listErr }}
	r, c := newRequestReconciler(t, funcs, lifecycleProfile(), lifecycleRequest(), makeReadyCluster(hubClusterName))

	got, err := reconcileRequest(t, r, c)
	if !errors.Is(err, listErr) {
		t.Fatalf("expected list error, got %v", err)
	}
	if got.Status.Phase != "" {
		t.Fatalf("expected no status write, got phase %q", got.Status.Phase)
	}
}

func TestAccessGrantReconciler_Lifecycle(t *testing.T) {
	scheme := setupTestScheme(t)

	expiresAt := metav1.NewTime(time.Now().Add(1 * time.Hour))
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      grantTestOne,
			Namespace: defaultNs,
		},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester: protemv1alpha1.RequesterInfo{
				Username: aliceUsername,
				Email:    aliceEmail,
			},
			Role: protemv1alpha1.TargetRoleRef{
				Kind: roleKind,
				Name: adminName,
			},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  expiresAt,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant).
		WithStatusSubresource(grant).
		Build()

	reconciler := &AccessGrantReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ctx := context.Background()

	// Step 1: Adds finalizer
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantTestOne}})
	if err != nil {
		t.Fatalf("first reconcile failed: %v", err)
	}

	// Step 2: Creates RoleBinding in "production"
	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantTestOne}})
	if err != nil {
		t.Fatalf("second reconcile failed: %v", err)
	}

	var rb rbacv1.RoleBinding
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: productionNs, Name: "protem-grant-default.grant-test-1"}, &rb); err != nil {
		t.Fatalf("expected RoleBinding to be created: %v", err)
	}

	if len(rb.Subjects) != 1 || rb.Subjects[0].Name != aliceEmail {
		t.Fatalf("expected subject alice@company.com, got %v", rb.Subjects)
	}
	if rb.RoleRef.Name != adminName {
		t.Fatalf("expected role admin, got %q", rb.RoleRef.Name)
	}

	var updatedGrant protemv1alpha1.AccessGrant
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: defaultNs, Name: grantTestOne}, &updatedGrant); err != nil {
		t.Fatalf("failed to get grant: %v", err)
	}
	if updatedGrant.Status.Phase != protemv1alpha1.GrantPhaseActive {
		t.Fatalf("expected status Active, got %s", updatedGrant.Status.Phase)
	}

	// Step 3: Test Revocation
	updatedGrant.Spec.Revocation = &protemv1alpha1.RevocationDetails{
		By:     "bob@company.com",
		At:     metav1.Now(),
		Reason: "Triage complete",
	}
	if err := fakeClient.Update(ctx, &updatedGrant); err != nil {
		t.Fatalf("failed to mark grant revoked: %v", err)
	}

	_, err = reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantTestOne}})
	if err != nil {
		t.Fatalf("reconcile after revocation failed: %v", err)
	}

	// RoleBinding must be deleted
	err = fakeClient.Get(ctx, client.ObjectKey{Namespace: productionNs, Name: "protem-grant-default.grant-test-1"}, &rb)
	if err == nil {
		t.Fatalf("expected RoleBinding to be deleted after revocation, but it still exists")
	}

	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: defaultNs, Name: grantTestOne}, &updatedGrant); err != nil {
		t.Fatalf("failed to get grant: %v", err)
	}
	if updatedGrant.Status.Phase != protemv1alpha1.GrantPhaseRevoked {
		t.Fatalf("expected status Revoked, got %s", updatedGrant.Status.Phase)
	}
}

func TestAccessGrantReconciler_Expiration(t *testing.T) {
	scheme := setupTestScheme(t)

	// Grant that expired 10 minutes ago
	pastExpiry := metav1.NewTime(time.Now().Add(-10 * time.Minute))
	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:       grantExpiredOne,
			Namespace:  defaultNs,
			Finalizers: []string{protemFinalizer},
		},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: "req-exp-1",
			ProfileRef: profOne,
			Requester: protemv1alpha1.RequesterInfo{
				Username: aliceUsername,
				Email:    aliceEmail,
			},
			Role: protemv1alpha1.TargetRoleRef{
				Kind: roleKind,
				Name: viewRole,
			},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  pastExpiry,
		},
		Status: protemv1alpha1.AccessGrantStatus{
			Phase: protemv1alpha1.GrantPhaseActive,
		},
	}

	rb := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protem-grant-default.grant-expired-1",
			Namespace: productionNs,
			Labels: map[string]string{
				binding.LabelManagedBy:      "protem",
				binding.LabelGrant:          grantExpiredOne,
				binding.LabelGrantNamespace: defaultNs,
			},
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: rbacAPIGroup,
			Kind:     roleKind,
			Name:     viewRole,
		},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.UserKind, Name: aliceEmail},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(grant, rb).
		WithStatusSubresource(grant).
		Build()

	reconciler := &AccessGrantReconciler{
		Client: fakeClient,
		Scheme: scheme,
	}

	ctx := context.Background()

	// Reconcile -> detects expiration, deletes RoleBinding, sets status Expired
	_, err := reconciler.Reconcile(ctx, ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantExpiredOne}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var fetchedRB rbacv1.RoleBinding
	err = fakeClient.Get(ctx, client.ObjectKey{Namespace: productionNs, Name: "protem-grant-default.grant-expired-1"}, &fetchedRB)
	if err == nil {
		t.Fatalf("expected RoleBinding to be deleted after expiration")
	}

	var updatedGrant protemv1alpha1.AccessGrant
	if err := fakeClient.Get(ctx, client.ObjectKey{Namespace: defaultNs, Name: grantExpiredOne}, &updatedGrant); err != nil {
		t.Fatalf("failed to get grant: %v", err)
	}

	if updatedGrant.Status.Phase != protemv1alpha1.GrantPhaseExpired {
		t.Fatalf("expected status Expired, got %s", updatedGrant.Status.Phase)
	}
}

func TestAccessGrantReconciler_RefusesToAdoptUnmanagedBinding(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-unmanaged", Namespace: defaultNs, Finalizers: []string{protemFinalizer}},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
	}

	// Pre-existing unmanaged RoleBinding without grant label!
	unmanagedRB := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protem-grant-default.grant-unmanaged",
			Namespace: productionNs,
			// NO labels!
		},
		RoleRef:  rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: roleKind, Name: adminName},
		Subjects: []rbacv1.Subject{{Kind: rbacv1.UserKind, Name: "intruder@evil.com"}},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant, unmanagedRB).WithStatusSubresource(grant).Build()
	reconciler := &AccessGrantReconciler{Client: fakeClient, Scheme: scheme}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: "grant-unmanaged"}})
	if err == nil {
		t.Fatalf("expected error refusing to adopt unmanaged RoleBinding, got nil")
	}
}

func TestAccessGrantReconciler_CorrectsDrift(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: grantDrift, Namespace: defaultNs, Finalizers: []string{protemFinalizer}},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
	}

	// Existing managed RoleBinding has drifted with extra attacker subject!
	driftedRB := &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "protem-grant-default.grant-drift",
			Namespace: productionNs,
			Labels: map[string]string{
				binding.LabelManagedBy:      "protem",
				binding.LabelGrant:          grantDrift,
				binding.LabelGrantNamespace: defaultNs,
			},
		},
		RoleRef: rbacv1.RoleRef{APIGroup: rbacAPIGroup, Kind: roleKind, Name: adminName},
		Subjects: []rbacv1.Subject{
			{Kind: rbacv1.UserKind, Name: aliceEmail},
			{Kind: rbacv1.UserKind, Name: "attacker@evil.com"}, // Extra drifted subject!
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant, driftedRB).WithStatusSubresource(grant).Build()
	reconciler := &AccessGrantReconciler{Client: fakeClient, Scheme: scheme}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: grantDrift}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var correctedRB rbacv1.RoleBinding
	_ = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: productionNs, Name: "protem-grant-default.grant-drift"}, &correctedRB)
	if len(correctedRB.Subjects) != 1 || correctedRB.Subjects[0].Name != aliceEmail {
		t.Fatalf("expected drift to be corrected to only alice@company.com, got subjects %v", correctedRB.Subjects)
	}
}

func TestAccessGrantReconciler_SubjectTemplate(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-tmpl", Namespace: defaultNs, Finalizers: []string{protemFinalizer}},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant).WithStatusSubresource(grant).Build()
	reconciler := &AccessGrantReconciler{
		Client:          fakeClient,
		Scheme:          scheme,
		SubjectTemplate: "oidc:{{.Email}}", // EKS / OIDC prefix
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: "grant-tmpl"}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var rb rbacv1.RoleBinding
	_ = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: productionNs, Name: "protem-grant-default.grant-tmpl"}, &rb)
	if len(rb.Subjects) != 1 || rb.Subjects[0].Name != "oidc:alice@company.com" {
		t.Fatalf("expected subject oidc:alice@company.com, got %v", rb.Subjects)
	}
}

// C3: Hub must not create local bindings unless the Hub itself is targeted
func TestAccessGrantReconciler_NoHubBindingsUnlessHubTargeted(t *testing.T) {
	scheme := setupTestScheme(t)

	// Grant targeting only remote spoke cluster "prod-spoke-1"
	grantRemote := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-remote", Namespace: defaultNs, Finalizers: []string{protemFinalizer}},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: "req-rem",
			ProfileRef: "prof-rem",
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: clusterRoleKind, Name: "cluster-admin"},
			Scope:      protemv1alpha1.ProfileScopeCluster,
			Clusters:   []string{"prod-spoke-1"}, // Hub is "hub-cluster", NOT targeted!
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grantRemote).WithStatusSubresource(grantRemote).Build()
	reconciler := &AccessGrantReconciler{
		Client:         fakeClient,
		Scheme:         scheme,
		HubClusterName: hubClusterName,
	}

	_, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKey{Namespace: defaultNs, Name: "grant-remote"}})
	if err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	// 1. Verify NO ClusterRoleBinding was created on Hub!
	var crb rbacv1.ClusterRoleBinding
	err = fakeClient.Get(context.Background(), client.ObjectKey{Name: "protem-grant-default.grant-remote"}, &crb)
	if err == nil {
		t.Fatalf("VULNERABILITY C3: Hub created local ClusterRoleBinding for a grant targeting only remote spoke 'prod-spoke-1'!")
	}

	// 2. Verify ClusterGrant was fanned out in spoke namespace protem-cluster-prod-spoke-1
	var cg protemv1alpha1.ClusterGrant
	err = fakeClient.Get(context.Background(), client.ObjectKey{Namespace: "protem-cluster-prod-spoke-1", Name: "default.grant-remote"}, &cg)
	if err != nil {
		t.Fatalf("expected ClusterGrant to be created in namespace protem-cluster-prod-spoke-1: %v", err)
	}
}

func TestAccessGrantReconciler_RefreshesStaleManagedBindingsOfSameLength(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-stale", Namespace: defaultNs, Finalizers: []string{protemFinalizer}},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: roleKind, Name: adminName},
			Scope:      protemv1alpha1.ProfileScopeNamespaced,
			Namespaces: []string{productionNs},
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
		Status: protemv1alpha1.AccessGrantStatus{
			Phase:           protemv1alpha1.GrantPhaseActive,
			ManagedBindings: []protemv1alpha1.ManagedBindingRef{{Kind: "RoleBinding", Namespace: "staging", Name: "protem-grant-default.grant-stale"}},
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant).WithStatusSubresource(grant).Build()
	reconciler := &AccessGrantReconciler{Client: fakeClient, Scheme: scheme}
	key := client.ObjectKey{Namespace: defaultNs, Name: "grant-stale"}

	if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
		t.Fatalf("reconcile failed: %v", err)
	}

	var updated protemv1alpha1.AccessGrant
	if err := fakeClient.Get(context.Background(), key, &updated); err != nil {
		t.Fatalf("failed to get grant: %v", err)
	}
	if len(updated.Status.ManagedBindings) != 1 || updated.Status.ManagedBindings[0].Namespace != productionNs {
		t.Fatalf("expected managed bindings to name production, got %v", updated.Status.ManagedBindings)
	}
}

func TestAccessGrantReconciler_UnchangedClusterStatesDoNotRewriteStatus(t *testing.T) {
	scheme := setupTestScheme(t)

	grant := &protemv1alpha1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{Name: "grant-steady", Namespace: defaultNs, Finalizers: []string{protemFinalizer}},
		Spec: protemv1alpha1.AccessGrantSpec{
			RequestRef: reqOne,
			ProfileRef: profOne,
			Requester:  protemv1alpha1.RequesterInfo{Username: aliceUsername, Email: aliceEmail},
			Role:       protemv1alpha1.TargetRoleRef{Kind: clusterRoleKind, Name: viewRole},
			Scope:      protemv1alpha1.ProfileScopeCluster,
			Clusters:   []string{"prod-spoke-1"},
			ExpiresAt:  metav1.NewTime(time.Now().Add(1 * time.Hour)),
		},
	}

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(grant).WithStatusSubresource(grant).Build()
	reconciler := &AccessGrantReconciler{Client: fakeClient, Scheme: scheme, HubClusterName: hubClusterName}
	key := client.ObjectKey{Namespace: defaultNs, Name: "grant-steady"}

	resourceVersionAfterReconcile := func() string {
		t.Helper()
		if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: key}); err != nil {
			t.Fatalf("reconcile failed: %v", err)
		}
		var g protemv1alpha1.AccessGrant
		if err := fakeClient.Get(context.Background(), key, &g); err != nil {
			t.Fatalf("failed to get grant: %v", err)
		}
		return g.ResourceVersion
	}

	first := resourceVersionAfterReconcile()
	time.Sleep(1100 * time.Millisecond)
	if second := resourceVersionAfterReconcile(); second != first {
		t.Fatalf("status rewritten without a state change: resourceVersion %s -> %s", first, second)
	}
}

func TestAccessGrantReconciler_SameNameGrantsInOtherNamespacesFanOutSeparately(t *testing.T) {
	scheme := setupTestScheme(t)
	grantIn := func(ns, requester string) *protemv1alpha1.AccessGrant {
		return &protemv1alpha1.AccessGrant{
			ObjectMeta: metav1.ObjectMeta{Name: "x", Namespace: ns, Finalizers: []string{protemFinalizer}},
			Spec: protemv1alpha1.AccessGrantSpec{
				Requester: protemv1alpha1.RequesterInfo{Username: requester},
				Role:      protemv1alpha1.TargetRoleRef{Kind: clusterRoleKind, Name: viewRole},
				Scope:     protemv1alpha1.ProfileScopeCluster,
				Clusters:  []string{"c1"},
				ExpiresAt: metav1.NewTime(time.Now().Add(time.Hour)),
			},
		}
	}
	teamA, teamB := grantIn("team-a", aliceUsername), grantIn("team-b", "bob")
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(teamA, teamB).WithStatusSubresource(teamA).Build()
	reconciler := &AccessGrantReconciler{Client: fakeClient, Scheme: scheme, HubClusterName: "hub"}

	for _, g := range []*protemv1alpha1.AccessGrant{teamA, teamB} {
		if _, err := reconciler.Reconcile(context.Background(), ctrl.Request{NamespacedName: client.ObjectKeyFromObject(g)}); err != nil {
			t.Fatalf("reconcile %s: %v", client.ObjectKeyFromObject(g), err)
		}
	}

	var cgs protemv1alpha1.ClusterGrantList
	if err := fakeClient.List(context.Background(), &cgs, client.InNamespace("protem-cluster-c1")); err != nil {
		t.Fatal(err)
	}
	requesters := map[string]string{}
	for _, cg := range cgs.Items {
		requesters[cg.Labels[binding.LabelGrantNamespace]] = cg.Spec.Requester.Username
	}
	if len(cgs.Items) != 2 || requesters["team-a"] != aliceUsername || requesters["team-b"] != "bob" {
		t.Fatalf("got %d ClusterGrants with requesters by grant namespace %v, want team-a:alice and team-b:bob", len(cgs.Items), requesters)
	}
}

func TestGrantForBinding(t *testing.T) {
	grantLabels := map[string]string{binding.LabelGrant: "X", binding.LabelGrantNamespace: "team-a"}
	teamAX := []reconcile.Request{{NamespacedName: types.NamespacedName{Namespace: "team-a", Name: "X"}}}
	for _, tc := range []struct {
		name string
		obj  client.Object
		want []reconcile.Request
	}{
		{"RoleBinding in another namespace", &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "b", Labels: grantLabels}}, teamAX},
		{"ClusterRoleBinding", &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: "b", Labels: grantLabels}}, teamAX},
		{"missing grant namespace label", &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Namespace: "prod", Name: "b", Labels: map[string]string{binding.LabelGrant: "X"}}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := grantForBinding(context.Background(), tc.obj); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("grantForBinding = %v, want %v", got, tc.want)
			}
		})
	}
}
