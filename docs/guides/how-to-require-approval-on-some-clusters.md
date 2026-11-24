# How to require approval on some clusters

Make one profile self-service on some clusters and require approval on others, by giving it
more than one approval rule.

## From a manifest

```yaml
apiVersion: protem.dev/v1alpha1
kind: AccessProfile
metadata:
  name: prod-admin
  namespace: default
spec:
  displayName: Production admin
  role:
    kind: ClusterRole
    name: edit
  scope: Namespaced
  namespaces:
  - default
  clusters:
    names:
    - prod-eu
    - prod-us
  duration:
    default: 30m
    max: 2h
  requesters:
    groups:
    - sre
  justification:
    reason: Required
    ticketPattern: "^INC-[0-9]+$"
  approval:
    rules:
    - name: prod-eu-needs-two
      when:
        clusterSelector:
          matchLabels:
            env: prod
      mode: Manual
      approvers:
        groups:
        - sre-leads
      requiredApprovals: 2
      allowSelfApproval: false
```

`clusters` picks which clusters the profile can be requested on at all, by name or by label
selector; each rule's own `when.clusterSelector` (or `when.requesterGroups`/`when.requesterUsers`)
decides which of those clusters, and which requesters, the rule applies to. Rules are checked in
order and the first match per cluster wins, so put a narrower rule before a broader one and a
catch-all (a rule with no `when`) last. `requiredApprovals` sets how many distinct people must
approve; `allowSelfApproval` (default `false`) lets a requester who is also a named approver
approve their own request. A cluster that matches no rule rejects the request.

## From the UI

In the profile editor's **Approval** section, add rules with **Add rule**, and use the move and
remove controls to put them in the order you want. Each rule has its own cluster labels and
requester groups/users, **Auto** or **Manual** mode, and, for Manual, approvers, **Approvals
required** and **Allow self-approval**.

Every field's constraints are in
[Custom resources](../reference/custom-resources.md#accessprofile); see also
[How to create a profile](how-to-create-a-profile.md).
