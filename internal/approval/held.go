package approval

import (
	"fmt"
	"slices"
	"time"

	v1 "github.com/iLLeniumStudios/protem/api/v1alpha1"
)

// Held explains why req should not go ahead: its requester already waits for, or holds, the same
// profile on one of clusters. It returns "" when nothing stands in the way.
//
// clusters is what req would target; none means every cluster of the profile. requests and grants
// are the others in req's namespace. Between two requests not validated yet the older one wins, so
// two sent at the same moment cannot both pass; a req that is not created yet counts as the newest.
func Held(req *v1.AccessRequest, clusters []string, requests []v1.AccessRequest, grants []v1.AccessGrant, now time.Time) string {
	who := Person(req.Spec.Requester)
	shared := func(other []string) string {
		switch {
		case len(clusters) == 0 && len(other) == 0:
			return "its clusters"
		case len(clusters) == 0:
			return other[0]
		case len(other) == 0:
			return clusters[0]
		}
		for _, c := range clusters {
			if slices.Contains(other, c) {
				return c
			}
		}
		return ""
	}
	older := func(o *v1.AccessRequest) bool {
		if req.CreationTimestamp.IsZero() {
			return true
		}
		if c := o.CreationTimestamp.Compare(req.CreationTimestamp.Time); c != 0 {
			return c < 0
		}
		return o.Name < req.Name
	}

	for i := range grants {
		g := &grants[i]
		ended := g.Spec.Revocation != nil || !now.Before(g.Spec.ExpiresAt.Time) ||
			g.Status.Phase == v1.GrantPhaseExpired || g.Status.Phase == v1.GrantPhaseRevoked
		if ended || g.Spec.RequestRef == req.Name || g.Spec.ProfileRef != req.Spec.ProfileRef || !SamePerson(who, Person(g.Spec.Requester)) {
			continue
		}
		if on := shared(g.Spec.Clusters); on != "" {
			return fmt.Sprintf("This profile is already granted on %s until %s (%s). Revoke it first to request it again.",
				on, g.Spec.ExpiresAt.UTC().Format("2006-01-02 15:04 MST"), g.Name)
		}
	}

	for i := range requests {
		o := &requests[i]
		if o.Name == req.Name || o.Spec.ProfileRef != req.Spec.ProfileRef || !SamePerson(who, Person(o.Spec.Requester)) {
			continue
		}
		switch o.Status.Phase {
		case v1.RequestPhasePending:
		case v1.RequestPhaseApproved:
			if o.Status.GrantRef != "" {
				continue
			}
		case "":
			if !older(o) {
				continue
			}
		default:
			continue
		}
		target := o.Spec.Clusters
		if o.Status.Resolved != nil {
			target = o.Status.Resolved.Clusters
		}
		if on := shared(target); on != "" {
			return fmt.Sprintf("A request for this profile on %s is already waiting for a decision (%s). Cancel it first to send a new one.", on, o.Name)
		}
	}
	return ""
}
