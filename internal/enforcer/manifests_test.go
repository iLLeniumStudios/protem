package enforcer

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	coordinationv1 "k8s.io/api/coordination/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/apimachinery/pkg/util/yaml"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const agentSpokeNS = "protem-system"

func envtestBinaryDir() string {
	basePath := filepath.Join("..", "..", "bin", "k8s")
	entries, err := os.ReadDir(basePath)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() {
			return filepath.Join(basePath, entry.Name())
		}
	}
	return ""
}

func startEnvtest(t *testing.T) (*rest.Config, *runtime.Scheme) {
	t.Helper()
	binDir := envtestBinaryDir()
	if binDir == "" && os.Getenv("KUBEBUILDER_ASSETS") == "" {
		t.Skip("envtest binaries not found: set KUBEBUILDER_ASSETS or run 'make setup-envtest'")
	}
	env := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: binDir,
	}
	cfg, err := env.Start()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := env.Stop(); err != nil {
			t.Error(err)
		}
	})

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err := protemv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	return cfg, scheme
}

// applyKustomization creates the resources a kustomization.yaml lists. A non-empty namespace
// is set the way kustomize's `namespace:` field sets it, including ServiceAccount subjects.
func applyKustomization(t *testing.T, ctx context.Context, c client.Client, dir, namespace string) {
	t.Helper()
	var kustomization struct {
		Resources []string `json:"resources"`
	}
	decodeFile(t, filepath.Join(dir, "kustomization.yaml"), func(d *yaml.YAMLOrJSONDecoder) error { return d.Decode(&kustomization) })
	if len(kustomization.Resources) == 0 {
		t.Fatalf("%s lists no resources", dir)
	}
	for _, resource := range kustomization.Resources {
		decodeFile(t, filepath.Join(dir, resource), func(d *yaml.YAMLOrJSONDecoder) error {
			for {
				obj := &unstructured.Unstructured{}
				if err := d.Decode(&obj.Object); err != nil {
					return err
				}
				if len(obj.Object) == 0 {
					continue
				}
				if namespace != "" {
					setNamespace(t, c, obj, namespace)
				}
				if err := c.Create(ctx, obj); err != nil {
					t.Fatalf("create %s %s from %s: %v", obj.GetKind(), obj.GetName(), resource, err)
				}
			}
		})
	}
}

func decodeFile(t *testing.T, path string, decode func(*yaml.YAMLOrJSONDecoder) error) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := decode(yaml.NewYAMLOrJSONDecoder(bytes.NewReader(data), 4096)); err != nil && !errors.Is(err, io.EOF) {
		t.Fatalf("decode %s: %v", path, err)
	}
}

func setNamespace(t *testing.T, c client.Client, obj *unstructured.Unstructured, namespace string) {
	t.Helper()
	namespaced, err := c.IsObjectNamespaced(obj)
	if err != nil {
		t.Fatal(err)
	}
	if namespaced {
		obj.SetNamespace(namespace)
	}
	subjects, found, _ := unstructured.NestedSlice(obj.Object, "subjects")
	if !found {
		return
	}
	for _, s := range subjects {
		if subject := s.(map[string]any); subject["kind"] == rbacv1.ServiceAccountKind {
			subject["namespace"] = namespace
		}
	}
	if err := unstructured.SetNestedSlice(obj.Object, subjects, "subjects"); err != nil {
		t.Fatal(err)
	}
}

