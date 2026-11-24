package controller

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
	"github.com/iLLeniumStudios/protem/internal/approval"
	"github.com/iLLeniumStudios/protem/internal/fleet"
)

const (
	LabelProfileRef     = "protem.dev/profile-ref"
	pendingPollInterval = 30 * time.Second
	// A new request waits this long for a targeted cluster that is not Ready (the hub just
	// started, an agent is reconnecting) before it is rejected for it.
	readyGrace        = time.Minute
	readyPollInterval = 5 * time.Second
)

// Transition is everything one reconcile of an AccessRequest has to write.
type Transition struct {
	Status       *v1.AccessRequestStatus // nil: no status write
	Grant        *v1.AccessGrant         // nil: nothing to create; the caller sets the controller reference
	RequeueAfter time.Duration
}

// Others is the rest of the request's namespace, which decides whether the requester already
// holds or waits for what they ask for. It is read only when validating a new request.
type Others struct {
	Requests []v1.AccessRequest
	Grants   []v1.AccessGrant
}

// Advance computes the next lifecycle step of req at now without mutating its inputs.
// A nil profile means the referenced AccessProfile does not exist.
func Advance(req *v1.AccessRequest, profile *v1.AccessProfile, clusters []v1.TargetCluster, others Others, now time.Time) Transition {
	switch req.Status.Phase {
	case v1.RequestPhaseRejected, v1.RequestPhaseCancelled, v1.RequestPhaseTimedOut:
		return Transition{}
	case v1.RequestPhaseApproved:
		if req.Status.GrantRef != "" {
			return Transition{}
		}
	}

	req = req.DeepCopy()
	if profile == nil {
		return conclude(req, v1.RequestPhaseRejected, now, "Validated", "ProfileNotFound",
			fmt.Sprintf("AccessProfile %q does not exist in namespace %q", req.Spec.ProfileRef, req.Namespace))
	}
	switch req.Status.Phase {
	case "":
		return validate(req, profile, clusters, others, now)
	case v1.RequestPhasePending:
		return review(req, profile, now)
	case v1.RequestPhaseApproved:
		return issue(req, profile, now)
	}
	return Transition{}
}

func notARequester(username string, r v1.ProfileRequesters) string {
	var who []string
	if len(r.Groups) > 0 {
		who = append(who, fmt.Sprintf("the groups %v", r.Groups))
	}
	if len(r.Users) > 0 {
		who = append(who, fmt.Sprintf("the users %v", r.Users))
	}
	return fmt.Sprintf("%q may not request this profile. It can be requested by %s.", username, strings.Join(who, " and "))
}

func validate(req *v1.AccessRequest, profile *v1.AccessProfile, clusters []v1.TargetCluster, others Others, now time.Time) Transition {
	if !approval.CanRequest(approval.Person(req.Spec.Requester), profile) {
		return conclude(req, v1.RequestPhaseRejected, now, "Validated", "UnauthorizedRequester",
			notARequester(req.Spec.Requester.Username, profile.Spec.Requesters))
	}
	if pattern := profile.Spec.Justification.TicketPattern; pattern != "" {
		if matched, err := regexp.MatchString(pattern, req.Spec.Justification.Ticket); err != nil || !matched {
			return conclude(req, v1.RequestPhaseRejected, now, "Validated", "InvalidTicket",
				fmt.Sprintf("Ticket %q does not match required pattern (%s)", req.Spec.Justification.Ticket, pattern))
		}
	}
	if profile.Spec.Justification.Reason == v1.JustificationRequired && req.Spec.Justification.Reason == "" {
		return conclude(req, v1.RequestPhaseRejected, now, "Validated", "MissingJustification",
			"A business justification reason is required for this profile")
	}

	eval, err := fleet.ResolveAndEvaluate(profile, req, clusters)
	if errors.Is(err, fleet.ErrNotReady) {
		if now.Sub(req.CreationTimestamp.Time) < readyGrace {
			return Transition{RequeueAfter: readyPollInterval}
		}
		return conclude(req, v1.RequestPhaseRejected, now, "Validated", "ClusterNotReady", err.Error())
	}
	if err != nil {
		return conclude(req, v1.RequestPhaseRejected, now, "Validated", "ResolutionFailed", err.Error())
	}
	if held := approval.Held(req, eval.ResolvedClusters, others.Requests, others.Grants, now); held != "" {
		return conclude(req, v1.RequestPhaseRejected, now, "Validated", "AlreadyHeld", held)
	}

	reqs := make([]v1.ClusterApprovalRequirement, 0, len(eval.ClusterRules))
	for _, cr := range eval.ClusterRules {
		reqs = append(reqs, v1.ClusterApprovalRequirement{
			ClusterName:       cr.ClusterName,
			RuleName:          cr.RuleName,
			Mode:              cr.Mode,
			RequiredApprovals: cr.RequiredApprovals,
			ApproverGroups:    cr.ApproverGroups,
			ApproverUsers:     cr.ApproverUsers,
			AllowSelfApproval: cr.AllowSelfApproval,
		})
	}
	role := profile.Spec.Role
	req.Status.Resolved = &v1.ResolvedScope{
		Role:         &role,
		Scope:        profile.Spec.Scope,
		Clusters:     eval.ResolvedClusters,
		Namespaces:   eval.ResolvedNamespaces,
		Duration:     eval.ResolvedDuration,
		Requirements: reqs,
	}
	setCondition(req, now, "Validated", metav1.ConditionTrue, "ScopeResolved", "Scope successfully resolved and frozen")

	if eval.OverallMode == v1.ApprovalModeAuto {
		req.Status.Approvals = append(req.Status.Approvals, v1.ApprovalDecision{
			User:     "system:auto-approval",
			Decision: v1.ApprovalDecisionApproved,
			At:       metav1.NewTime(now),
			Rule:     "auto",
		})
		return conclude(req, v1.RequestPhaseApproved, now, "Decided", "AutoApproved", "Request automatically approved per profile policy")
	}
	return conclude(req, v1.RequestPhasePending, now, "Decided", "PendingApproval", "Awaiting review by authorized approvers")
}

