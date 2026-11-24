# Enforce expiry on the target cluster

A grant's bindings are removed by the agent running on the target cluster itself, on a timer that keeps running whether or not the hub is reachable.

**Status:** Accepted, 2026-11-23.

## Context

RBAC bindings do not expire on their own; something has to delete them when a grant's `expiresAt` passes. If that something is the hub, driven by its own clock, then a hub outage or a network partition between the hub and a target cluster means access the hub meant to end keeps working until the connection is restored: the system fails open exactly when it is least observable.

## Decision

The agent removes expired bindings locally, independent of the hub. `autonomousSweepLoop` in `internal/enforcer/agent.go` runs `sweepExpired` every 5 seconds; `sweepExpired` calls `binding.Sweep` with a `keep` function that checks only the `protem.dev/expires-at` annotation stamped on each binding against the agent's own clock (`now.Before(expiresAt)`), with no call to the hub in the loop. This runs alongside, not instead of, `SyncOnce`'s own per-grant expiry check against the `ClusterGrant` it last read from the hub.

## Consequences

- Access still ends on time even if the hub is down, unreachable, or the agent cannot renew its hub token: this is why the security model calls expiry "fail-closed on the agent", in contrast to new approvals and revocations, which cannot happen at all without the hub.
- A revocation (as opposed to a natural expiry) still needs the agent to read the hub at least once after the revocation is recorded; it is not enforced by the local timer, only expiry is.
- Expiry is judged by the target cluster's own clock against the `expiresAt` timestamp stamped into the binding's annotation at apply time. Clock skew between the hub and a target cluster shows up as access ending early or late on that cluster; keeping clocks synchronised is listed as an operator responsibility in the security model.
- The sweep only ever deletes bindings labelled with Protem's own `protem.dev/managed-by` owner; it does not touch anything it did not create.
