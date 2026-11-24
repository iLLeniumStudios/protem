package binding

import (
	"context"
	"fmt"
	"strings"
	"time"

	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	LabelManagedBy      = "protem.dev/managed-by"
	LabelGrant          = "protem.dev/grant"
	LabelGrantNamespace = "protem.dev/grant-namespace"
	AnnotationExpiry    = "protem.dev/expires-at"
)

type Desired struct {
	Grant, GrantNamespace, Subject string
	Role                           protemv1alpha1.TargetRoleRef
	Scope                          protemv1alpha1.ProfileScope
	Namespaces                     []string
	ExpiresAt                      time.Time
}

// Apply creates or corrects the bindings of one grant. On failure it still returns the bindings
// already in place, so the caller reports the access that exists, not the access it intended.
func Apply(ctx context.Context, c client.Client, owner string, d Desired) ([]protemv1alpha1.ManagedBindingRef, error) {
	name := "protem-grant-" + fleet.GrantKey(d.GrantNamespace, d.Grant)
	if d.Scope == protemv1alpha1.ProfileScopeCluster {
		crb := &rbacv1.ClusterRoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name}}
		if err := apply(ctx, c, owner, d, "ClusterRoleBinding", crb, &crb.RoleRef, &crb.Subjects); err != nil {
			return nil, err
		}
		return []protemv1alpha1.ManagedBindingRef{{Kind: "ClusterRoleBinding", Name: name}}, nil
	}

	managed := make([]protemv1alpha1.ManagedBindingRef, 0, len(d.Namespaces))
	for _, ns := range d.Namespaces {
		if ns == "" || ns == "*" {
			continue
		}
		rb := &rbacv1.RoleBinding{ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: ns}}
		if err := apply(ctx, c, owner, d, "RoleBinding", rb, &rb.RoleRef, &rb.Subjects); err != nil {
			return managed, err
		}
		managed = append(managed, protemv1alpha1.ManagedBindingRef{Kind: "RoleBinding", Namespace: ns, Name: name})
	}
	return managed, nil
}

func apply(ctx context.Context, c client.Client, owner string, d Desired, kind string, obj client.Object, roleRef *rbacv1.RoleRef, subjects *[]rbacv1.Subject) error {
	op, err := controllerutil.CreateOrUpdate(ctx, c, obj, func() error {
		labels := obj.GetLabels()
		if obj.GetResourceVersion() != "" && (labels[LabelManagedBy] != owner || labels[LabelGrant] != d.Grant || labels[LabelGrantNamespace] != d.GrantNamespace) {
			return fmt.Errorf("refusing to adopt %s %s: managed by %q for grant %q in namespace %q", kind, client.ObjectKeyFromObject(obj), labels[LabelManagedBy], labels[LabelGrant], labels[LabelGrantNamespace])
		}
		if labels == nil {
			labels = map[string]string{}
		}
		labels[LabelManagedBy] = owner
		labels[LabelGrant] = d.Grant
		if d.GrantNamespace != "" {
			labels[LabelGrantNamespace] = d.GrantNamespace
		}
		obj.SetLabels(labels)

		annotations := obj.GetAnnotations()
		if annotations == nil {
			annotations = map[string]string{}
		}
		annotations[AnnotationExpiry] = d.ExpiresAt.UTC().Format(time.RFC3339)
		obj.SetAnnotations(annotations)

		*roleRef = rbacv1.RoleRef{APIGroup: rbacv1.GroupName, Kind: d.Role.Kind, Name: d.Role.Name}
		*subjects = []rbacv1.Subject{{Kind: rbacv1.UserKind, APIGroup: rbacv1.GroupName, Name: d.Subject}}
		return nil
	})
	if err != nil {
		return err
	}
	if op != controllerutil.OperationResultNone {
		logf.FromContext(ctx).Info("Reconciled binding", "kind", kind, "key", client.ObjectKeyFromObject(obj), "subject", d.Subject, "op", op)
	}
	return nil
}

func Remove(ctx context.Context, c client.Client, owner, grantNamespace, grant string) error {
	op, values := selection.Equals, []string{grantNamespace}
	if grantNamespace == "" {
		op, values = selection.DoesNotExist, nil
	}
	inGrantNamespace, err := labels.NewRequirement(LabelGrantNamespace, op, values)
	if err != nil {
		return err
	}
	selector := labels.SelectorFromSet(labels.Set{LabelManagedBy: owner, LabelGrant: grant}).Add(*inGrantNamespace)
	return deleteUnkept(ctx, c, client.MatchingLabelsSelector{Selector: selector}, func(string, time.Time) bool { return false })
}

func Sweep(ctx context.Context, c client.Client, owner string, keep func(grant string, expiresAt time.Time) bool) error {
	return deleteUnkept(ctx, c, client.MatchingLabels{LabelManagedBy: owner}, keep)
}

func deleteUnkept(ctx context.Context, c client.Client, selector client.ListOption, keep func(grant string, expiresAt time.Time) bool) error {
	var rbs rbacv1.RoleBindingList
	if err := c.List(ctx, &rbs, selector); err != nil {
		return err
	}
	var crbs rbacv1.ClusterRoleBindingList
	if err := c.List(ctx, &crbs, selector); err != nil {
		return err
	}
	objs := make([]client.Object, 0, len(rbs.Items)+len(crbs.Items))
	for i := range rbs.Items {
		objs = append(objs, &rbs.Items[i])
	}
	for i := range crbs.Items {
		objs = append(objs, &crbs.Items[i])
	}

	var firstErr error
	for _, obj := range objs {
		expiresAt, _ := time.Parse(time.RFC3339, obj.GetAnnotations()[AnnotationExpiry])
		if keep(obj.GetLabels()[LabelGrant], expiresAt) {
			continue
		}
		logf.FromContext(ctx).Info("Deleting binding", "type", fmt.Sprintf("%T", obj), "key", client.ObjectKeyFromObject(obj), "grant", obj.GetLabels()[LabelGrant])
		if err := c.Delete(ctx, obj); err != nil && !apierrors.IsNotFound(err) && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func FormatSubject(tmpl string, r protemv1alpha1.RequesterInfo) string {
	if tmpl == "" {
		tmpl = "{{.Email}}"
	}
	res := strings.ReplaceAll(tmpl, "{{.Email}}", r.Email)
	res = strings.ReplaceAll(res, "{{.Username}}", r.Username)
	if res == "" || res == tmpl {
		if r.Email != "" {
			return r.Email
		}
		return r.Username
	}
	return res
}
