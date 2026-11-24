// Package audit derives the audit events of an access request and its grant.
package audit

import (
	"fmt"
	"slices"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"

	protemv1alpha1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

const autoApprover = "system:auto-approval"

// Event is one thing that happened to an access request or its grant.
type Event struct {
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

func requesterName(r protemv1alpha1.RequesterInfo) string {
	if r.Email != "" {
		return r.Email
	}
	return r.Username
}

// Events returns the request's events in the order they happened. Events only ever get appended as the
// request and its grant move on, so an event's index (and ID) never changes.
func Events(req *protemv1alpha1.AccessRequest, grant *protemv1alpha1.AccessGrant, now time.Time) []Event {
	base := Event{
		Namespace: req.Namespace,
		Request:   req.Name,
		Requester: requesterName(req.Spec.Requester),
		Profile:   req.Spec.ProfileRef,
		Ticket:    req.Spec.Justification.Ticket,
		Reason:    req.Spec.Justification.Reason,
	}
	base.Clusters = req.Spec.Clusters
	if resolved := req.Status.Resolved; resolved != nil {
		base.Clusters = resolved.Clusters
	}
	var events []Event
	add := func(event string, at time.Time, actor, comment string, fill ...func(*Event)) {
		// Events are added in the order things happened. Creation times come from the API server's clock and
		// decisions from the hub's, so a later event can carry an earlier time; never show it before its cause.
		if n := len(events); n > 0 && at.Before(events[n-1].Timestamp) {
			at = events[n-1].Timestamp
		}
		e := base
		e.ID = fmt.Sprintf("%s/%s#%d", req.Namespace, req.Name, len(events)+1)
		e.EventType, e.Timestamp, e.Actor, e.Comment = event, at, actor, comment
		for _, f := range fill {
			f(&e)
		}
		events = append(events, e)
	}

	add("REQUEST_SUBMITTED", req.CreationTimestamp.Time, requesterName(req.Spec.Requester), "")
	automatic, rejectedByPerson := false, false
	for i, d := range req.Status.Approvals {
		if slices.ContainsFunc(req.Status.Approvals[:i], func(o protemv1alpha1.ApprovalDecision) bool {
			return o.User == d.User && o.Decision == d.Decision && o.At.Equal(&d.At)
		}) {
			continue
		}
		switch d.Decision {
		case protemv1alpha1.ApprovalDecisionApproved:
			if d.User == autoApprover {
				automatic = true
				continue
			}
			add("APPROVAL_GIVEN", d.At.Time, d.User, d.Comment)
		case protemv1alpha1.ApprovalDecisionRejected:
			rejectedByPerson = true
			add("REQUEST_REJECTED", d.At.Time, d.User, d.Comment)
		case protemv1alpha1.ApprovalDecisionCancelled:
			add("REQUEST_CANCELLED", d.At.Time, d.User, d.Comment)
		}
	}

	conditions := req.Status.Conditions
	decidedCond := meta.FindStatusCondition(conditions, "Decided")
	switch req.Status.Phase {
	case protemv1alpha1.RequestPhaseApproved:
		if decidedCond != nil {
			comment := ""
			if automatic {
				comment = "No approval needed"
			}
			add("REQUEST_APPROVED", decidedCond.LastTransitionTime.Time, "", comment)
		}
	case protemv1alpha1.RequestPhaseRejected:
		cond := decidedCond
		if v := meta.FindStatusCondition(conditions, "Validated"); v != nil && v.Status == "False" {
			cond = v
		}
		if !rejectedByPerson && cond != nil {
			add("REQUEST_REJECTED", cond.LastTransitionTime.Time, "", cond.Message)
		}
	case protemv1alpha1.RequestPhaseTimedOut:
		if decidedCond != nil {
			add("REQUEST_TIMED_OUT", decidedCond.LastTransitionTime.Time, "", "")
		}
	}

	if grant != nil {
		onGrant := func(e *Event) { e.Grant, e.Clusters = grant.Name, grant.Spec.Clusters }
		add("ACCESS_GRANTED", grant.CreationTimestamp.Time, "", "", onGrant, func(e *Event) { e.Duration = duration(req) })
		if rev := grant.Spec.Revocation; rev != nil {
			add("ACCESS_REVOKED", rev.At.Time, rev.By, rev.Reason, onGrant)
		} else if !now.Before(grant.Spec.ExpiresAt.Time) {
			add("ACCESS_EXPIRED", grant.Spec.ExpiresAt.Time, "", "", onGrant)
		}
	}
	return events
}

func duration(req *protemv1alpha1.AccessRequest) string {
	if req.Status.Resolved != nil {
		return req.Status.Resolved.Duration.Duration.String()
	}
	if req.Spec.Duration != nil {
		return req.Spec.Duration.Duration.String()
	}
	return ""
}
