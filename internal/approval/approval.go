// Package approval decides who may act on an AccessRequest and what the recorded decisions imply.
package approval

import (
	"cmp"
	"errors"
	"fmt"
	"slices"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	v1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// Person is an identity acting on profiles and requests.
type Person struct {
	Username, Email string
	Groups          []string
}

var (
	ErrNotPending      = errors.New("request is not Pending")
	ErrUnresolved      = errors.New("request is awaiting requirement resolution")
	ErrNotEligible     = errors.New("person is not eligible for this decision")
	ErrAlreadyApproved = errors.New("person has already approved every rule they are eligible for")
)

// SamePerson reports whether a and b share a non-empty username or email. A missing email stands
// for the username: a request created with kubectl may name only the name the API server gave its
// caller, and that name can be the address the same person's login carries as their email.
func SamePerson(a, b Person) bool {
	aEmail, bEmail := cmp.Or(a.Email, a.Username), cmp.Or(b.Email, b.Username)
	return (a.Username != "" && a.Username == b.Username) || (aEmail != "" && aEmail == bEmail)
}

// CanRequest reports whether p is one of the profile's requester groups or users.
func CanRequest(p Person, profile *v1.AccessProfile) bool {
	return Named(p, profile.Spec.Requesters.Groups, profile.Spec.Requesters.Users)
}

// IsApprover reports whether p is named by any of the profile's approval rules.
func IsApprover(p Person, profile *v1.AccessProfile) bool {
	for _, rule := range profile.Spec.Approval.Rules {
		if rule.Approvers != nil && Named(p, rule.Approvers.Groups, rule.Approvers.Users) {
			return true
		}
	}
	return false
}

// CanView reports whether p is the requester or an approver of any resolved requirement of req.
func CanView(p Person, req *v1.AccessRequest) bool {
	if SamePerson(p, Person(req.Spec.Requester)) {
		return true
	}
	return slices.ContainsFunc(requirements(req), func(r v1.ClusterApprovalRequirement) bool {
		return Named(p, r.ApproverGroups, r.ApproverUsers)
	})
}

// Decide returns the decisions p would append to req.Status.Approvals for kind.
func Decide(p Person, req *v1.AccessRequest, kind v1.ApprovalDecisionType, comment string, at metav1.Time) ([]v1.ApprovalDecision, error) {
	if req.Status.Phase != v1.RequestPhasePending {
		return nil, fmt.Errorf("%w: phase is %s", ErrNotPending, req.Status.Phase)
	}
	decision := func(rule string) v1.ApprovalDecision {
		user := p.Email
		if user == "" {
			user = p.Username
		}
		return v1.ApprovalDecision{User: user, Decision: kind, At: at, Comment: comment, Rule: rule}
	}

	if kind == v1.ApprovalDecisionCancelled {
		if !SamePerson(p, Person(req.Spec.Requester)) {
			return nil, ErrNotEligible
		}
		return []v1.ApprovalDecision{decision("")}, nil
	}

	reqs := requirements(req)
	if len(reqs) == 0 {
		return nil, ErrUnresolved
	}
	var rules []string
	for _, r := range reqs {
		if eligible(p, req, r) && !slices.Contains(rules, r.RuleName) {
			rules = append(rules, r.RuleName)
		}
	}
	if len(rules) == 0 {
		return nil, ErrNotEligible
	}
	if kind == v1.ApprovalDecisionApproved {
		rules = slices.DeleteFunc(rules, func(rule string) bool {
			return slices.ContainsFunc(req.Status.Approvals, func(d v1.ApprovalDecision) bool {
				return d.Decision == v1.ApprovalDecisionApproved && d.Rule == rule && d.User != "" && (d.User == p.Email || d.User == p.Username)
			})
		})
		if len(rules) == 0 {
			return nil, ErrAlreadyApproved
		}
	}
	if kind == v1.ApprovalDecisionRejected {
		rules = rules[:1]
	}
	decisions := make([]v1.ApprovalDecision, 0, len(rules))
	for _, rule := range rules {
		decisions = append(decisions, decision(rule))
	}
	return decisions, nil
}

// Outcome is the phase the recorded decisions imply for a Pending request.
func Outcome(req *v1.AccessRequest) v1.RequestPhase {
	approvals := req.Status.Approvals
	if slices.ContainsFunc(approvals, isKind(v1.ApprovalDecisionCancelled)) {
		return v1.RequestPhaseCancelled
	}
	if slices.ContainsFunc(approvals, isKind(v1.ApprovalDecisionRejected)) {
		return v1.RequestPhaseRejected
	}
	reqs := Progress(req)
	if len(reqs) == 0 {
		return v1.RequestPhasePending
	}
	for _, p := range reqs {
		if p.Requirement.Mode != v1.ApprovalModeAuto && len(p.ApprovedBy) < p.Requirement.RequiredApprovals {
			return v1.RequestPhasePending
		}
	}
	return v1.RequestPhaseApproved
}

// RequirementProgress is one resolved requirement and the distinct users who approved its rule.
type RequirementProgress struct {
	Requirement v1.ClusterApprovalRequirement
	ApprovedBy  []string
}

// Progress lists each resolved requirement of req, in order, with the distinct approvers of its rule.
// ApprovedBy is never nil.
func Progress(req *v1.AccessRequest) []RequirementProgress {
	reqs := requirements(req)
	out := make([]RequirementProgress, 0, len(reqs))
	for _, r := range reqs {
		p := RequirementProgress{Requirement: r, ApprovedBy: []string{}}
		for _, d := range req.Status.Approvals {
			if d.Decision == v1.ApprovalDecisionApproved && r.RuleName != "" && d.Rule == r.RuleName && !slices.Contains(p.ApprovedBy, d.User) {
				p.ApprovedBy = append(p.ApprovedBy, d.User)
			}
		}
		out = append(out, p)
	}
	return out
}

func eligible(p Person, req *v1.AccessRequest, r v1.ClusterApprovalRequirement) bool {
	return r.Mode != v1.ApprovalModeAuto &&
		Named(p, r.ApproverGroups, r.ApproverUsers) &&
		(r.AllowSelfApproval || !SamePerson(p, Person(req.Spec.Requester)))
}

// Named reports whether p belongs to one of groups, or is one of users by a non-empty username or email.
func Named(p Person, groups, users []string) bool {
	return inAny(p.Groups, groups) || slices.ContainsFunc(users, func(u string) bool {
		return u != "" && (u == p.Email || u == p.Username)
	})
}

func inAny(have, want []string) bool {
	return slices.ContainsFunc(have, func(g string) bool { return slices.Contains(want, g) })
}

func requirements(req *v1.AccessRequest) []v1.ClusterApprovalRequirement {
	if req.Status.Resolved == nil {
		return nil
	}
	return req.Status.Resolved.Requirements
}

func isKind(kind v1.ApprovalDecisionType) func(v1.ApprovalDecision) bool {
	return func(d v1.ApprovalDecision) bool { return d.Decision == kind }
}
