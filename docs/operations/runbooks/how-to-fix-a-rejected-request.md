# How to fix a request rejected with ClusterNotReady or ResolutionFailed

Find out why a request was auto-rejected before anyone decided it, and fix the cause.

**Notice:** a request ends `Rejected` within a minute or so of being created, with no approval
decision recorded against it, and its `Validated` condition reason is `ClusterNotReady` or
`ResolutionFailed`.

**Confirm.** Read the condition and its message:

```sh
kubectl --context hub get accessrequest <name> -o jsonpath='{.status.conditions}'
```

- `Reason: ClusterNotReady` — a cluster the request targeted was still not `Ready` after a
  one-minute grace period. The message names the cluster(s) waiting, e.g. `no cluster of profile
  "prod-view" is ready (prod-eu, prod-us)`, or, for an explicitly requested cluster, `requested
  cluster "prod-eu" is not ready`.
- `Reason: ResolutionFailed` — the profile itself could not be resolved against the fleet. Common
  messages: `profile "x" has neither clusters.selector nor clusters.names configured` (profile
  misconfigured), `requested cluster "x" is not permitted by profile "y"` (the request named a
  cluster outside the profile's `clusters`), `requested namespace "x" is not permitted by profile
  "y"`, or `cluster "x" matched no approval rules in profile "y"` (every rule's `when` excluded
  that cluster — add a catch-all rule).

**Act.**

- For `ClusterNotReady`: check why the cluster isn't `Ready` — see
  [How to fix a cluster shown as not reporting](how-to-fix-a-cluster-not-reporting.md). Once it
  reconnects, the requester sends a new request; this one is already finished.
- For `ResolutionFailed`: fix the profile (its `clusters`, approval rules, or namespace list) per
  the message, following
  [How to create a profile](../../guides/how-to-create-a-profile.md) or
  [How to require approval on some clusters](../../guides/how-to-require-approval-on-some-clusters.md),
  then have the requester send a new request.
