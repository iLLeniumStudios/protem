package enforcer

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	authorizationv1 "k8s.io/api/authorization/v1"
	coordinationv1 "k8s.io/api/coordination/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/equality"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/binding"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

var enforcerLog = logf.Log.WithName("protem-agent")

const bindingOwner = "protem-agent"

// AgentConfig configures the spoke cluster enforcer.
type AgentConfig struct {
	ClusterName     string
	HubClient       client.Client
	SpokeClient     client.Client
	SubjectTemplate string
	SyncInterval    time.Duration
}

// Agent reconciles ClusterGrants from the Hub onto the local spoke cluster.
type Agent struct {
	Config AgentConfig

	// The allowed-roles state is touched only by the lease renewal goroutine.
	roles, rolesProblem string
	rolesPartial        bool
	rolesCheckedAt      time.Time
}

// NewAgent initializes a new spoke agent.
func NewAgent(cfg AgentConfig) *Agent {
	if cfg.SyncInterval == 0 {
		cfg.SyncInterval = 3 * time.Second
	}
	if cfg.SubjectTemplate == "" {
		cfg.SubjectTemplate = "{{.Email}}"
	}
	return &Agent{
		Config: cfg,
	}
}

// Start begins the reconciliation, lease renewal, and autonomous local sweep loops.
func (a *Agent) Start(ctx context.Context) error {
	enforcerLog.Info("Starting Protem agent", "cluster", a.Config.ClusterName)

	// 1. Lease renewal loop to report liveness to Hub
	go a.renewLeaseLoop(ctx)

	// 2. Autonomous local sweep loop (fails closed on spoke even during Hub partition)
	go a.autonomousSweepLoop(ctx)

	// 3. Hub grants reconciliation loop
	ticker := time.NewTicker(a.Config.SyncInterval)
	defer ticker.Stop()

	// Initial sync pass
	if err := a.SyncOnce(ctx); err != nil {
		enforcerLog.Error(err, "Initial sync pass failed")
	}

	for {
		select {
		case <-ctx.Done():
			enforcerLog.Info("Stopping Protem agent")
			return nil
		case <-ticker.C:
			if err := a.SyncOnce(ctx); err != nil {
				enforcerLog.Error(err, "Sync pass error")
			}
		}
	}
}

// SyncOnce reads ClusterGrants in protem-cluster-<name> on the Hub and applies them locally on the spoke.
func (a *Agent) SyncOnce(ctx context.Context) error {
	clusterNS := fleet.ClusterNamespace(a.Config.ClusterName)

	// List ClusterGrants on Hub in this cluster's namespace (no cluster-scoped TargetCluster read required!)
	var grantList protemv1alpha1.ClusterGrantList
	if err := a.Config.HubClient.List(ctx, &grantList, client.InNamespace(clusterNS)); err != nil {
		return fmt.Errorf("failed to list ClusterGrants in %s on Hub: %w", clusterNS, err)
	}

	activeCount := 0
	activeGrantNames := make(map[string]bool)

	for i := range grantList.Items {
		grant := grantList.Items[i]
		isExpired := time.Now().After(grant.Spec.ExpiresAt.Time)
		isRevoked := grant.Spec.Revocation != nil

		if isExpired || isRevoked {
			if err := binding.Remove(ctx, a.Config.SpokeClient, bindingOwner, "", grant.Name); err != nil {
				enforcerLog.Error(err, "Failed to remove local bindings for grant", "grant", grant.Name)
				continue
			}

			targetPhase := protemv1alpha1.ClusterGrantPhaseExpired
			if isRevoked {
				targetPhase = protemv1alpha1.ClusterGrantPhaseRevoked
			}
			a.report(ctx, &grant, targetPhase, "", nil, nil)
		} else {
			activeCount++
			activeGrantNames[grant.Name] = true
			rules, err := a.roleRules(ctx, &grant)
			var bindings []protemv1alpha1.ManagedBindingRef
			if errors.Is(err, errRoleNotFound) {
				if rmErr := binding.Remove(ctx, a.Config.SpokeClient, bindingOwner, "", grant.Name); rmErr != nil {
					enforcerLog.Error(rmErr, "Failed to remove local bindings for grant", "grant", grant.Name)
					continue
				}
			} else if err == nil {
				bindings, err = a.ensureLocalBindings(ctx, &grant)
			}
			if err != nil {
				enforcerLog.Error(err, "Failed to apply local bindings for grant", "grant", grant.Name)
				a.report(ctx, &grant, protemv1alpha1.ClusterGrantPhaseFailed, err.Error(), bindings, nil)
				continue
			}
			a.report(ctx, &grant, protemv1alpha1.ClusterGrantPhaseActive, "Applied successfully by protem-agent", bindings, rules)
		}
	}

	if err := binding.Sweep(ctx, a.Config.SpokeClient, bindingOwner, func(grant string, _ time.Time) bool {
		return activeGrantNames[grant]
	}); err != nil {
		return fmt.Errorf("failed to sweep orphaned local bindings: %w", err)
	}
	return nil
}

