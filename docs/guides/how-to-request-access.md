# How to request access

Ask for time-boxed access to one or more clusters through the hub UI, or by creating an
AccessRequest directly.

## From the UI

1. Open **Activate access** (`/request`). It lists every profile that names your groups or you
   as a requester.
2. Each profile shows how it is decided: **Granted at once** needs no approval, **Approval by
   <names>** (or **Needs approval** if none are named) means someone has to approve it, and
   **Approval on some clusters** means it depends which clusters you pick. Choose **Activate
   access** or **Request approval** on the profile you want.
3. On the request form, pick the clusters (only the profile's ready clusters are offered),
   a duration up to the profile's longest allowed, a ticket if the profile requires one, and a
   reason if the profile requires one — approvers and the audit log see it.
4. Submit. A self-service request goes live at once; others wait for approval.

See the result on **For you** (if it's waiting or active) or **My requests** (every request you
have sent).

## From a manifest

```yaml
apiVersion: protem.dev/v1alpha1
kind: AccessRequest
metadata:
  name: alice-dev-view
  namespace: default
spec:
  profileRef: dev-view
  requester:
    username: alice
    email: alice@company.com
    groups:
    - devs
  clusters:
  - prod-eu
  duration: 30m
  justification:
    reason: Checking a failing rollout.
```

`spec` is immutable once created; to change anything, create a new request. `clusters` and
`duration` may be left out to use everything the profile allows. Field details are in
[Custom resources](../reference/custom-resources.md#accessrequest).

If the cluster's admission policy is installed, the API server accepts this manifest only from a
caller whose own identity matches `spec.requester`, unless the caller holds the `delegate` verb —
see [How to enforce requester identity for kubectl requests](how-to-enforce-requester-identity-for-kubectl-requests.md).
