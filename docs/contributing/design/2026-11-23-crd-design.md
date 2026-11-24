# CRD design review and proposed v1alpha2 shape

Companion to `jit-access-review.md`. That doc covers behaviour bugs; this one
covers the **API**: each CRD's fields, and the authorization model for running
protem as the central access point for a fleet.

## 1. Who decides what

A central Protem has six actors. Most confusion in access tools comes from
deciding all six in one place. Split them by *what kind* of decision it is:

| Actor | Decides | Expressed in | Enforced by |
|---|---|---|---|
| Platform admin | clusters, CRDs, fleet-wide floors | hub RBAC on all `protem` resources | hub API server |
| Profile owner | what can be requested, by whom, approved by whom | hub RBAC `create/update` on `accessprofiles` in a namespace | hub API server |
| Spoke owner | the **ceiling**: which roles the hub may ever hand out on their cluster | spoke-agent RBAC: `bind` with `resourceNames` | spoke API server |
| Requester | asks | `AccessProfile.spec.requesters` | operator (identity stamped at admission) |
| Approver | approves/rejects | `AccessProfile.spec.approval.rules[].approvers` | operator |
| Auditor | reads | hub RBAC `get/list` | hub API server |

Rule of thumb: **configuration authz is Kubernetes RBAC; workflow authz
(request/approve) is profile fields.** Admin-ness is "can this user `update`
`accessgrants` in this namespace" (a SubjectAccessReview from the web API), not
a Keycloak `realm_access.roles` claim (`internal/api/auth.go`), which only works
with Keycloak.

The spoke-owner row is what makes a central hub safe: the hub is tier-0, and
the only thing that stops a compromised hub from granting `cluster-admin`
everywhere is the spoke agent's own RBAC. Granting the agent
`bind` only on `resourceNames: [view, edit, protem-debug]` is a native,
per-cluster ceiling that the hub cannot override.

## 2. Approval model: approvers per cluster set and per role

What you described maps onto the profile like this:

- **Per role**: one profile = one role + scope. Approvers live on the profile.
- **Per cluster set**: approval is a list of **rules**, matched per target
  cluster by cluster labels (and optionally requester group or user).
- **Self-elevating**: a rule with `mode: Auto`.
- **Approval required**: `mode: Manual` with `approvers` and `requiredApprovals`.
- **Break-glass**: `mode: Auto` plus `notify` (approvers are paged and review after the fact).

```yaml
apiVersion: protem.dev/v1alpha2
kind: AccessProfile
metadata:
  name: payments-edit
  namespace: team-payments          # see 4.1
  labels:
    protem.dev/tier: write   # lets fleet-wide policies select profiles (section 3)
spec:
  displayName: Payments edit
  description: Edit access to payments namespaces
  role: {kind: ClusterRole, name: edit}
  scope: Namespaced                  # Namespaced | Cluster; Cluster requires role.kind=ClusterRole
  namespaces: [payments, payments-jobs]  # required iff Namespaced; requests pick a subset
  clusters:                          # exactly one of selector | names; empty is invalid
    selector: {matchLabels: {team: payments}}
  duration: {default: 1h, max: 8h}   # metav1.Duration
  requesters:                        # at least one group or user
    groups: [payments-devs, sre]
    users: [oncall@example.com]      # usernames or emails
  justification:
    reason: Required                 # Required | Optional
    ticketPattern: '^PAY-[0-9]+$'
  approval:
    pendingTimeout: 24h              # unapproved requests expire; no stale approvals
    rules:                           # first match per target cluster; no match = rejected
    - name: sre-self-service
      when: {requesterGroups: [sre], requesterUsers: [lead@example.com]}
      mode: Auto
      maxDuration: 2h                # rule can only tighten profile max
    - name: non-prod
      when: {clusterSelector: {matchExpressions: [{key: env, operator: NotIn, values: [prod]}]}}
      mode: Auto
    - name: prod
      when: {clusterSelector: {matchLabels: {env: prod}}}
      mode: Manual
      approvers: {groups: [payments-leads], users: []}
      requiredApprovals: 1
      allowSelfApproval: false       # default false; requester's own approval never counts
      notify: [slack-payments]       # optional sink reference, later
```

`requesters.groups` and `requesters.users` are alternatives: a person may request the profile when they are in one of the groups or their username or email is one of the users, and the profile must name at least one of either. A rule's `when.requesterGroups` and `when.requesterUsers` match the requester the same way (either list suffices, neither set matches every requester), and an empty entry never matches anyone.

Evaluation, which must be specified exactly because it is the security boundary:

