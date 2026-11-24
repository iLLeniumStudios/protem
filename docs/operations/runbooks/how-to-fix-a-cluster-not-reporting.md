# How to fix a cluster shown as not reporting

Diagnose and recover a target cluster the hub shows as disconnected.

**Notice:** the Clusters page shows **Waiting for agent** or **Not reporting** for a cluster, or
the Access timeline badges it **Agent not reporting**.

**Confirm.** The hub's `TargetCluster` condition names which case it is:

```sh
kubectl --context hub get targetcluster <name> -o jsonpath='{.status.conditions}'
```

- `Reason: AwaitingFirstHeartbeat` — the agent has never renewed its Lease on the hub. It either
  was never deployed, or cannot reach the hub at all.
- `Reason: HeartbeatTimeout` — the agent was connected but its Lease has not been renewed for
  more than 90 seconds (the hub's fixed heartbeat timeout).

Check the Lease itself for the last renewal time:

```sh
kubectl --context hub -n protem-cluster-<name> get lease agent -o jsonpath='{.spec.renewTime}'
```

**Act.**

- If the agent was never deployed, or its Deployment is missing or crash-looping on the target
  cluster, follow [How to connect a target cluster](../../guides/how-to-connect-a-target-cluster.md)
  from "What gets installed on the target cluster" on.
- If the agent is running (`kubectl --context <cluster> -n protem-system get pods`) but still
  not reporting, check its logs for "Unauthorized" — see
  [How to fix an agent logging Unauthorized](how-to-fix-agent-unauthorized.md).
- If the agent is running and its logs show no error, check that the target cluster can reach
  the hub at the `server` address in `protem-agent-hub`'s kubeconfig (network policy, firewall,
  DNS).

Access already granted on that cluster keeps ending on time regardless: the agent removes
expired bindings on its own 5-second sweep whether or not it can reach the hub.
