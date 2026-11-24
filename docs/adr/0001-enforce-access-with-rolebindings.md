# Enforce access with RoleBindings

Protem turns each grant into ordinary RoleBindings or a ClusterRoleBinding on the cluster, not into answers from an authorization webhook. This page records why.

**Status:** Accepted, 2026-11-23.

## Context

An approved grant has to become permission that a cluster's API server honours. There are two ways to do that in Kubernetes:

- **RBAC bindings.** Create a RoleBinding or ClusterRoleBinding that names the person and the role, and delete it when the grant ends. The API server's own RBAC evaluates every call.
- **An authorization webhook.** Configure the API server with `--authorization-config` so that it asks a webhook about calls RBAC has already denied, and let the webhook allow calls covered by an active grant. Expiry then takes effect on the next request, and nothing per person is written to the cluster.

The webhook needs access to the API server's flags, which EKS, GKE and AKS do not give. It also sits in the request path of every denied call, so it has to be highly available and fast. And it is unsafe without extra cluster configuration: identity-provider groups that collide with its synthetic groups have to be rejected, and the authorizer's cache of allowed decisions has to be disabled. Bindings work on any conformant cluster, but they stay in place until something deletes them, and whoever creates them needs the `bind` permission for each role.

The design history compares the two in [landscape and review, section 6](../contributing/design/2026-11-23-landscape-and-review.md).

## Decision

Access is enforced with RBAC bindings only. `internal/binding` writes one RoleBinding named `protem-grant-<key>` in each namespace of a namespaced grant, or one ClusterRoleBinding for a grant with `scope: Cluster`. Each binding carries the labels `protem.dev/managed-by`, `protem.dev/grant` and `protem.dev/grant-namespace` and the annotation `protem.dev/expires-at`. The agent writes these bindings on a target cluster, and the operator writes them on the hub when the hub is itself a target. Neither one evaluates RBAC rules itself.

Request and approval code does not know how access is enforced. Enforcement reads only the grant, so a second mechanism could be added later without changing either.

`TargetCluster.spec.enforcement` accepts `RoleBinding` (the default) and `Authorizer`. Only `RoleBinding` is implemented. No component reads the field, so setting `Authorizer` has no effect.

## Consequences

- Protem needs no control-plane access, so it runs on managed and self-managed clusters alike. That is the reason for the "Plain RBAC" line in the README.
- Access in force is visible with `kubectl get rolebindings,clusterrolebindings -l protem.dev/managed-by`.
- The component that creates bindings needs `bind` on each role it grants. The cluster owner limits that with `resourceNames`, which becomes the per-cluster ceiling described in the [security model](../architecture/security-model.md).
- A binding outlives its grant until something deletes it. [ADR 0005](0005-enforce-expiry-on-the-target-cluster.md) covers how expiry is enforced anyway.
- The ceiling lists role names, so a cluster owner who lists `edit` trusts whatever `edit` contains on that cluster. Rules finer than RBAC, such as resource-name prefixes, are not possible.
- Removing a binding stops new calls, but an `exec`, `port-forward` or watch that is already open keeps running. A webhook would not change this.
- `apply` refuses to adopt an existing binding with the same name that another owner or grant labelled, so Protem never takes over a binding it did not create.
