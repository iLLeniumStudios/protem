package fleet

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/yaml"
)

const enrolCluster = "edge-1"

func agentObjects() []client.Object {
	ns := ClusterNamespace(enrolCluster)
	key := func(obj client.Object) client.Object {
		obj.SetNamespace(ns)
		obj.SetName(AgentName)
		return obj
	}
	return []client.Object{key(&corev1.ServiceAccount{}), key(&rbacv1.Role{}), key(&rbacv1.RoleBinding{})}
}

func resourceVersions(t *testing.T, c client.Client) []string {
	t.Helper()
	versions := make([]string, 0, len(agentObjects()))
	for _, obj := range agentObjects() {
		if err := c.Get(context.Background(), client.ObjectKeyFromObject(obj), obj); err != nil {
			t.Fatalf("get %T: %v", obj, err)
		}
		if obj.GetLabels()["app.kubernetes.io/managed-by"] != "protem" {
			t.Errorf("%T labels = %v, want managed-by protem", obj, obj.GetLabels())
		}
		versions = append(versions, obj.GetResourceVersion())
	}
	return versions
}

func TestEnsureAgentIdentity(t *testing.T) {
	ctx := context.Background()
	c := fake.NewClientBuilder().WithScheme(clientgoscheme.Scheme).Build()
	if err := EnsureAgentIdentity(ctx, c, enrolCluster); err != nil {
		t.Fatal(err)
	}
	first := resourceVersions(t, c)

	var binding rbacv1.RoleBinding
	_ = c.Get(ctx, client.ObjectKey{Namespace: ClusterNamespace(enrolCluster), Name: AgentName}, &binding)
	wantSubject := rbacv1.Subject{Kind: rbacv1.ServiceAccountKind, Name: AgentName, Namespace: ClusterNamespace(enrolCluster)}
	if binding.RoleRef != (rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: AgentName}) ||
		len(binding.Subjects) != 1 || binding.Subjects[0] != wantSubject {
		t.Errorf("binding = %+v %+v, want the agent Role bound to the agent ServiceAccount", binding.RoleRef, binding.Subjects)
	}

	if err := EnsureAgentIdentity(ctx, c, enrolCluster); err != nil {
		t.Fatal(err)
	}
	if second := resourceVersions(t, c); !equality.Semantic.DeepEqual(first, second) {
		t.Errorf("second ensure changed objects: %v -> %v", first, second)
	}

	var role rbacv1.Role
	_ = c.Get(ctx, client.ObjectKey{Namespace: ClusterNamespace(enrolCluster), Name: AgentName}, &role)
	role.Rules = append(role.Rules, rbacv1.PolicyRule{APIGroups: []string{""}, Resources: []string{"secrets"}, Verbs: []string{"get"}})
	if err := c.Update(ctx, &role); err != nil {
		t.Fatal(err)
	}
	if err := EnsureAgentIdentity(ctx, c, enrolCluster); err != nil {
		t.Fatal(err)
	}
	_ = c.Get(ctx, client.ObjectKeyFromObject(&role), &role)
	if !equality.Semantic.DeepEqual(role.Rules, AgentRules()) {
		t.Errorf("drifted Role not corrected: %+v", role.Rules)
	}

	if err := RemoveAgentIdentity(ctx, c, enrolCluster); err != nil {
		t.Fatal(err)
	}
	for _, obj := range agentObjects() {
		err := c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		if _, isRole := obj.(*rbacv1.Role); isRole {
			if err != nil {
				t.Errorf("Role after remove: %v, want it left in place", err)
			}
		} else if !apierrors.IsNotFound(err) {
			t.Errorf("%T after remove: err = %v, want not found", obj, err)
		}
	}
	if err := RemoveAgentIdentity(ctx, c, enrolCluster); err != nil {
		t.Errorf("second remove: %v", err)
	}
}

func TestAgentRulesMatchManifest(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "config", "agent", "hub", "role.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var role rbacv1.Role
	if err := yaml.Unmarshal(data, &role); err != nil {
		t.Fatal(err)
	}
	if !equality.Semantic.DeepEqual(role.Rules, AgentRules()) {
		t.Errorf("AgentRules() = %+v, config/agent/hub/role.yaml has %+v", AgentRules(), role.Rules)
	}
}