func serviceAccountClient(t *testing.T, cfg *rest.Config, scheme *runtime.Scheme, namespace string) client.Client {
	t.Helper()
	impersonated := rest.CopyConfig(cfg)
	impersonated.Impersonate = rest.ImpersonationConfig{
		UserName: "system:serviceaccount:" + namespace + ":protem-agent",
		Groups:   []string{"system:serviceaccounts", "system:serviceaccounts:" + namespace, "system:authenticated"},
	}
	c, err := client.New(impersonated, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func manifestGrant(name string, role protemv1alpha1.TargetRoleRef, scope protemv1alpha1.ProfileScope) *protemv1alpha1.ClusterGrant {
	grant := clusterGrant()
	grant.Name = name
	grant.Spec.Role = role
	grant.Spec.Scope = scope
	return grant
}

type manifestEnv struct {
	admin, hub, target                        client.Client
	cfg                                       *rest.Config
	scheme                                    *runtime.Scheme
	agent                                     *Agent
	hubNS, otherNS                            string
	view                                      rbacv1.ClusterRole
	viewDev, viewAll, clusterAdmin, roleGhost *protemv1alpha1.ClusterGrant
}

func TestAgentManifestsAreSufficientAndNoWider(t *testing.T) {
	e := setupManifestEnv(t)
	t.Run("view grants are bound and report the view rules", e.assertViewGrantsBound)
	t.Run("cluster-admin outside the ceiling is forbidden", e.assertOutsideCeilingForbidden)
	t.Run("missing role fails with RoleNotFound", e.assertMissingRoleFails)
	t.Run("lease is created and renewed", e.assertLeaseRenewed)
	t.Run("hub identity is confined to its namespace and status", e.assertHubIdentityConfined)
	t.Run("agent renews its hub token and stores it", e.assertHubTokenRenewed)
	t.Run("hub identity gets tokens only for its own ServiceAccount", e.assertHubTokenOnlyForItsOwnServiceAccount)
	t.Run("target identity reaches only the hub Secret", e.assertTargetIdentityConfinedToHubSecret)
	t.Run("stored hub token is preferred unless expired", e.assertStoredHubTokenPreferredUnlessExpired)
}

func setupManifestEnv(t *testing.T) *manifestEnv {
	t.Helper()
	cfg, scheme := startEnvtest(t)
	ctx := t.Context()
	admin, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		t.Fatal(err)
	}

	const cluster = "spoke-1"
	e := &manifestEnv{admin: admin, cfg: cfg, scheme: scheme, hubNS: fleet.ClusterNamespace(cluster), otherNS: fleet.ClusterNamespace("other")}
	for _, ns := range []string{e.hubNS, e.otherNS, devNs} {
		if err := admin.Create(ctx, &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ns}}); err != nil {
			t.Fatal(err)
		}
	}
	applyKustomization(t, ctx, admin, filepath.Join("..", "..", "config", "agent", "hub"), e.hubNS)
	applyKustomization(t, ctx, admin, filepath.Join("..", "..", "config", "agent", "spoke"), "")
	e.view = viewWithRules(t, ctx, admin)

	e.viewDev = manifestGrant("team-a.view-dev", protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "view"}, protemv1alpha1.ProfileScopeNamespaced)
	e.viewAll = manifestGrant("team-a.view-all", protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "view"}, protemv1alpha1.ProfileScopeCluster)
	e.clusterAdmin = manifestGrant("team-a.admin", protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "cluster-admin"}, protemv1alpha1.ProfileScopeCluster)
	e.roleGhost = manifestGrant("team-a.missing", protemv1alpha1.TargetRoleRef{Kind: "ClusterRole", Name: "does-not-exist"}, protemv1alpha1.ProfileScopeCluster)
	for _, grant := range []*protemv1alpha1.ClusterGrant{e.viewDev, e.viewAll, e.clusterAdmin, e.roleGhost} {
		if err := admin.Create(ctx, grant); err != nil {
			t.Fatal(err)
		}
	}

	e.hub = serviceAccountClient(t, cfg, scheme, e.hubNS)
	e.target = serviceAccountClient(t, cfg, scheme, agentSpokeNS)
	e.agent = NewAgent(AgentConfig{ClusterName: cluster, HubClient: e.hub, SpokeClient: e.target})
	for range 2 {
		if err := e.agent.SyncOnce(ctx); err != nil {
			t.Fatalf("SyncOnce: %v", err)
		}
	}
	if err := e.agent.sweepExpired(ctx, time.Now()); err != nil {
		t.Fatalf("sweepExpired: %v", err)
	}
	return e
}

func viewWithRules(t *testing.T, ctx context.Context, admin client.Client) rbacv1.ClusterRole {
	t.Helper()
	var view rbacv1.ClusterRole
	if err := wait.PollUntilContextTimeout(ctx, 100*time.Millisecond, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		return admin.Get(ctx, client.ObjectKey{Name: "view"}, &view) == nil, nil
	}); err != nil {
		t.Fatalf("ClusterRole view never appeared: %v", err)
	}
	if len(view.Rules) == 0 {
		// envtest runs no kube-controller-manager, so nothing aggregates rules into view.
		view.Rules = viewRules
		if err := admin.Update(ctx, &view); err != nil {
			t.Fatal(err)
		}
	}
	return view
}

func (e *manifestEnv) status(t *testing.T, grant *protemv1alpha1.ClusterGrant) protemv1alpha1.ClusterGrantStatus {
	t.Helper()
	var got protemv1alpha1.ClusterGrant
	if err := e.admin.Get(t.Context(), client.ObjectKeyFromObject(grant), &got); err != nil {
		t.Fatal(err)
	}
	return got.Status
}

func (e *manifestEnv) bindingExists(t *testing.T, obj client.Object, namespace, name string) bool {
	t.Helper()
	err := e.admin.Get(t.Context(), client.ObjectKey{Namespace: namespace, Name: name}, obj)
	if err != nil && !apierrors.IsNotFound(err) {
		t.Fatal(err)
	}
	return err == nil
}

