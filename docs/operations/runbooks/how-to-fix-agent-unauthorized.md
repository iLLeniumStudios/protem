# How to fix an agent logging Unauthorized

Recover an agent whose hub token has stopped working.

**Notice:** the agent's logs show a 401/Unauthorized error talking to the hub, or
`enforcerLog` lines like "Failed to renew the hub token" repeating with growing backoff.

**Confirm.**

```sh
kubectl --context <cluster> -n protem-system logs deployment/protem-agent | tail -n 50
```

or, from the hub side, check how stale the agent's Lease is (see
[How to fix a cluster shown as not reporting](how-to-fix-a-cluster-not-reporting.md) for the
exact command) — an agent that cannot authenticate to the hub stops renewing it too.

**Act.** Reissue the agent's hub credential:

- From the Fleet/Clusters page: open the cluster's actions menu and choose **Reissue
  credentials**, then apply the new files it gives you on the target cluster.
- By hand: delete and recreate the agent's ServiceAccount on the hub
  (`config/agent/hub/service_account.yaml`), which cuts off every token issued for it, then mint
  a new first token and redeploy as in
  [How to connect a target cluster](../../guides/how-to-connect-a-target-cluster.md).

Either way is the same recovery as [Restore an agent whose hub token
expired](../../guides/how-to-connect-a-target-cluster.md#restore-an-agent-whose-hub-token-expired),
which has the full steps; this page is only the fast path to it.
