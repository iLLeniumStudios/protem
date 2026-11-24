# Requests freeze their scope at creation

An `AccessRequest` resolves its role, scope, clusters, namespaces and duration once, when it first passes validation, and never recomputes them from the profile again.

**Status:** Accepted, 2026-11-24.

## Context

`AccessProfile` is a reusable object: the same profile is requested by many people over time, and its owner edits it as needs change. If a grant were built from whatever the profile currently says at approval or issue time, an edit between a request's creation and its approval could change what gets approved without the approver seeing it: the implementation review's R2 finding describes a profile edited from `view` to `cluster-admin` after a `view` request was approved, issuing `cluster-admin` on the strength of the old approval.

The alternative, recomputing from the live profile at grant time, is simpler to implement but makes every approval provisional on nobody editing the profile in between.

## Decision

`validate()` in `internal/controller/accessrequest_lifecycle.go` resolves the profile once and writes the result into `req.Status.Resolved` (`Role`, `Scope`, `Clusters`, `Namespaces`, `Duration`, and the per-cluster `Requirements` used for approval): the "Validated" condition's `ScopeResolved` reason marks this point. Every later step reads only `status.resolved`:

- `review()` evaluates approvals against the frozen `Requirements`, not against the profile.
- `issue()` builds the `AccessGrant`'s `Role`, `Scope`, `Namespaces`, `Clusters` and `Duration` from `resolved`, and rejects the request with reason `UnresolvedScope` if any of them is missing.

A profile edit after a request is validated has no effect on that request.

## Consequences

- An approver reviews exactly what will be granted; a later profile edit cannot change an already-decided request's outcome.
- A request made before a profile existed, or before it resolved any eligible cluster, carries no frozen scope and is rejected rather than issued with a partial one.
- Changing a profile to fix a mistake does not retroactively fix or break requests already in flight; a requester who needs the new shape must submit a new request.
- `status.resolved` is the single source `issue()` reads from; a bug that leaves it unset is caught by the `UnresolvedScope` rejection instead of silently granting an empty or stale scope.
