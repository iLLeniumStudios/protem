package api

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"strings"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	riskAllVerbs        = "Allows every verb on the listed resources."
	riskAllResources    = "Applies to every resource in the listed API groups."
	riskSecrets         = "Can read Secrets."
	riskPodAccess       = "Can open a shell or a network tunnel into pods."
	riskKubelet         = "Can reach the kubelet API on nodes."
	riskTokens          = "Can mint service account tokens."
	riskImpersonate     = "Can impersonate other users, groups or service accounts."
	riskRBAC            = "Can change RBAC, which allows granting itself more access."
	riskNonResourceURLs = "Allows every non-resource URL."
)

func containsAny(have []string, want ...string) bool {
	return slices.ContainsFunc(have, func(s string) bool { return slices.Contains(want, s) })
}

func ruleRisks(rule rbacv1.PolicyRule) []string {
	resources := func(rs ...string) bool { return containsAny(rule.Resources, rs...) }
	verbs := func(vs ...string) bool { return containsAny(rule.Verbs, vs...) }
	var risks []string
	add := func(matched bool, risk string) {
		if matched {
			risks = append(risks, risk)
		}
	}
	add(verbs("*"), riskAllVerbs)
	add(resources("*"), riskAllResources)
	add(resources("secrets", "*") && verbs("get", "list", "watch", "*"), riskSecrets)
	add(resources("pods/exec", "pods/attach", "pods/portforward") && verbs("create", "get", "*"), riskPodAccess)
	add(resources("nodes/proxy"), riskKubelet)
	add(resources("serviceaccounts/token") && verbs("create", "*"), riskTokens)
	add(verbs("impersonate"), riskImpersonate)
	add(verbs("bind", "escalate") ||
		(verbs("create", "update", "patch", "delete", "*") &&
			resources("roles", "clusterroles", "rolebindings", "clusterrolebindings") &&
			containsAny(rule.APIGroups, rbacv1.GroupName, "*")), riskRBAC)
	add(containsAny(rule.NonResourceURLs, "*"), riskNonResourceURLs)
	return risks
}

func toPermissionRules(rules []rbacv1.PolicyRule) []PermissionRuleView {
	views := make([]PermissionRuleView, 0, len(rules))
	for _, r := range rules {
		views = append(views, PermissionRuleView{
			APIGroups:       r.APIGroups,
			Resources:       r.Resources,
			ResourceNames:   r.ResourceNames,
			NonResourceURLs: r.NonResourceURLs,
			Verbs:           r.Verbs,
			Risks:           ruleRisks(r),
		})
	}
	return views
}

func managedBy(obj metav1.Object) string {
	labels, annotations := obj.GetLabels(), obj.GetAnnotations()
	has := func(m map[string]string, key string) bool { _, ok := m[key]; return ok }
	switch {
	case has(annotations, "argocd.argoproj.io/tracking-id") || has(labels, "argocd.argoproj.io/instance"):
		return "Argo CD"
	case has(labels, "kustomize.toolkit.fluxcd.io/name") || has(labels, "helm.toolkit.fluxcd.io/name"):
		return "Flux"
	case labels["app.kubernetes.io/managed-by"] == "Helm":
		return "Helm"
	}
	return labels["protem.dev/managed-by"]
}

func (s *Server) hubPermissions(ctx context.Context, role protemv1alpha1.TargetRoleRef, namespaces []string) (PermissionsView, error) {
	view := PermissionsView{Role: RoleView(role), Source: "hub", Rules: []PermissionRuleView{}}
	var rules []rbacv1.PolicyRule
	fetched := false
	var err error
	switch {
	case role.Kind == "ClusterRole":
		var cr rbacv1.ClusterRole
		err = s.Client.Get(ctx, client.ObjectKey{Name: role.Name}, &cr)
		rules, fetched = cr.Rules, err == nil
	case role.Kind == "Role" && len(namespaces) > 0:
		var r rbacv1.Role
		err = s.Client.Get(ctx, client.ObjectKey{Namespace: namespaces[0], Name: role.Name}, &r)
		rules, fetched = r.Rules, err == nil
	}
	if err != nil && !apierrors.IsNotFound(err) {
		return view, err
	}
	if !fetched {
		view.Message = fmt.Sprintf("%s %q is not defined on the hub; it may exist only on the target clusters.", role.Kind, role.Name)
		return view, nil
	}
	view.Found, view.Rules = true, toPermissionRules(rules)
	return view, nil
}

