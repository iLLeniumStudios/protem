package api

import (
	"slices"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

// UserInfo represents an authenticated user identity.
type UserInfo struct {
	Username string   `json:"username"`
	Email    string   `json:"email"`
	Groups   []string `json:"groups"`
	Roles    []string `json:"roles"`
}

// MeView is the authenticated user plus how the server authenticates.
type MeView struct {
	UserInfo `tstype:",extends"`
	DevAuth  bool `json:"devAuth"`
	// SignOutURL is where the UI sends the browser to end the session (--sign-out-url); empty hides the link.
	SignOutURL string `json:"signOutURL,omitempty"`
}

// CapabilitiesView is what the caller may do on the hub, decided by SubjectAccessReviews, never by token role claims.
type CapabilitiesView struct {
	// Management is true when the server runs with --ui-management=enabled.
	Management bool `json:"management"`
	// ProfileNamespaces lists namespaces where the caller may create and update AccessProfiles; "*" means every namespace.
	ProfileNamespaces []string `json:"profileNamespaces"`
	// ManageClusters: the caller may update TargetClusters.
	ManageClusters bool `json:"manageClusters"`
	// AddClusters: the caller may add and remove TargetClusters and issue their agents' credentials.
	AddClusters bool `json:"addClusters"`
	// ViewFleet: the caller may list TargetClusters.
	ViewFleet bool `json:"viewFleet"`
	// ViewAll: the caller may read every AccessRequest and AccessGrant (auditor or admin).
	ViewAll bool `json:"viewAll"`
}

type StatsView struct {
	ActiveGrants      int `json:"activeGrants"`
	PendingRequests   int `json:"pendingRequests"`
	TotalProfiles     int `json:"totalProfiles"`
	TotalRequests     int `json:"totalRequests"`
	ConnectedClusters int `json:"connectedClusters"`
}

// AuditItem is one thing that happened to a request or its grant. Rows are derived from facts the
// objects record once (creation, each decision, revocation, expiry), so a row never changes after it appears.
type AuditItem struct {
	// ID is stable for the life of the request: "<namespace>/<request>#<sequence>".
	ID        string    `json:"id"`
	Timestamp time.Time `json:"timestamp"`
	// EventType is REQUEST_SUBMITTED, APPROVAL_GIVEN, REQUEST_APPROVED, REQUEST_REJECTED, REQUEST_CANCELLED,
	// REQUEST_TIMED_OUT, ACCESS_GRANTED, ACCESS_REVOKED or ACCESS_EXPIRED.
	EventType string `json:"eventType"`
	Namespace string `json:"namespace"`
	Request   string `json:"request"`
	Grant     string `json:"grant,omitempty"`
	Requester string `json:"requester"`
	// Actor is who did it; empty when the system did (automatic approval, timeout, expiry, failed validation).
	Actor    string   `json:"actor,omitempty"`
	Profile  string   `json:"profile"`
	Clusters []string `json:"clusters,omitempty"`
	Duration string   `json:"duration,omitempty"`
	Ticket   string   `json:"ticket,omitempty"`
	// Reason is the requester's justification.
	Reason string `json:"reason,omitempty"`
	// Comment is the actor's comment, the rejection or revocation reason, or the system's explanation.
	Comment string `json:"comment,omitempty"`
}

// Pages: Items is one page in the route's order; Total counts every item the caller may see that matches the filters.

type RequestPage struct {
	Items []RequestView `json:"items"`
	Total int           `json:"total"`
}

type GrantPage struct {
	Items []GrantView `json:"items"`
	Total int         `json:"total"`
}

type AuditPage struct {
	Items []AuditItem `json:"items"`
	Total int         `json:"total"`
}

type CreateRequestPayload struct {
	ProfileRef string   `json:"profileRef"`
	Duration   string   `json:"duration"`
	Reason     string   `json:"reason"`
	Ticket     string   `json:"ticket,omitempty"`
	Namespaces []string `json:"targetNamespaces,omitempty"`
	Clusters   []string `json:"targetClusters,omitempty"`
}

type RoleView struct {
	Kind string `json:"kind"`
	Name string `json:"name"`
}

type RequesterView struct {
	Username string `json:"username"`
	Email    string `json:"email"`
}

type ProfileView struct {
	Namespace       string   `json:"namespace"`
	Name            string   `json:"name"`
	DisplayName     string   `json:"displayName"`
	Description     string   `json:"description,omitempty"`
	Role            RoleView `json:"role"`
	Scope           string   `json:"scope"`
	Namespaces      []string `json:"namespaces,omitempty"`
	Clusters        []string `json:"clusters"`
	DefaultDuration string   `json:"defaultDuration"`
	MaxDuration     string   `json:"maxDuration"`
	ReasonRequired  bool     `json:"reasonRequired"`
	TicketPattern   string   `json:"ticketPattern,omitempty"`
	// Activation is "at-once", "approval" or "mixed" (it differs by cluster): how a request by the
	// caller would be decided, or how the rules decide in general when the caller cannot request.
	Activation      string   `json:"activation"`
	ApproverGroups  []string `json:"approverGroups,omitempty"`
	ApproverUsers   []string `json:"approverUsers,omitempty"`
	RequesterGroups []string `json:"requesterGroups,omitempty"`
	RequesterUsers  []string `json:"requesterUsers,omitempty"`
	CanRequest      bool     `json:"canRequest"`
	CanApprove      bool     `json:"canApprove"`
	// ManagedBy names the GitOps tool or lock that owns the object ("Argo CD", "Flux", "Helm", or the lock label's value); empty when the UI may edit it.
	ManagedBy string `json:"managedBy,omitempty"`
}

// ProfileDetailView is one profile with everything needed to display, copy and edit it.
type ProfileDetailView struct {
	ProfileView     `tstype:",extends"`
	ResourceVersion string                           `json:"resourceVersion"`
	Spec            protemv1alpha1.AccessProfileSpec `json:"spec" tstype:"AccessProfileSpec"`
	// YAML is a clean manifest (apiVersion, kind, name, namespace, labels, spec) for GitOps users to copy.
	YAML string `json:"yaml"`
}

// ProfilePayload creates (name required) or replaces (resourceVersion required) a profile's spec.
type ProfilePayload struct {
	Name            string                           `json:"name,omitempty"`
	ResourceVersion string                           `json:"resourceVersion,omitempty"`
	Spec            protemv1alpha1.AccessProfileSpec `json:"spec" tstype:"AccessProfileSpec"`
}

// PermissionRuleView is one RBAC rule; Risks is non-empty when the rule deserves an approver's attention.
type PermissionRuleView struct {
	APIGroups       []string `json:"apiGroups,omitempty"`
	Resources       []string `json:"resources,omitempty"`
	ResourceNames   []string `json:"resourceNames,omitempty"`
	NonResourceURLs []string `json:"nonResourceURLs,omitempty"`
	Verbs           []string `json:"verbs"`
	Risks           []string `json:"risks,omitempty"`
}

// PermissionsView is what a role grants and where that answer was read.
type PermissionsView struct {
	Role RoleView `json:"role"`
	// Source is "hub" (the hub's copy of the role) or "cluster" (reported by that cluster's agent after binding).
	Source string `json:"source"`
	// Found is false when the role could not be read; Message says why.
	Found   bool                 `json:"found"`
	Message string               `json:"message,omitempty"`
	Rules   []PermissionRuleView `json:"rules"`
}

type BindingView struct {
	Kind      string `json:"kind"`
	Namespace string `json:"namespace,omitempty"`
	Name      string `json:"name"`
}

// ClusterPermissionsView is what one grant actually holds on one cluster.
type ClusterPermissionsView struct {
	Cluster     string          `json:"cluster"`
	State       string          `json:"state"`
	Message     string          `json:"message,omitempty"`
	Subject     string          `json:"subject,omitempty"`
	Bindings    []BindingView   `json:"bindings,omitempty"`
	Permissions PermissionsView `json:"permissions"`
}

type DecisionView struct {
	User     string    `json:"user"`
	Decision string    `json:"decision"`
	At       time.Time `json:"at"`
	Comment  string    `json:"comment,omitempty"`
	Rule     string    `json:"rule,omitempty"`
}

type RequestActions struct {
	Approve bool `json:"approve"`
	Reject  bool `json:"reject"`
	Cancel  bool `json:"cancel"`
}

type RequirementView struct {
	Cluster        string   `json:"cluster"`
	Rule           string   `json:"rule"`
	Mode           string   `json:"mode"`
	Required       int      `json:"required"`
	ApprovedBy     []string `json:"approvedBy"`
	ApproverGroups []string `json:"approverGroups,omitempty"`
	ApproverUsers  []string `json:"approverUsers,omitempty"`
}

type RequestView struct {
	Namespace string        `json:"namespace"`
	Name      string        `json:"name"`
	Requester RequesterView `json:"requester"`
	Profile   string        `json:"profile"`
	// Role and Scope are the ones frozen when the request was resolved; absent before that.
	Role         *RoleView         `json:"role,omitempty"`
	Scope        string            `json:"scope,omitempty"`
	Reason       string            `json:"reason"`
	Ticket       string            `json:"ticket,omitempty"`
	Duration     string            `json:"duration"`
	Phase        string            `json:"phase"`
	CreatedAt    time.Time         `json:"createdAt"`
	Clusters     []string          `json:"clusters,omitempty"`
	Namespaces   []string          `json:"namespaces,omitempty"`
	Requirements []RequirementView `json:"requirements,omitempty"`
	GrantRef     string            `json:"grantRef,omitempty"`
	Approvals    []DecisionView    `json:"approvals,omitempty"`
	// FlaggedRules counts the rules of the request's role, as the hub defines it, that deserve an
	// approver's attention; 0 when there are none or the hub cannot read the role.
	FlaggedRules int            `json:"flaggedRules,omitempty"`
	Actions      RequestActions `json:"actions"`
}

type RevocationView struct {
	By     string    `json:"by,omitempty"`
	At     time.Time `json:"at"`
	Reason string    `json:"reason,omitempty"`
}

type ClusterStateView struct {
	Name    string `json:"name"`
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type GrantActions struct {
	Revoke bool `json:"revoke"`
}

type GrantView struct {
	Namespace string `json:"namespace"`
	Name      string `json:"name"`
	// Request is the name of the AccessRequest, in the same namespace, that this grant came from.
	Request    string        `json:"request"`
	Requester  RequesterView `json:"requester"`
	Profile    string        `json:"profile"`
	Role       RoleView      `json:"role"`
	Scope      string        `json:"scope"`
	Namespaces []string      `json:"namespaces,omitempty"`
	Clusters   []string      `json:"clusters,omitempty"`
	// StartedAt is when the grant was issued; with ExpiresAt it gives the whole duration.
	StartedAt        time.Time          `json:"startedAt"`
	ExpiresAt        time.Time          `json:"expiresAt"`
	Phase            string             `json:"phase"`
	RemainingSeconds int64              `json:"remainingSeconds"`
	Revocation       *RevocationView    `json:"revocation,omitempty"`
	ClusterStates    []ClusterStateView `json:"clusterStates,omitempty"`
	Actions          GrantActions       `json:"actions"`
}

type ClusterView struct {
	Name              string            `json:"name"`
	DisplayName       string            `json:"displayName"`
	Description       string            `json:"description,omitempty"`
	Labels            map[string]string `json:"labels,omitempty"`
	Phase             string            `json:"phase"`
	Ready             bool              `json:"ready"`
	AgentVersion      string            `json:"agentVersion,omitempty"`
	LastHeartbeat     *time.Time        `json:"lastHeartbeat,omitempty"`
	ActiveGrantsCount int               `json:"activeGrantsCount"`
	Paused            bool              `json:"paused"`
	Subject           string            `json:"subject,omitempty"`
	Enforcement       string            `json:"enforcement,omitempty"`
	ManagedBy         string            `json:"managedBy,omitempty"`
	// BoundAs is the Kubernetes user name the caller would be bound as on this cluster.
	BoundAs string `json:"boundAs"`
	// Agent is "none" for the hub itself (it has no agent), "waiting" for a cluster whose agent has
	// never reported in, "connected" while its heartbeat is fresh, and "lost" once it is not.
	Agent string `json:"agent"`
	// AllowedRoles are the roles the agent reported it may bind on the cluster; "*" means any.
	// Absent when it has not reported, and for the hub, where the operator's own limit applies.
	AllowedRoles []string `json:"allowedRoles,omitempty"`
	// AllowedRolesPartial: the cluster's authorizer could not list everything, so more roles than
	// AllowedRoles may be allowed.
	AllowedRolesPartial bool `json:"allowedRolesPartial,omitempty"`
}

// AddClusterPayload registers a target cluster from the UI.
type AddClusterPayload struct {
	// Name is the cluster's identifier: a DNS label of at most 40 characters. It cannot be changed later.
	Name        string            `json:"name"`
	DisplayName string            `json:"displayName"`
	Description string            `json:"description,omitempty"`
	Labels      map[string]string `json:"labels,omitempty"`
}

// JoinView is everything needed to connect an agent to the hub. It is returned once and not stored.
type JoinView struct {
	Cluster string `json:"cluster"`
	// ExpiresAt is when the join token stops working. An agent that started before then has already
	// replaced it with a token of its own.
	ExpiresAt time.Time `json:"expiresAt"`
	// Kubeconfig is the agent's hub kubeconfig holding the join token, to be saved as hub.kubeconfig.
	Kubeconfig string `json:"kubeconfig"`
	// Kustomization is a kustomization.yaml that installs the agent on the target cluster, reading
	// hub.kubeconfig from the same directory.
	Kustomization string `json:"kustomization"`
	// Command applies that directory to the target cluster.
	Command string `json:"command"`
}

// ClusterSettingsPayload changes the settings the UI may edit on a TargetCluster; absent fields stay as they are.
type ClusterSettingsPayload struct {
	Paused  *bool   `json:"paused,omitempty"`
	Subject *string `json:"subject,omitempty"`
	// Labels replaces the cluster's labels, except keys under a reserved prefix, which are kept and may not be sent.
	Labels *map[string]string `json:"labels,omitempty"`
}

func toProfileView(p *protemv1alpha1.AccessProfile, person approval.Person, spokes []protemv1alpha1.TargetCluster) ProfileView {
	clusters, _ := fleet.EligibleClusters(p, spokes)
	if clusters == nil {
		clusters = []string{}
	}
	v := ProfileView{
		Namespace:       p.Namespace,
		Name:            p.Name,
		DisplayName:     p.Spec.DisplayName,
		Description:     p.Spec.Description,
		Role:            RoleView(p.Spec.Role),
		Scope:           string(p.Spec.Scope),
		Namespaces:      p.Spec.Namespaces,
		Clusters:        clusters,
		DefaultDuration: p.Spec.Duration.Default.Duration.String(),
		MaxDuration:     p.Spec.Duration.Max.Duration.String(),
		ReasonRequired:  p.Spec.Justification.Reason == protemv1alpha1.JustificationRequired,
		TicketPattern:   p.Spec.Justification.TicketPattern,
		RequesterGroups: p.Spec.Requesters.Groups,
		RequesterUsers:  p.Spec.Requesters.Users,
		CanRequest:      approval.CanRequest(person, p),
		CanApprove:      approval.IsApprover(person, p),
		ManagedBy:       managedBy(p),
	}
	var auto, manual bool
	for _, r := range p.Spec.Approval.Rules {
		auto, manual = auto || r.Mode == protemv1alpha1.ApprovalModeAuto, manual || r.Mode != protemv1alpha1.ApprovalModeAuto
		if r.Approvers != nil {
			v.ApproverGroups = appendNew(v.ApproverGroups, r.Approvers.Groups...)
			v.ApproverUsers = appendNew(v.ApproverUsers, r.Approvers.Users...)
		}
	}
	if v.CanRequest && len(clusters) > 0 {
		auto, manual = callerModes(p, person, spokes, clusters)
	}
	switch {
	case auto && manual:
		v.Activation = "mixed"
	case auto:
		v.Activation = "at-once"
	default:
		v.Activation = "approval"
	}
	return v
}

// callerModes reports whether a request by person would be decided automatically on some
// eligible cluster, and whether it would need approval on some.
func callerModes(p *protemv1alpha1.AccessProfile, person approval.Person, spokes []protemv1alpha1.TargetCluster, eligible []string) (auto, manual bool) {
	req := &protemv1alpha1.AccessRequest{Spec: protemv1alpha1.AccessRequestSpec{Requester: protemv1alpha1.RequesterInfo(person)}}
	for i := range spokes {
		if !slices.Contains(eligible, spokes[i].Name) {
			continue
		}
		// For one eligible cluster the only failure is that no rule matches it: no grant at once.
		eval, err := fleet.ResolveAndEvaluate(p, req, spokes[i:i+1])
		if err == nil && eval.OverallMode == protemv1alpha1.ApprovalModeAuto {
			auto = true
		} else {
			manual = true
		}
	}
	return auto, manual
}

func appendNew(dst []string, values ...string) []string {
	for _, s := range values {
		if !slices.Contains(dst, s) {
			dst = append(dst, s)
		}
	}
	return dst
}

func requestPhase(req *protemv1alpha1.AccessRequest) string {
	if req.Status.Phase == "" {
		return string(protemv1alpha1.RequestPhasePending)
	}
	return string(req.Status.Phase)
}

func requestDuration(req *protemv1alpha1.AccessRequest) string {
	if req.Status.Resolved != nil {
		return req.Status.Resolved.Duration.Duration.String()
	}
	if req.Spec.Duration != nil {
		return req.Spec.Duration.Duration.String()
	}
	return ""
}

func toRequestView(req *protemv1alpha1.AccessRequest, person approval.Person, now metav1.Time, flaggedRules int) RequestView {
	can := func(kind protemv1alpha1.ApprovalDecisionType) bool {
		_, err := approval.Decide(person, req, kind, "", now)
		return err == nil
	}
	v := RequestView{
		Namespace:    req.Namespace,
		Name:         req.Name,
		Requester:    RequesterView{Username: req.Spec.Requester.Username, Email: req.Spec.Requester.Email},
		Profile:      req.Spec.ProfileRef,
		Reason:       req.Spec.Justification.Reason,
		Ticket:       req.Spec.Justification.Ticket,
		Duration:     requestDuration(req),
		Phase:        requestPhase(req),
		CreatedAt:    req.CreationTimestamp.Time,
		Clusters:     req.Spec.Clusters,
		Namespaces:   req.Spec.Namespaces,
		GrantRef:     req.Status.GrantRef,
		FlaggedRules: flaggedRules,
		Actions: RequestActions{
			Approve: can(protemv1alpha1.ApprovalDecisionApproved),
			Reject:  can(protemv1alpha1.ApprovalDecisionRejected),
			Cancel:  can(protemv1alpha1.ApprovalDecisionCancelled),
		},
	}
	if r := req.Status.Resolved; r != nil {
		v.Clusters, v.Namespaces = r.Clusters, r.Namespaces
		if r.Role != nil {
			role := RoleView(*r.Role)
			v.Role = &role
		}
		v.Scope = string(r.Scope)
	}
	for _, p := range approval.Progress(req) {
		r := p.Requirement
		v.Requirements = append(v.Requirements, RequirementView{
			Cluster:        r.ClusterName,
			Rule:           r.RuleName,
			Mode:           string(r.Mode),
			Required:       r.RequiredApprovals,
			ApprovedBy:     p.ApprovedBy,
			ApproverGroups: r.ApproverGroups,
			ApproverUsers:  r.ApproverUsers,
		})
	}
	for _, d := range req.Status.Approvals {
		v.Approvals = append(v.Approvals, DecisionView{
			User:     d.User,
			Decision: string(d.Decision),
			At:       d.At.Time,
			Comment:  d.Comment,
			Rule:     d.Rule,
		})
	}
	return v
}

func toGrantView(g *protemv1alpha1.AccessGrant, person approval.Person, mayRevokeAny bool, now time.Time) GrantView {
	var remaining int64
	if g.Status.Phase == protemv1alpha1.GrantPhaseActive && now.Before(g.Spec.ExpiresAt.Time) {
		remaining = int64(g.Spec.ExpiresAt.Sub(now).Seconds())
	}
	v := GrantView{
		Namespace:        g.Namespace,
		Name:             g.Name,
		Request:          g.Spec.RequestRef,
		StartedAt:        g.CreationTimestamp.Time,
		Requester:        RequesterView{Username: g.Spec.Requester.Username, Email: g.Spec.Requester.Email},
		Profile:          g.Spec.ProfileRef,
		Role:             RoleView(g.Spec.Role),
		Scope:            string(g.Spec.Scope),
		Namespaces:       g.Spec.Namespaces,
		Clusters:         g.Spec.Clusters,
		ExpiresAt:        g.Spec.ExpiresAt.Time,
		Phase:            string(g.Status.Phase),
		RemainingSeconds: remaining,
		Actions: GrantActions{
			Revoke: g.Spec.Revocation == nil && now.Before(g.Spec.ExpiresAt.Time) && (mayRevokeAny || approval.SamePerson(person, approval.Person(g.Spec.Requester))),
		},
	}
	if r := g.Spec.Revocation; r != nil {
		v.Revocation = &RevocationView{By: r.By, At: r.At.Time, Reason: r.Reason}
	}
	for _, c := range g.Status.Clusters {
		v.ClusterStates = append(v.ClusterStates, ClusterStateView{Name: c.ClusterName, State: string(c.State), Message: c.Message})
	}
	return v
}

func toClusterView(c *protemv1alpha1.TargetCluster, activeGrants int) ClusterView {
	v := ClusterView{
		Name:              c.Name,
		DisplayName:       c.Spec.DisplayName,
		Description:       c.Spec.Description,
		Labels:            c.Labels,
		Phase:             string(c.Status.Phase),
		Ready:             fleet.IsClusterReady(c),
		AgentVersion:      c.Status.AgentVersion,
		ActiveGrantsCount: activeGrants,
		Paused:            c.Spec.Paused,
		Subject:           c.Spec.Subject,
		Enforcement:       string(c.Spec.Enforcement),
		ManagedBy:         managedBy(c),
	}
	if hb := c.Status.LastHeartbeat; hb != nil {
		v.LastHeartbeat = &hb.Time
	}
	return v
}

func toProfileDetailView(p *protemv1alpha1.AccessProfile, person approval.Person, spokes []protemv1alpha1.TargetCluster) (ProfileDetailView, error) {
	metadata := map[string]any{"name": p.Name, "namespace": p.Namespace}
	if len(p.Labels) > 0 {
		metadata["labels"] = p.Labels
	}
	manifest, err := yaml.Marshal(map[string]any{
		"apiVersion": protemv1alpha1.GroupVersion.String(),
		"kind":       "AccessProfile",
		"metadata":   metadata,
		"spec":       p.Spec,
	})
	if err != nil {
		return ProfileDetailView{}, err
	}
	return ProfileDetailView{
		ProfileView:     toProfileView(p, person, spokes),
		ResourceVersion: p.ResourceVersion,
		Spec:            p.Spec,
		YAML:            string(manifest),
	}, nil
}