func review(req *v1.AccessRequest, profile *v1.AccessProfile, now time.Time) Transition {
	var remaining time.Duration
	if timeout := profile.Spec.Approval.PendingTimeout; timeout != nil && timeout.Duration > 0 {
		deadline := req.CreationTimestamp.Add(timeout.Duration)
		if now.After(deadline) {
			return conclude(req, v1.RequestPhaseTimedOut, now, "Decided", "TimedOut", "Request timed out awaiting approvals")
		}
		remaining = deadline.Sub(now)
	}

	switch approval.Outcome(req) {
	case v1.RequestPhaseCancelled:
		return conclude(req, v1.RequestPhaseCancelled, now, "Decided", "Cancelled", "Request cancelled by requester")
	case v1.RequestPhaseRejected:
		for _, d := range req.Status.Approvals {
			if d.Decision == v1.ApprovalDecisionRejected {
				return conclude(req, v1.RequestPhaseRejected, now, "Decided", "Rejected", fmt.Sprintf("Rejected by %s: %s", d.User, d.Comment))
			}
		}
	case v1.RequestPhaseApproved:
		return conclude(req, v1.RequestPhaseApproved, now, "Decided", "Approved", "All required approvals obtained")
	}

	if remaining > 0 && remaining < pendingPollInterval {
		return Transition{RequeueAfter: remaining + time.Second}
	}
	return Transition{RequeueAfter: pendingPollInterval}
}

func issue(req *v1.AccessRequest, profile *v1.AccessProfile, now time.Time) Transition {
	resolved := req.Status.Resolved
	if resolved == nil || resolved.Role == nil || resolved.Scope == "" || resolved.Duration.Duration <= 0 {
		return conclude(req, v1.RequestPhaseRejected, now, "Decided", "UnresolvedScope",
			"Request was approved without a frozen role, scope and duration; submit a new request")
	}

	approvedAt := now
	if decided := meta.FindStatusCondition(req.Status.Conditions, "Decided"); decided != nil && decided.Status == metav1.ConditionTrue {
		approvedAt = decided.LastTransitionTime.Time
	}
	duration := resolved.Duration.Duration

	grant := &v1.AccessGrant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("grant-%s", req.Name),
			Namespace: req.Namespace,
			Labels: map[string]string{
				"protem.dev/request-ref": req.Name,
				LabelProfileRef:          profile.Name,
			},
		},
		Spec: v1.AccessGrantSpec{
			RequestRef: req.Name,
			ProfileRef: profile.Name,
			Requester:  req.Spec.Requester,
			Role:       *resolved.Role,
			Scope:      resolved.Scope,
			Namespaces: resolved.Namespaces,
			Clusters:   resolved.Clusters,
			ExpiresAt:  metav1.NewTime(approvedAt.Add(duration)),
		},
	}
	req.Status.GrantRef = grant.Name
	return Transition{Status: &req.Status, Grant: grant}
}

func conclude(req *v1.AccessRequest, phase v1.RequestPhase, now time.Time, condType, reason, message string) Transition {
	status := metav1.ConditionFalse
	if phase == v1.RequestPhaseApproved {
		status = metav1.ConditionTrue
	}
	req.Status.Phase = phase
	setCondition(req, now, condType, status, reason, message)
	return Transition{Status: &req.Status}
}

func setCondition(req *v1.AccessRequest, now time.Time, condType string, status metav1.ConditionStatus, reason, message string) {
	old := meta.FindStatusCondition(req.Status.Conditions, condType)
	reasonChanged := old != nil && old.Reason != reason
	meta.SetStatusCondition(&req.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(now),
	})
	// SetStatusCondition keeps the old time unless the status flips. Waiting -> timed out keeps
	// status False, and the audit history reads this time as when the request timed out.
	if reasonChanged {
		meta.FindStatusCondition(req.Status.Conditions, condType).LastTransitionTime = metav1.NewTime(now)
	}
}
