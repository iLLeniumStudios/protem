# Hub and agents

Why an agent pulls its grants from the hub instead of the hub pushing them, and what that choice costs and buys.

For what an AccessGrant and a ClusterGrant are, see [Concepts](concepts.md); for the path a request takes through them, see [How it works](how-it-works.md).

## Why agents pull

The hub never opens a connection to a target cluster, and holds no credential for one. `SyncOnce` in `internal/enforcer/agent.go` runs on the agent: it lists the ClusterGrants in its own namespace on the hub and applies them locally, every `--sync-interval` (3 seconds by default). The hub never calls out to reach a cluster.

This is a deliberate trade against a more common push design, where a controller on the hub would hold a kubeconfig per target cluster and apply bindings there directly:

- **What it buys:** a target cluster's credentials never leave that cluster, and the hub cannot be made to reach a cluster it was not meant to reach — not by a bug, and not by whoever compromises the hub. The [security model](security-model.md) builds on exactly this: a stolen hub can mint grants, but it still cannot connect anywhere.
- **What it costs:** the hub cannot push a revocation; it can only wait for the agent to come back and read it. A disconnected cluster keeps whatever it already applied until it reconnects or the access expires on its own (below). A push design would let a hub-initiated revocation land immediately, at the cost of the hub holding every cluster's credentials.

## What an agent can read

An agent's hub credential is a ServiceAccount token scoped to one namespace, `protem-cluster-<name>`, created for it there (`config/agent/hub/role.yaml`). Inside that namespace it may `list` ClusterGrants, `update` their status, and manage its own Lease and its own renewed token. It cannot read any other cluster's namespace, any AccessRequest, or any AccessProfile. A token taken from one agent therefore discloses only that one cluster's current grants — who holds which role until when — never the justification or ticket behind them, and gives no access to any other cluster or to the hub's own.

A cluster is `Ready` on the hub once its agent's Lease has been renewed within the last 90 seconds (`HeartbeatTimeout` in `internal/controller/targetcluster_controller.go`); before the first heartbeat it is `Disconnected` with reason `AwaitingFirstHeartbeat`, and after 90 seconds without one it is `Disconnected` with reason `HeartbeatTimeout`. [Request lifecycle](request-lifecycle.md) covers what a request does while its target cluster is not yet ready.

## How the agent keeps its hub credential fresh

The agent requests a new token for its own ServiceAccount from the hub once a third of the
current token's lifetime has passed, and uses it from the next request on. `--hub-token-ttl`
sets the lifetime it asks for (default `24h`, so a new token every 8 hours; at least `10m`). The
hub may grant less; the agent schedules from what it got. A failed renewal is retried after 30
seconds, then with growing pauses up to 10 minutes, while the agent keeps the current token.

The Deployment passes `--hub-token-secret=protem-system/protem-agent-hub`: each new token is
written to the key `token` of that Secret, next to `kubeconfig`, and at start the agent uses it
instead of the kubeconfig's token as long as it has not expired. To start again from a new first
token, replace `kubeconfig` and delete the `token` key.

If the hub stays unreachable for longer than the lifetime, the token expires before the agent can
renew it, and the agent cannot reach the hub again on its own: a new first token has to be minted
and put in the Secret (see [How to connect a target cluster](../guides/how-to-connect-a-target-cluster.md)).
The expired `token` key is ignored. Access on the cluster still ends on time meanwhile (see below).

A token taken from the agent is valid for at most the lifetime, and only for what the agent may
do in its own namespace `protem-cluster-<name>` on the hub.

If the hub kubeconfig authenticates some other way (a client certificate, an exec plugin, a token
file), the agent logs that token renewal is off and uses the kubeconfig as it is.

## What happens when the hub is unreachable

Expiry still works: every binding carries its expiry time, and the agent deletes expired
bindings, and bindings whose expiry it cannot read, without asking the hub. A revocation,
however, is a change on the hub; it cannot reach the cluster until the agent can read the hub
again. Until then a revoked binding stays until it expires.

## Why each cluster has a role ceiling

A target cluster's own API server, not the hub, decides which roles an agent may ever bind there: the agent's ClusterRole lists the allowed role names under its `bind` rule's `resourceNames` (`config/agent/spoke/cluster_role.yaml`), and Kubernetes RBAC itself refuses a `bind` for anything outside that list. The hub carries the equivalent list for itself, in case the hub is also a target cluster (`config/rbac/hub_bindable_roles.yaml`).

The ceiling exists because the agent acts on instructions it reads from the hub, and a cluster's owner should not have to trust the hub, or whoever administers it, with every role on their cluster. Someone who takes over the hub's operator, or an agent, can issue grants for any role an AccessProfile names — but they still cannot cause a binding for a role the cluster owner never added to the ceiling. Each cluster owner decides, independently, what the hub is allowed to ever hand out on their cluster, and can extend that list only by changing `resourceNames` themselves. See [How to connect a target cluster](../guides/how-to-connect-a-target-cluster.md#5-set-the-ceiling) for how to change it.

A grant for a role outside the ceiling fails on the target cluster's own API server with a `forbidden` error; a grant for a role that does not exist there fails with `RoleNotFound`. Either way, no binding is created, and the failure is visible on the grant's per-cluster status.
