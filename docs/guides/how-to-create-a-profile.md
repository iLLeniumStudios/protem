# How to create a profile

Define a role people can request, where it applies, who may request it, and who approves it —
from Git as a plain manifest, or from the UI.

## From a manifest

```yaml
apiVersion: protem.dev/v1alpha1
kind: AccessProfile
metadata:
  name: db-edit
  namespace: default
spec:
  displayName: Database edit
  role:
    kind: ClusterRole
    name: edit
  scope: Namespaced
  namespaces:
  - payments
  clusters:
    selector:
      matchLabels:
        team: payments
  duration:
    default: 30m
    max: 4h
  requesters:
    groups:
    - payments-devs
    users:
    - bot@company.com
  justification:
    reason: Required
  approval:
    pendingTimeout: 24h
    rules:
    - name: dev-self-service
      when:
        clusterSelector:
          matchLabels:
            env: dev
      mode: Auto
    - name: prod-needs-lead
      when:
        clusterSelector:
          matchLabels:
            env: prod
      mode: Manual
      approvers:
        groups:
        - payments-leads
      requiredApprovals: 1
```

This profile targets every cluster labelled `team: payments`. Rules are checked in order; the
first whose `when` matches a resolved cluster decides for it, so here a `payments` cluster also
labelled `env: dev` is self-service, and one labelled `env: prod` needs a `payments-leads`
approval. A rule with no `when` matches every cluster, so put a catch-all rule last if you want
one. `requesters` lists who may request this profile at all: `groups`, `users`, or both — at
least one is required.

Apply it like any manifest: `kubectl apply -f db-edit.yaml`. Every field, its constraints and
what each one means is in [Custom resources](../reference/custom-resources.md#accessprofile) and
[Concepts](../architecture/concepts.md#accessprofile).

## From the UI

This needs `--ui-management=enabled` (chart value `uiManagement: enabled`), and `create` (to add
a profile) or `update` (to edit one) on `accessprofiles` in the namespace.

1. Open **Profiles**, choose **Create profile**, and pick the namespace the profile lives in.
2. Fill in **Role and scope** (the Role or ClusterRole and the namespaces, or Cluster scope),
   **Clusters** (by name or by label selector), **Request settings** (requester groups and users,
   default and longest duration, reason and ticket pattern requirements), and **Approval**
   (pending timeout, and one or more rules in order).
3. For each rule, set its cluster labels and requester groups/users to match on (leave empty to
   match everything), **Auto** or **Manual** mode, and for Manual: approvers, how many approvals
   are required, and whether a requester may approve their own request.
4. Choose **Create profile**.

A profile created from Git (not through the UI) shows as managed, and the UI opens it read-only;
edit it in Git instead.