// flaggedRules returns a counter of the flagged rules of a request's role as the hub defines it,
// reading each distinct role once. A role the hub cannot read counts 0.
func (s *Server) flaggedRules(ctx context.Context) func(*protemv1alpha1.AccessRequest) int {
	type roleKey struct{ kind, name, namespaces string }
	counts := map[roleKey]int{}
	return func(req *protemv1alpha1.AccessRequest) int {
		var role protemv1alpha1.TargetRoleRef
		var namespaces []string
		if r := req.Status.Resolved; r != nil && r.Role != nil {
			role, namespaces = *r.Role, r.Namespaces
		} else {
			var p protemv1alpha1.AccessProfile
			if err := s.Client.Get(ctx, client.ObjectKey{Namespace: req.Namespace, Name: req.Spec.ProfileRef}, &p); err != nil {
				return 0
			}
			role, namespaces = p.Spec.Role, p.Spec.Namespaces
		}
		key := roleKey{kind: role.Kind, name: role.Name}
		if role.Kind == "Role" {
			key.namespaces = strings.Join(namespaces, ",")
		}
		n, seen := counts[key]
		if !seen {
			view, _ := s.hubPermissions(ctx, role, namespaces)
			for _, r := range view.Rules {
				if len(r.Risks) > 0 {
					n++
				}
			}
			counts[key] = n
		}
		return n
	}
}

func (s *Server) handleProfileDetail(w http.ResponseWriter, r *http.Request, user UserInfo) {
	var p protemv1alpha1.AccessProfile
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: r.PathValue("ns"), Name: r.PathValue("name")}, &p); err != nil {
		s.writeKubeError(w, err)
		return
	}
	s.writeProfileDetail(r.Context(), w, http.StatusOK, &p, user)
}

func (s *Server) writeProfileDetail(ctx context.Context, w http.ResponseWriter, status int, p *protemv1alpha1.AccessProfile, user UserInfo) {
	var spokes protemv1alpha1.TargetClusterList
	if err := s.Client.List(ctx, &spokes); err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	view, err := toProfileDetailView(p, toPerson(user), spokes.Items)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	s.writeJSON(w, status, view)
}

func (s *Server) handleProfilePermissions(w http.ResponseWriter, r *http.Request, _ UserInfo) {
	var p protemv1alpha1.AccessProfile
	if err := s.Client.Get(r.Context(), client.ObjectKey{Namespace: r.PathValue("ns"), Name: r.PathValue("name")}, &p); err != nil {
		s.writeKubeError(w, err)
		return
	}
	view, err := s.hubPermissions(r.Context(), p.Spec.Role, p.Spec.Namespaces)
	if err != nil {
		s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
		return
	}
	s.writeJSON(w, http.StatusOK, view)
}

func (s *Server) handleGrantPermissions(w http.ResponseWriter, r *http.Request, user UserInfo) {
	ctx := r.Context()
	var grant protemv1alpha1.AccessGrant
	if err := s.Client.Get(ctx, client.ObjectKey{Namespace: r.PathValue("ns"), Name: r.PathValue("name")}, &grant); err != nil {
		s.writeKubeError(w, err)
		return
	}
	if !s.mayReadGrant(ctx, w, user, &grant) {
		return
	}

	views := make([]ClusterPermissionsView, 0, len(grant.Spec.Clusters))
	for _, cluster := range grant.Spec.Clusters {
		view := ClusterPermissionsView{Cluster: cluster}
		var cg protemv1alpha1.ClusterGrant
		err := s.Client.Get(ctx, client.ObjectKey{Namespace: fleet.ClusterNamespace(cluster), Name: fleet.GrantKey(grant.Namespace, grant.Name)}, &cg)
		switch {
		case err == nil:
			view.State, view.Message, view.Subject = string(cg.Status.Phase), cg.Status.Message, cg.Spec.Subject
			for _, b := range cg.Status.Bindings {
				view.Bindings = append(view.Bindings, BindingView(b))
			}
			view.Permissions = PermissionsView{
				Role:   RoleView(grant.Spec.Role),
				Source: "cluster",
				Found:  len(cg.Status.Rules) > 0,
				Rules:  toPermissionRules(cg.Status.Rules),
			}
			if !view.Permissions.Found {
				view.Permissions.Message = "The cluster has not reported the rules of this role."
			}
		case apierrors.IsNotFound(err):
			for _, c := range grant.Status.Clusters {
				if c.ClusterName == cluster {
					view.State, view.Message = string(c.State), c.Message
				}
			}
			if view.Permissions, err = s.hubPermissions(ctx, grant.Spec.Role, grant.Spec.Namespaces); err != nil {
				s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
				return
			}
		default:
			s.writeJSON(w, http.StatusInternalServerError, map[string]string{errorKey: err.Error()})
			return
		}
		views = append(views, view)
	}
	s.writeJSON(w, http.StatusOK, views)
}