1. Resolve the request's clusters and namespaces **at creation** (subset of
   the profile's), write them to `status.resolved`, and freeze them. Approvers
   approve exactly what they see; a new prod cluster joining mid-request does
   not widen it. Today resolution happens at grant creation and its error is
   discarded (`accessrequest_controller.go:189`, `targetClusters, _ :=`). A
   request naming a disallowed cluster therefore yields a grant with *no*
   clusters, which the hub then serves to every spoke (`jit-access-review.md` §7).
2. For each resolved cluster, pick the first rule whose `when` matches (all set
   conditions must hold; an empty `when` matches everything).
3. The request is **Approved** when every cluster's rule is satisfied: `Auto`
   is satisfied at once; `Manual` needs `requiredApprovals` distinct eligible
   approvals. Any eligible rejection → **Rejected**. In practice the strictest
   cluster governs a mixed dev+prod request.
4. Grant duration = min(request, profile max, every matched rule's `maxDuration`).

## 3. Guardrails above profiles: model, use cases, options

### The model

Two layers, owned by different people:

- **Profile rules** (section 2) are *the profile author's choice*: "on prod,
  payments-leads approve".
- **Guardrails** are *the organisation's minimums*, which a profile author
  cannot weaken: "any admin-tier access on prod also needs security", "nothing
  on prod is self-service", "team X's profiles may only touch team X's
  clusters and only `view`/`edit`".

Guardrails only matter when the person writing profiles is not the person
accountable for the minimums. If one platform team writes every profile through
reviewed PRs, the PR review *is* the guardrail.

### Use cases

| Organisation | Who writes profiles | Guardrails needed? |
|---|---|---|
| A. One platform team, profiles in a GitOps repo | Platform team | No. PR review + hub RBAC on `accessprofiles` |
| B. As A, plus compliance (SOC 2 / ISO 27001: "privileged prod access needs a second person") | Platform team | Nice to have: a machine check that no profile violates the rule, instead of trusting reviewers |
| C. Delegated: each team lead owns profiles in their hub namespace (enabled by namespaced profiles, decision 1) | Team leads | **Yes**: bounds on roles/clusters per namespace, and org floors teams can't remove |
| D. Managed-service / multi-customer platform | Customers or their admins | Yes, and they must be exact |

### Two ways to implement guardrails

**Option 1: `ValidatingAdmissionPolicy` on `AccessProfile`.** Native, no new
CRD, no operator code. Checks the *profile text* when it is created/updated:

```yaml
# Delegation bound: profiles in a team namespace may only use view/edit and
# must pin their clusters to the team label of their namespace.
validations:
- expression: "object.spec.role.name in ['view', 'edit']"
- expression: >
    has(object.spec.clusters.selector) &&
    object.spec.clusters.selector.matchLabels['team'] ==
    namespaceObject.metadata.labels['protem.dev/team']
# Floor: admin-tier profiles cannot be self-service anywhere.
- expression: >
    !(object.metadata.labels['protem.dev/tier'] == 'admin') ||
    object.spec.approval.rules.all(r, r.mode == 'Manual')
```

The pinned `matchLabels` entry is safe because label selector terms are ANDed:
whatever else the author adds can only narrow the match.

Limits: it sees the profile, not the request. It cannot express "on clusters
that are *actually* prod at request time, add security as approver", because
whether a selector hits prod clusters depends on cluster labels, not profile
text. It also does not re-check existing profiles when a new rule is added.

**Option 2: an `AccessPolicy` CRD, evaluated at request time.** Cluster-scoped,
platform-admin-only, ANDed with the profile's rules per resolved cluster:

```yaml
kind: AccessPolicy
metadata: {name: prod-admin-floor}
spec:
  profileSelector: {matchLabels: {protem.dev/tier: admin}}
  clusterSelector: {matchLabels: {env: prod}}
  require:
    approvers: {groups: [security-oncall]}
    requiredApprovals: 1
    maxDuration: 2h
    forbidAuto: true
```

Exact (evaluated against the resolved clusters), applies to existing profiles
immediately, and the UI can show *why* an approval is needed. Cost: a new CRD,
evaluation code in the security-critical path, and tests for it.

### Decision

Both, with a strict split of responsibilities:

- **Invariant: profiles are the only thing that grants; policies only
  restrict.** An `AccessPolicy` may add approvers, raise `requiredApprovals`,
  forbid `Auto`, or shorten `maxDuration`. It can never allow anything on its own.
  All matching policies are ANDed with the profile's rules. A missing policy,
  or a bug that skips one, therefore at worst leaves access as the profile
  alone allows. It never widens it.
- **Authoring-time bounds** (which roles/clusters a team namespace may put in a
  profile): `ValidatingAdmissionPolicy` samples, shipped with the core. Bad
  profiles are rejected at write time with a clear error.
- **Request-time floors** (minimums that depend on the resolved clusters):
  `AccessPolicy`, shipped **before 1.0**, after the core escalation fixes and
  the v1alpha2 API.
- **Do now, at no cost:** implement request evaluation as "per resolved
  cluster, a list of requirements, all must hold". Profile rules contribute
  requirements today; `AccessPolicy` later appends to the same list without
  reshaping the evaluator.

## 4. Per-CRD evaluation

### 4.1 Cross-cutting

| Issue | Recommendation |
|---|---|
| Everything is cluster-scoped | **Decided: make `AccessProfile`, `AccessRequest`, `AccessGrant` namespaced** (profile's namespace holds its requests and grants). Delegation, read access for approvers, and tenant separation then come from plain namespace RBAC. Scope cannot be changed in place later. One `protem-system` namespace works fine for a central-only team. Requests are also private (reasons, tickets); cluster-scoped means any `list` sees all of them |
| API group `protem.dev` | Reserved; rename (`jit-access-review.md` §3.8) |
| Durations are `string` | `metav1.Duration` everywhere |
| Required/optional is implicit (no `omitempty` = required) | Explicit `+required`/`+optional`. Today `AccessProfile.requireReason` (a bool) and `AccessRequest.reason` are schema-required while the code treats them as optional |
| Specs are mutable | CEL `self == oldSelf` on request and grant `spec` (grant: except the revocation block) |
| `phase` + ad-hoc booleans | `conditions` (`+listType=map`, `+listMapKey=type`) + `observedGeneration`. Keep a `phase` string only as a printcolumn convenience |
| Usernames/emails in label values | `protem.dev/requester: <username>` (`accessrequest_controller.go:197`) fails creation when the username has `@` or exceeds 63 chars. Use a hash label or none; use field indexers in the operator |

### 4.2 `AccessProfile`

| Field | Verdict | Change |
|---|---|---|
| `displayName`, `description` | OK | |
| `roleRef` | OK shape | Rename `role`; CEL: `scope: Cluster` ⇒ `kind: ClusterRole` |
| `targetNamespaces` | **Ambiguous; empty means cluster-wide** | `scope` enum + `namespaces`; CEL ties them |
| `maxDuration`, `defaultDuration` | Strings, unvalidated | `duration: {default, max}`, CEL `default <= max` |
| `approvalPolicy {type, allowedApproverGroups}` | Approver groups never enforced; one policy per profile | `approval.rules[]` (section 2) |
| `allowedRequesterGroups` | Empty = everyone | `requesters.groups` and `requesters.users`, at least one between them; use an explicit `system:authenticated` group if you mean everyone |
| `requireReason`, `ticketValidationRegex` | OK intent | Group under `justification`; validate regex in the controller → `Ready=False` condition |
| `clusterSelector` + `clusterNames` | **Intersection semantics; neither set = every Ready cluster** (`fleet/resolver.go:54-58`) | `clusters: {selector \| names}`, exactly one, required. A profile must never mean "the whole fleet" by omission |
| status `activeGrantsCount` | Fine | Add conditions: `Ready` (role exists on hub, regex compiles), `matchedClusters` count |

### 4.3 `AccessRequest`

| Field | Verdict | Change |
|---|---|---|
| `profileRef` | OK | Local name in the same namespace once namespaced |
| `requester` | **Client-supplied** | Keep, but stamped/validated from `request.userInfo` (VAP; `jit-access-review.md` §3.2) |
| `targetNamespaces`, `targetClusters` | No subset check | Optional subset; resolved into `status.resolved` at creation (section 2, step 1) |
| `duration` | String; schema-required but code defaults it | Optional `metav1.Duration` |
| `reason`, `ticket` | OK | Move under `justification` to mirror the profile |
| status `phase` | Includes `Expired`/`Revoked`, a copy of the grant's lifecycle | `Pending, Approved, Rejected, Cancelled, TimedOut`. The grant owns active/expired/revoked; don't mirror two state machines |
| status `approver`, `approvedAt` | Single approver | `approvals: [{user, decision, at, comment, rule}]`, needed for N-of-M and audit |
| status `rejectionReason` | OK | Fold into a `Decided` condition message |
| status `conditions` | Declared, unused | Use: `Validated`, `Decided` |

### 4.4 `AccessGrant`

| Field | Verdict | Change |
|---|---|---|
| `requestRef`, `profileRef`, `requester`, `roleRef`, namespaces, clusters | Good: immutable snapshot | Keep; enforce immutability |
| `expiresAt` | Good: absolute time | Keep |
| `revoked`, `revokedBy`, `revokedAt`, `revokeReason` | Four loose fields | `revocation: {by, at, reason}`; presence = revoked |
| status `active` | Redundant with `phase` | Delete |
| status `remainingSeconds` | etcd write every 30s | Delete (`jit-access-review.md` §3.6) |
| status `managedBindings` | Single-cluster only | `clusters: [{name, state, bindings[], message, lastTransitionTime}]` aggregated from per-cluster objects |

### 4.5 `ClusterAccessGrant`

Duplicates `AccessGrant` (same snapshot fields, `targetClusters`, revocation)
and no controller reads it. **Delete.** Replace it with the per-cluster
fan-out object from `jit-access-review.md` §7: one namespaced object per
(grant, cluster) in `protem-cluster-<name>`, carrying only what the spoke needs
(subject, role, namespaces, `expiresAt`, revoked). That is the only object a
spoke agent can read, so requests' reasons and tickets never leave the hub.

### 4.6 `TargetCluster`

See `jit-access-review.md` §7: drop `spec.labels` (use `metadata.labels`), add
`enforcement`, `subject` mapping and `paused`, replace `phase` with conditions
driven by a Lease.

## 5. Other multi-cluster considerations

- **Roles must exist on each spoke.** `edit`/`view` do; custom roles may not.
  The agent reports `RoleNotFound` per cluster. Later, optionally, let a profile
  ship role `rules` that the agent materialises as `protem:<profile>`; that is
  more power for the hub, so the spoke ceiling still applies.
- **Namespaces differ per cluster.** The agent reports missing namespaces per
  cluster instead of failing the whole grant.
- **One active grant per (user, profile)**. A second request should extend the
  existing grant (article lesson: recompute from original start, re-check caps), not
  stack a second grant.
- **Notifications are part of approval.** Manual approval without approver
  notification is unusable. Keep it a thin sink (`notify: [<sink>]` →
  webhook/Slack), not a framework.
- **Same IdP, different usernames.** Covered by `TargetCluster.spec.subject`.

## 6. Decisions

1. **Namespaced profiles/requests/grants: decided, yes.** Part of the
   v1alpha2 API change, done together with the API group rename.
2. **Guardrails above profiles: decided.** Restrict-only `AccessPolicy`
   before 1.0 for request-time floors; `ValidatingAdmissionPolicy` samples for
   authoring-time bounds, shipped with the core. See section 3, "Decision".
3. **Approval channel: decided.** Web API now (approvals written to request
   status by the operator's ServiceAccount only; humans never get RBAC on
   `accessrequests/status`). kubectl approval is on the roadmap
   (`jit-access-review.md` §4): an `AccessApproval` object whose approver
   identity is stamped from `request.userInfo` at admission (CRDs cannot add a
   CSR-style `/approval` subresource), counted by the same rule evaluation as
   web approvals.

## 7. Web UI: detail views and optional management

Status: implemented, except `AccessPolicy` editing (that kind does not exist yet). How to
enable and deploy it: `--ui-management=enabled` plus `config/rbac/ui_management_role.yaml`;
login is in `docs/login.md`, the agent in `docs/agent-deployment.md`.

### 7.1 Show what a profile actually grants

Approvers and requesters see only a role *name* today. Add a read-only profile
detail page (`/profiles/{ns}/{name}`), linked from the catalog, the request
form, and the approval view:

- **Permissions:** the role's rules, grouped by API group and resource, with
  verbs. Flag the dangerous ones: `*` verbs or resources, `secrets`,
  `pods/exec`, `impersonate`, `bind`/`escalate`, and RBAC writes.
- **Where:** scope, namespaces, and the clusters it currently resolves to.
- **Who:** requester groups, and per approval rule its clusters, mode,
  approvers, required approvals and maximum duration.
- **Copy as YAML**, so GitOps users can take a profile into their repo.

Where the rules come from matters, because a wrong preview misleads an approver:

1. Phase 1: the hub reads its own copy of the `Role`/`ClusterRole` (it already
   has `get` on both) through a new `GET /api/namespaces/{ns}/profiles/{name}/permissions`.
   Label it "as defined on the hub". Correct for built-in roles (`view`, `edit`,
   `admin`, `cluster-admin`), and may differ on a spoke for custom roles.
2. Phase 2: the spoke agent reports the rules of the role it actually bound, in
   `ClusterGrant.status`. Grant detail then shows the real per-cluster
   permissions, and a missing role appears as `RoleNotFound`.

Also add to request and grant detail: the frozen role and scope
(`implementation-review.md` R2), and per-cluster binding state with the
agent's message.

### 7.2 Managing profiles and clusters from the UI, optionally

Per-object ownership, plus one global switch:

- `--ui-management=readonly|enabled`, default `readonly`. In `readonly` the API
  registers no write routes for configuration at all, so a UI bug cannot write.
- Even when enabled, an object managed by GitOps stays read-only in the UI.
  Detect it from the Argo CD tracking annotation or label, the Flux
  `kustomize.toolkit.fluxcd.io/*` labels, or a `protem.dev/managed-by`
  label. Without this, a UI edit is silently reverted on the next sync.
  Show "Managed by GitOps" with a link to the source where the annotation gives one.

What the UI may write:

| Object | UI write | Why |
|---|---|---|
| `AccessRequest`, approve/reject/cancel, revoke | Yes (today) | Workflow |
| `AccessProfile` | Yes, when enabled | Main ask |
| `TargetCluster` | `paused`, labels, `subject` only | Registration stays a deploy step |
| `AccessPolicy` (later) | Yes, when enabled, platform admins only | Same form machinery |
| `AccessGrant`, `ClusterGrant` | **Never** | A hand-made grant bypasses approval. The break-glass path is an `Auto` rule with notification |

Authorization for writes:

- The API checks the caller with a SubjectAccessReview for the exact verb,
  resource and namespace, then writes with the operator's ServiceAccount.
  Hub RBAC stays the single source of who may edit what.
- Do not use impersonation: `impersonate` on users and groups would let a
  compromised operator act as anyone on the hub.
- The operator's write permission on `accessprofiles` and `targetclusters`
  ships as a separate, opt-in ClusterRole. A read-only install cannot write
  them even if the flag is flipped by mistake.
- Guardrail `ValidatingAdmissionPolicy` rules (section 3) still apply, because
  writes go through the API server. They must key on the object and its
  namespace, not on `request.userInfo`, which is the operator for UI writes.
- Stamp `protem.dev/last-modified-by` with the real user and emit
  an Event, since the API server audit log shows the operator.
- `GET /api/capabilities` returns what the caller may do (`management`, the namespaces where
  they may edit profiles, `manageClusters`, `viewFleet`, `viewAll`), computed from SARs. The UI's
  admin and approver checks use these, not token role claims.

### 7.3 What was built, and what differs from the proposal

1. Profile detail with the permission preview and Copy as YAML.
2. Capabilities on their own route, refreshed every tenth poll and after each write.
3. `--ui-management` with profile create, edit and delete, and the GitOps-managed lock.
4. `TargetCluster` paused, subject and labels. An empty subject means the hub's default.
5. Agent-reported rules: `ClusterGrant.status.rules`, shown per cluster on the grant page.

Differences and limits:

- The agent reports rules for a `ClusterRole` only. A namespaced `Role` can differ per
  namespace, so it gets an existence check and no rules.
- A grant that fails part-way keeps the bindings already made and reports them, with state
  `Failed`. It does not remove them, to avoid creating and deleting access on every sync.
- Profile updates need the `resourceVersion` the editor loaded; a stale one gets 409.
- Capabilities list only namespaces that already hold a profile, unless the caller may create
  profiles everywhere. Creating the first profile in a new namespace works through the API
  check, but the UI offers the namespace only to callers with all-namespace rights.
- State-changing requests must be `application/json`, which is what stops forged cross-site
  form posts when a session cookie is in play.

Adding clusters from the UI came later and replaces the "registration stays a deploy step" row
above. With management enabled, `POST /api/clusters` creates a TargetCluster (`create
targetclusters`); `POST /api/clusters/{name}/join` and `/reissue` (`update` on that cluster)
make sure the agent's ServiceAccount, Role and RoleBinding exist and return a kubeconfig holding
a join token of `--join-token-ttl` plus a kustomization that installs the agent, reissue first
replacing the ServiceAccount so every earlier token stops working; both need `--hub-api-url`.
`DELETE /api/clusters/{name}` (`delete`) is refused while an active grant targets the cluster or
while it is managed by GitOps. `ClusterView.agent` reports `none`, `waiting`, `connected` or
`lost`, and capabilities add `addClusters`. The TargetCluster controller keeps the agent's
identity in step for every cluster but the hub and removes it, through a finalizer, when the
cluster is deleted. Steps: `docs/agent-deployment.md`.