// report writes the grant's status to the hub only when it changed, so a grant that keeps failing
// the same way costs the hub no writes.
func (a *Agent) report(ctx context.Context, grant *protemv1alpha1.ClusterGrant, phase protemv1alpha1.ClusterGrantPhase, message string, bindings []protemv1alpha1.ManagedBindingRef, rules []rbacv1.PolicyRule) {
	s := &grant.Status
	if s.Phase == phase && s.Message == message && equality.Semantic.DeepEqual(s.Bindings, bindings) && equality.Semantic.DeepEqual(s.Rules, rules) {
		return
	}
	now := metav1.Now()
	s.Phase, s.Message, s.Bindings, s.Rules, s.LastTransitionTime = phase, message, bindings, rules, &now
	if err := a.Config.HubClient.Status().Update(ctx, grant); err != nil {
		enforcerLog.Error(err, "Failed to report grant status to the hub", "grant", grant.Name)
	}
}

var errRoleNotFound = errors.New("RoleNotFound")

func (a *Agent) roleRules(ctx context.Context, grant *protemv1alpha1.ClusterGrant) ([]rbacv1.PolicyRule, error) {
	role := grant.Spec.Role
	if role.Kind == "ClusterRole" {
		var cr rbacv1.ClusterRole
		err := a.Config.SpokeClient.Get(ctx, client.ObjectKey{Name: role.Name}, &cr)
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: ClusterRole %q does not exist on this cluster", errRoleNotFound, role.Name)
		}
		return cr.Rules, err
	}
	for _, ns := range grant.Spec.Namespaces {
		if ns == "" || ns == "*" {
			continue
		}
		err := a.Config.SpokeClient.Get(ctx, client.ObjectKey{Namespace: ns, Name: role.Name}, &rbacv1.Role{})
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("%w: Role %q does not exist in namespace %q", errRoleNotFound, role.Name, ns)
		}
		if err != nil {
			return nil, err
		}
	}
	return nil, nil
}

func (a *Agent) ensureLocalBindings(ctx context.Context, grant *protemv1alpha1.ClusterGrant) ([]protemv1alpha1.ManagedBindingRef, error) {
	subject := grant.Spec.Subject
	if subject == "" {
		subject = binding.FormatSubject(a.Config.SubjectTemplate, grant.Spec.Requester)
	}
	return binding.Apply(ctx, a.Config.SpokeClient, bindingOwner, binding.Desired{
		Grant:      grant.Name,
		Subject:    subject,
		Role:       grant.Spec.Role,
		Scope:      grant.Spec.Scope,
		Namespaces: grant.Spec.Namespaces,
		ExpiresAt:  grant.Spec.ExpiresAt.Time,
	})
}

// autonomousSweepLoop runs on the spoke locally every 5 seconds.
// It checks local bindings for expiration using the binding.AnnotationExpiry timestamp.
// If expired, it purges the binding immediately, guaranteeing that network loss
// to the Hub never extends privileged access past its TTL.
func (a *Agent) autonomousSweepLoop(ctx context.Context) {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.sweepExpired(ctx, time.Now()); err != nil {
				enforcerLog.Error(err, "Autonomous sweep of expired bindings failed")
			}
		}
	}
}

func (a *Agent) sweepExpired(ctx context.Context, now time.Time) error {
	return binding.Sweep(ctx, a.Config.SpokeClient, bindingOwner, func(_ string, expiresAt time.Time) bool {
		return now.Before(expiresAt)
	})
}

