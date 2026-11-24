# How to fix a grant that is Failed on a cluster

Find out why a grant's binding failed on one of its target clusters, and fix the cause.

**Notice:** a grant's detail page shows a cluster as `Failed` instead of `Active`, with a message
under it.

**Confirm.** Read the ClusterGrant's status directly:

```sh
kubectl --context hub -n protem-cluster-<name> get clustergrant <namespace>.<grant-name> -o yaml
```

`.status.message` names the cause:

- A `forbidden` error from the target cluster's own API server — the role is real but outside
  the ceiling in `config/agent/spoke/cluster_role.yaml`'s `resourceNames` on that cluster. Fix
  with [How to limit the roles a cluster allows](../../guides/how-to-limit-the-roles-a-cluster-allows.md).
- `RoleNotFound: ClusterRole "x" does not exist on this cluster` (or `Role "x" does not exist in
  namespace "y"`) — the role the profile names was never created on that target cluster. Create
  it there, or point the profile at a role that exists.

In both cases no binding is created on the cluster; access is simply absent there, not partially
applied.

**Act.** Once the ceiling is extended or the role exists on the target cluster, no retry is
needed: the agent's next sync pass (its `--sync-interval`, default 3s) picks the grant up and
reports `Active`.
