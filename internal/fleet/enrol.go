package fleet

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// AgentName names the agent's ServiceAccount, Role and RoleBinding in its cluster's namespace on the hub.
const AgentName = "protem-agent"

const managedByLabel, managedByProtem = "app.kubernetes.io/managed-by", "protem"

// AgentRules are the agent's permissions on the hub, the same as config/agent/hub/role.yaml.
func AgentRules() []rbacv1.PolicyRule {
	return []rbacv1.PolicyRule{
		{APIGroups: []string{"protem.dev"}, Resources: []string{"clustergrants"}, Verbs: []string{"list"}},
		{APIGroups: []string{"protem.dev"}, Resources: []string{"clustergrants/status"}, Verbs: []string{"update"}},
		{APIGroups: []string{"coordination.k8s.io"}, Resources: []string{"leases"}, Verbs: []string{"get", "create", "update"}},
		{APIGroups: []string{""}, Resources: []string{"serviceaccounts/token"}, Verbs: []string{"create"}, ResourceNames: []string{AgentName}},
	}
}

func agentKey(cluster string) metav1.ObjectMeta {
	return metav1.ObjectMeta{Namespace: ClusterNamespace(cluster), Name: AgentName}
}

func agentMeta(cluster string) metav1.ObjectMeta {
	meta := agentKey(cluster)
	meta.Labels = map[string]string{managedByLabel: managedByProtem}
	return meta
}

// getOrCreate reads current, or creates desired under the same name when there is none.
func getOrCreate(ctx context.Context, c client.Client, current, desired client.Object) (created bool, err error) {
	err = c.Get(ctx, client.ObjectKeyFromObject(desired), current)
	if !apierrors.IsNotFound(err) {
		return false, err
	}
	if err := c.Create(ctx, desired); err != nil && !apierrors.IsAlreadyExists(err) {
		return false, fmt.Errorf("create %T %s: %w", desired, client.ObjectKeyFromObject(desired), err)
	}
	return true, nil
}

// relabel reports whether obj lacked the managed-by label, adding it if so.
func relabel(obj metav1.Object) bool {
	labels := obj.GetLabels()
	if labels[managedByLabel] == managedByProtem {
		return false
	}
	if labels == nil {
		labels = map[string]string{}
	}
	labels[managedByLabel] = managedByProtem
	obj.SetLabels(labels)
	return true
}

// EnsureAgentIdentity creates the agent's ServiceAccount, Role and RoleBinding in the cluster's namespace on
// the hub, and the namespace if it is missing, and puts back whatever has drifted.
func EnsureAgentIdentity(ctx context.Context, c client.Client, cluster string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: ClusterNamespace(cluster)}}
	if _, err := getOrCreate(ctx, c, &corev1.Namespace{}, ns); err != nil {
		return err
	}
	sa := &corev1.ServiceAccount{ObjectMeta: agentMeta(cluster)}
	if _, err := getOrCreate(ctx, c, &corev1.ServiceAccount{}, sa); err != nil {
		return err
	}

	role := &rbacv1.Role{ObjectMeta: agentKey(cluster)}
	created, err := getOrCreate(ctx, c, role, &rbacv1.Role{ObjectMeta: agentMeta(cluster), Rules: AgentRules()})
	if err != nil {
		return err
	}
	if !created && (relabel(role) || !equality.Semantic.DeepEqual(role.Rules, AgentRules())) {
		role.Rules = AgentRules()
		if err := c.Update(ctx, role); err != nil {
			return fmt.Errorf("correct Role %s: %w", client.ObjectKeyFromObject(role), err)
		}
	}

	want := &rbacv1.RoleBinding{
		ObjectMeta: agentMeta(cluster),
		RoleRef:    rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: "Role", Name: AgentName},
		Subjects:   []rbacv1.Subject{{Kind: rbacv1.ServiceAccountKind, Name: AgentName, Namespace: ClusterNamespace(cluster)}},
	}
	binding := &rbacv1.RoleBinding{ObjectMeta: agentKey(cluster)}
	if created, err = getOrCreate(ctx, c, binding, want.DeepCopy()); err != nil || created {
		return err
	}
	if binding.RoleRef != want.RoleRef {
		// roleRef cannot be changed in place.
		if err := c.Delete(ctx, binding, client.Preconditions{UID: &binding.UID}); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("replace RoleBinding %s: %w", client.ObjectKeyFromObject(binding), err)
		}
		if err := c.Create(ctx, want); err != nil {
			return fmt.Errorf("replace RoleBinding %s: %w", client.ObjectKeyFromObject(want), err)
		}
		return nil
	}
	if relabel(binding) || !equality.Semantic.DeepEqual(binding.Subjects, want.Subjects) {
		binding.Subjects = want.Subjects
		if err := c.Update(ctx, binding); err != nil {
			return fmt.Errorf("correct RoleBinding %s: %w", client.ObjectKeyFromObject(binding), err)
		}
	}
	return nil
}

// ReplaceAgentAccount deletes and recreates the agent's ServiceAccount, which invalidates every token issued
// for it, so the join token and the running agent are cut off together.
func ReplaceAgentAccount(ctx context.Context, c client.Client, cluster string) error {
	if err := c.Delete(ctx, &corev1.ServiceAccount{ObjectMeta: agentKey(cluster)}); client.IgnoreNotFound(err) != nil {
		return fmt.Errorf("delete ServiceAccount %s/%s: %w", ClusterNamespace(cluster), AgentName, err)
	}
	if err := c.Create(ctx, &corev1.ServiceAccount{ObjectMeta: agentMeta(cluster)}); err != nil {
		return fmt.Errorf("create ServiceAccount %s/%s: %w", ClusterNamespace(cluster), AgentName, err)
	}
	return nil
}

// RemoveAgentIdentity deletes the agent's ServiceAccount, which invalidates all its tokens, and its RoleBinding.
// The Role stays, inert: the operator may not delete Roles.
func RemoveAgentIdentity(ctx context.Context, c client.Client, cluster string) error {
	for _, obj := range []client.Object{
		&corev1.ServiceAccount{ObjectMeta: agentKey(cluster)},
		&rbacv1.RoleBinding{ObjectMeta: agentKey(cluster)},
	} {
		if err := c.Delete(ctx, obj); client.IgnoreNotFound(err) != nil {
			return fmt.Errorf("delete %T %s: %w", obj, client.ObjectKeyFromObject(obj), err)
		}
	}
	return nil
}