// renewLeaseLoop creates and renews a coordination.k8s.io/v1 Lease in protem-cluster-<name> on the Hub.
func (a *Agent) renewLeaseLoop(ctx context.Context) {
	if err := a.renewLease(ctx); err != nil {
		enforcerLog.Error(err, "Failed to renew agent Lease on Hub")
	}
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := a.renewLease(ctx); err != nil {
				enforcerLog.Error(err, "Failed to renew agent Lease on Hub")
			}
		}
	}
}

func (a *Agent) renewLease(ctx context.Context) error {
	clusterNS := fleet.ClusterNamespace(a.Config.ClusterName)
	leaseDurationSeconds := int32(30)
	now := metav1.NowMicro()
	roles, partial := a.allowedRoles(ctx, now.Time)
	var lease coordinationv1.Lease
	err := a.Config.HubClient.Get(ctx, client.ObjectKey{Namespace: clusterNS, Name: fleet.AgentLeaseName}, &lease)
	if apierrors.IsNotFound(err) {
		lease = coordinationv1.Lease{
			ObjectMeta: metav1.ObjectMeta{
				Name:      fleet.AgentLeaseName,
				Namespace: clusterNS,
			},
			Spec: coordinationv1.LeaseSpec{
				HolderIdentity:       &a.Config.ClusterName,
				LeaseDurationSeconds: &leaseDurationSeconds,
				RenewTime:            &now,
			},
		}
		setAllowedRoles(&lease, roles, partial)
		return a.Config.HubClient.Create(ctx, &lease)
	}
	if err != nil {
		return err
	}
	lease.Spec.RenewTime = &now
	setAllowedRoles(&lease, roles, partial)
	return a.Config.HubClient.Update(ctx, &lease)
}

func setAllowedRoles(lease *coordinationv1.Lease, roles string, partial bool) {
	if partial {
		metav1.SetMetaDataAnnotation(&lease.ObjectMeta, fleet.AllowedRolesPartialAnnotation, "true")
	} else {
		delete(lease.Annotations, fleet.AllowedRolesPartialAnnotation)
	}
	if roles == "" {
		delete(lease.Annotations, fleet.AllowedRolesAnnotation)
		return
	}
	metav1.SetMetaDataAnnotation(&lease.ObjectMeta, fleet.AllowedRolesAnnotation, roles)
}

// allowedRoles asks the target, at most once a minute, which roles the agent may bind: "" when it
// cannot say, and partial when it listed only some rules, so more roles may be allowed.
func (a *Agent) allowedRoles(ctx context.Context, now time.Time) (roles string, partial bool) {
	if now.Sub(a.rolesCheckedAt) < time.Minute {
		return a.roles, a.rolesPartial
	}
	a.rolesCheckedAt = now
	review := &authorizationv1.SelfSubjectRulesReview{Spec: authorizationv1.SelfSubjectRulesReviewSpec{Namespace: metav1.NamespaceDefault}}
	a.roles, a.rolesPartial = "", false
	err := a.Config.SpokeClient.Create(ctx, review)
	if err == nil {
		a.roles = allowedRolesAnnotation(review.Status.ResourceRules)
		a.rolesPartial = a.roles != "" && review.Status.Incomplete
	}
	problem := ""
	switch {
	case err != nil:
		problem = "the rules review failed: " + err.Error()
	case a.roles == "" && review.Status.Incomplete:
		problem = "the target reports its rules incomplete: " + review.Status.EvaluationError
	case a.roles == "":
		problem = "no rule lets the agent bind roles"
	}
	if problem != "" && problem != a.rolesProblem {
		enforcerLog.Info("Not reporting allowed roles to the hub", "reason", problem)
	}
	a.rolesProblem = problem
	return a.roles, a.rolesPartial
}

// allowedRolesAnnotation is the fleet.AllowedRolesAnnotation value for the rules: the roles
// the rules let the agent bind, "*" for any, "" for none.
func allowedRolesAnnotation(rules []authorizationv1.ResourceRule) string {
	names := sets.New[string]()
	for _, r := range rules {
		if !grants(r.APIGroups, rbacv1.GroupName) || !grants(r.Verbs, "bind") || !grants(r.Resources, "roles") && !grants(r.Resources, "clusterroles") {
			continue
		}
		if len(r.ResourceNames) == 0 {
			return "*"
		}
		names.Insert(r.ResourceNames...)
	}
	return strings.Join(sets.List(names), ",")
}

func grants(values []string, want string) bool {
	return slices.Contains(values, want) || slices.Contains(values, "*")
}
