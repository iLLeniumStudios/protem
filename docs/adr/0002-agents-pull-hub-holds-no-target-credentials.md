# Agents pull; the hub holds no target cluster credentials

The hub never connects to a target cluster. Each cluster's agent connects to the hub instead, reads its own grants, and applies them locally.

**Status:** Accepted, 2026-11-23.

## Context

A hub managing many clusters' access needs some channel between the hub and each cluster. The common shape for this (breakglass, kube-jit) is the hub holding an admin kubeconfig per cluster and pushing bindings into it. That kubeconfig is the biggest single thing worth stealing: whoever reads it gets standing access to that cluster, with no expiry and no record of its use.

The alternative is a pull model: something on the target cluster watches the hub for its own cluster's grants and applies them there, authenticating to the hub rather than the hub authenticating to it.

## Decision

Each target cluster runs an agent (`cmd/enforcer`, `internal/enforcer/agent.go`) that connects outward to the hub's Kubernetes API, authenticating as a ServiceAccount scoped to its own namespace (`protem-cluster-<name>`, see `config/agent/hub/role.yaml`: `list` on `clustergrants`, `update` on `clustergrants/status`, and lease renewal only). `SyncOnce` lists `ClusterGrant` objects in that namespace and reconciles RoleBindings and ClusterRoleBindings on the target cluster with its own client (`SpokeClient`). No code in `internal/controller` or `internal/api` holds a `rest.Config` or kubeconfig pointed at a target cluster; the only cross-cluster client construction outside the agent is the join/reissue flow in `internal/api/enrol.go`, which mints the agent's own hub-scoped token, never a credential for the target cluster.

The agent renews its own hub token (`internal/enforcer/hubtoken.go`) after a third of its lifetime has passed, so the hub never needs to push a renewed credential to it either.

## Consequences

- Compromising the hub's operator gives no path to any target cluster's API: see the security model's "Someone who takes over the hub's operator" row.
- Stealing one agent's hub token exposes only that cluster's grants (who holds which role, not tickets or reasons) and lets the holder renew that token and report false status, but nothing about any other cluster, request or profile.
- A target cluster that cannot reach the hub keeps whatever bindings it already applied; it cannot learn about a new grant or a revocation until the connection is restored ([ADR 0005](0005-enforce-expiry-on-the-target-cluster.md) covers what still happens locally during that gap).
- Onboarding a cluster means installing the agent and giving it a join token, rather than handing the hub a kubeconfig; see [how to connect a target cluster](../guides/how-to-connect-a-target-cluster.md).
