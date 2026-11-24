# How to limit the roles a cluster allows

Set the ceiling of roles the hub may ever bind on a target cluster, and on the hub itself. Why
this ceiling exists, and who enforces it, is in
[Hub and agents](../architecture/hub-and-agents.md#why-each-cluster-has-a-role-ceiling).

## On a target cluster

Edit the `bind` rule's `resourceNames` in `config/agent/spoke/cluster_role.yaml`:

```yaml
  resourceNames:
  - view
  - edit
  - db-operator
```

Apply the change on the target cluster the same way you deployed the agent — through your
overlay's kustomize build, or by editing your own copy of `config/agent/spoke` if
`--agent-manifests` points at one. No AccessProfile can cause a binding for a role outside this
list on that cluster: the target cluster's own API server refuses the `bind` and the agent
reports the grant `Failed`.

## On the hub itself

If the hub is also a target cluster (its agentless entry, `hubClusterName`), its own ceiling is
`config/rbac/hub_bindable_roles.yaml`'s `resourceNames`, or the chart value `hubAllowedRoles`
(default `[view, edit]`; `[]` grants nothing there). Change it in your Helm values or your
kustomize overlay, the same way as any other chart value or manifest.

See [Custom resources](../reference/custom-resources.md#targetcluster) and
[Chart values](../reference/chart-values.md) for the surrounding fields.