func (e *manifestEnv) assertViewGrantsBound(t *testing.T) {
	for _, grant := range []*protemv1alpha1.ClusterGrant{e.viewDev, e.viewAll} {
		if s := e.status(t, grant); s.Phase != protemv1alpha1.ClusterGrantPhaseActive || !equality.Semantic.DeepEqual(s.Rules, e.view.Rules) {
			t.Errorf("%s status = %+v, want Active with the view rules %v", grant.Name, s, e.view.Rules)
		}
	}
	if !e.bindingExists(t, &rbacv1.RoleBinding{}, devNs, "protem-grant-team-a.view-dev") {
		t.Error("RoleBinding for the namespaced view grant is missing")
	}
	if !e.bindingExists(t, &rbacv1.ClusterRoleBinding{}, "", "protem-grant-team-a.view-all") {
		t.Error("ClusterRoleBinding for the cluster view grant is missing")
	}
}

func (e *manifestEnv) assertOutsideCeilingForbidden(t *testing.T) {
	if s := e.status(t, e.clusterAdmin); s.Phase != protemv1alpha1.ClusterGrantPhaseFailed || !strings.Contains(s.Message, "is forbidden") || !strings.Contains(s.Message, "attempting to grant RBAC permissions not currently held") {
		t.Errorf("cluster-admin grant status = %+v, want Failed with the API server's forbidden error", s)
	}
	if e.bindingExists(t, &rbacv1.ClusterRoleBinding{}, "", "protem-grant-team-a.admin") {
		t.Error("cluster-admin was bound although it is outside the ceiling")
	}
}

func (e *manifestEnv) assertMissingRoleFails(t *testing.T) {
	if s := e.status(t, e.roleGhost); s.Phase != protemv1alpha1.ClusterGrantPhaseFailed || !strings.HasPrefix(s.Message, "RoleNotFound:") {
		t.Errorf("missing-role grant status = %+v, want Failed with RoleNotFound", s)
	}
	if e.bindingExists(t, &rbacv1.ClusterRoleBinding{}, "", "protem-grant-team-a.missing") {
		t.Error("a binding exists for a role that does not exist")
	}
}

func (e *manifestEnv) assertLeaseRenewed(t *testing.T) {
	ctx := t.Context()
	leaseKey := client.ObjectKey{Namespace: e.hubNS, Name: fleet.AgentLeaseName}
	var lease coordinationv1.Lease
	if err := e.agent.renewLease(ctx); err != nil {
		t.Fatalf("create Lease: %v", err)
	}
	if err := e.admin.Get(ctx, leaseKey, &lease); err != nil {
		t.Fatal(err)
	}
	created := lease.Spec.RenewTime.Time
	time.Sleep(10 * time.Millisecond)
	if err := e.agent.renewLease(ctx); err != nil {
		t.Fatalf("renew Lease: %v", err)
	}
	if err := e.admin.Get(ctx, leaseKey, &lease); err != nil {
		t.Fatal(err)
	}
	if !lease.Spec.RenewTime.After(created) {
		t.Errorf("Lease renewTime %v not after %v", lease.Spec.RenewTime, created)
	}
	if got := lease.Annotations[fleet.AllowedRolesAnnotation]; got != shippedCeiling {
		t.Errorf("Lease allowed roles = %q, want %s from the shipped ClusterRole", got, shippedCeiling)
	}
	if got, ok := lease.Annotations[fleet.AllowedRolesPartialAnnotation]; ok {
		t.Errorf("Lease marks the allowed roles partial (%q) under the shipped ClusterRole", got)
	}
}

func (e *manifestEnv) assertHubIdentityConfined(t *testing.T) {
	ctx := t.Context()
	if err := e.hub.List(ctx, &protemv1alpha1.ClusterGrantList{}, client.InNamespace(e.otherNS)); !apierrors.IsForbidden(err) {
		t.Errorf("hub identity listing ClusterGrants in %s: err = %v, want forbidden", e.otherNS, err)
	}
	if err := e.hub.List(ctx, &protemv1alpha1.AccessRequestList{}, client.InNamespace(e.hubNS)); !apierrors.IsForbidden(err) {
		t.Errorf("hub identity listing AccessRequests: err = %v, want forbidden", err)
	}
	var grant protemv1alpha1.ClusterGrant
	if err := e.admin.Get(ctx, client.ObjectKeyFromObject(e.viewDev), &grant); err != nil {
		t.Fatal(err)
	}
	grant.Spec.Role.Name = "edit"
	if err := e.hub.Update(ctx, &grant); !apierrors.IsForbidden(err) {
		t.Errorf("hub identity updating a ClusterGrant spec: err = %v, want forbidden", err)
	}
}
