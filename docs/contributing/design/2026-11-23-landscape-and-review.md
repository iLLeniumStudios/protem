# protem: landscape, lessons, and improvement plan

Status: review of the codebase as of 2026-11-23. Multi-cluster (`TargetCluster`,
`ClusterAccessGrant`, `internal/fleet`) is under active development and is only
covered by design guidance in section 5, not reviewed as finished code.

## 1. Is this worth building?

Yes, with a narrow, explicit position. The open-source space is real but thin:

| Project | Enforcement model | State | Fit |
|---|---|---|---|
| [telekom/k8s-breakglass](https://github.com/telekom/k8s-breakglass) | Authorization webhook (SubjectAccessReview), hub/spoke | CRDs | Most mature; needs kube-apiserver `--authorization-config` on every target cluster ([docs](https://github.com/telekom/k8s-breakglass/blob/main/docs/webhook-setup.md)) |
| [samirtahir91/kube-jit](https://github.com/samirtahir91/kube-jit) | Temporary RoleBindings, per-cluster operator | PostgreSQL + CRD | Closest design; no reusable profiles, small project |
| [diranged/oz](https://github.com/diranged/oz) | Temporary Roles/RoleBindings, exec/pod access | CRDs | Single cluster, no approval flow, no UI |
| Teleport / Azure Protem / Google PAM / Common Fate, Apono, P0 | Proxy or cloud IAM | SaaS / enterprise | JIT is paid, or tied to one cloud |

Where protem can win, and what it must stay:

1. **Works on managed clusters.** It writes plain RBAC bindings, so it needs no
   API-server flags. breakglass's webhook model needs `--authorization-config`
   on the target API server, which managed control planes (EKS, GKE, AKS) do
   not generally expose. Verify per provider before using this in marketing.
2. **Profiles as the product.** `AccessProfile` (role + scope + max duration +
   approval policy + ticket regex + requester groups) is GitOps-friendly and
   has no equivalent in kube-jit.
3. **No database.** All state lives in CRDs. Keep it that way.
4. **kubectl-native.** A request should be creatable with `kubectl create`
   with the UI as a convenience, not the only trusted path (see 3.2).

Do not compete on breadth (session recording, DB/SSH access, many IdPs,
notification matrices). That is Teleport's game.

Before investing heavily, run breakglass against one managed cluster for an
hour. If it fits, contribute there; if not, the failure is this project's
reason to exist.

## 2. What to take from the Edixos article

Source: [Just-in-Time Privileged Access for Multi-Tenant Kubernetes](https://edixos.com/en/blog/just-in-time-privileged-access-kubernetes/)
(Edixos, 2026-07-04). Their `PrivilegedAccessRequest` CRD maps onto our
`AccessRequest` + `AccessGrant`.

| Article lesson | Take it? | Current state here | Action |
|---|---|---|---|
| Namespace-scoped `RoleBinding` by default; any `ClusterRoleBinding` is a bug until proven otherwise | **Yes, highest priority** | `ensureBindings` creates a `ClusterRoleBinding` whenever namespaces are empty or `["*"]` and the role is a `ClusterRole` (`internal/controller/accessgrant_controller.go:158-201`) | Add explicit `scope: Namespaced \| Cluster` to `AccessProfile`, default `Namespaced`; refuse to create a CRB unless the profile says `Cluster` |
| Server-side ownership check: caller may only target scopes they own | **Yes** | The request's `targetNamespaces` *replaces* the profile's (`internal/controller/accessrequest_controller.go:180-183`) with no subset check. Combined with the row above, `targetNamespaces: ["*"]` on any ClusterRole profile yields cluster-wide access | Request namespaces must be a subset of the profile's; `*` only if the profile itself is `Cluster` scope |
| One human per request, never a group | Yes, already done | Subject is the single requester (`accessgrant_controller.go:151-154`) | Keep; add a test that locks it |
| Hard duration cap enforced at admission, not only in the controller | **Yes** | Bad `duration` silently becomes 30m; bad `maxDuration` silently disables the cap (`accessrequest_controller.go:161-171`) | Reject unparsable durations; add a global operator-wide ceiling flag (e.g. `--max-grant-duration=48h`) applied on top of the profile |
| Duration wire-format bug (seconds read as nanoseconds) | Yes, as a rule | Durations are free-form `string` fields with no validation | Use `metav1.Duration` (string on the wire, typed in Go) for `duration`, `maxDuration`, `defaultDuration`; add CRD validation |
| Reconcile order: provision before expiry check | Already fine | Revoked → expired → ensure, requeue `min(time-to-expiry, 30s)` (`accessgrant_controller.go:80-145`) | Keep |
| Requeue at `min(expiry, safety interval)` | Already done | Same as above | Keep, but stop writing status every 30s (see 3.6) |
| Extension recomputes expiry from original start, re-checks cap | **Yes, small** | No extension path | Add `spec.extendBy` or extend via a new approved request; expiry = `approvedAt + total`, capped |
| Expiry print column as `string`, not `date` (kubectl rendering bug) | Yes, trivial | `ClusterAccessGrant` already uses `string`; `AccessGrant` has no expiry column | Add `Expires At` (`type=string`) to `AccessGrant` |
| Audit in logs/events; GC terminal objects after a retention window | **Yes** | Audit is rebuilt from current, mutable `AccessRequest` state (`internal/api/server.go:430-478`); nothing is ever GC'd | Emit Kubernetes Events and one structured log line per transition; GC terminal requests after a configurable retention (default e.g. 30d) |
| CMDB as source of truth, criticality gates | **No** (org-specific) | n/a | Generalise to *namespace ownership via labels/annotations* (kube-jit does this): `protem.<domain>/owner-group` on a namespace selects approvers |
| Two-track authorization (transverse vs per-asset roles) | Partially | n/a | Reachable later via profile scope + namespace-owner approval; no hardcoded allowlists |
| Freeze identity fields at creation | Yes | `spec` of `AccessRequest`/`AccessGrant` is mutable | Make `spec` immutable after create with CEL `x-kubernetes-validations: self == oldSelf` (except revoke fields) |

## 3. Issues found in the current code

Ordered by severity. The first four are privilege-escalation paths and must be
fixed before anyone runs this against a real cluster.

### 3.1 Authentication is bypassable (critical)

`internal/api/auth.go`:

- Lines 42-58: any caller can become any user, including `admin@company.com`,
  via the `X-Test-User` header or `?as=` query parameter. This runs
  unconditionally in production.
- Lines 60-96: the JWT payload is base64-decoded but the **signature, issuer,
  audience and expiry are never verified**. A hand-crafted token works.
- Lines 98-99: unauthenticated requests fall back to user `alice`.
- `internal/api/server.go:81-92`: CORS `Access-Control-Allow-Origin: *` on
  state-changing endpoints.

Fix: verify tokens with `github.com/coreos/go-oidc/v3` (issuer, audience, expiry,
JWKS), or sit behind oauth2-proxy and trust only its headers from a known
source. Gate dev personas behind an explicit `--dev-auth` flag that refuses to
start when not on loopback. Unauthenticated → 401. Drop wildcard CORS; the UI
is served from the same origin (`server.go:78`).

### 3.2 Requester identity is client-supplied (critical)

`AccessRequest.spec.requester` (username, email, **groups**) is written by the
client. Anyone with `create` on `accessrequests` via kubectl can claim any
groups and pass `allowedRequesterGroups` (`accessrequest_controller.go:81-104`).

Fix, cheapest first: a `ValidatingAdmissionPolicy` (native, no webhook server)
that requires `object.spec.requester.username == request.userInfo.username`
and `object.spec.requester.groups` ⊆ `request.userInfo.groups`, exempting the
operator's own ServiceAccount (which creates requests on behalf of
authenticated UI users). Kubernetes' `CertificateSigningRequest` is the
precedent: the API server stamps the requester; the client cannot.

### 3.3 Approval has no authorization (critical)

`internal/api/server.go:256-266` (`approve`):

- No check that the approver is in `approvalPolicy.allowedApproverGroups`. That
  field is only displayed (`server.go:144`), never enforced.
- No self-approval prevention.
- No check that the request is `Pending`. Approving a request the controller
  already **rejected** (not in requester groups, bad ticket, missing reason)
  flips it to `Approved`, and the controller then creates the grant
  (`accessrequest_controller.go:150`). All controller-side validation is
  therefore bypassable through the approve endpoint.

Fix: approve/reject only from `Pending`; approver ∈ allowed approver groups;
approver ≠ requester; use optimistic concurrency (already implicit via
`resourceVersion` on `Update`, keep it). Humans must never get RBAC on
`accessrequests/status`; that subresource *is* the approval.

### 3.4 Scope escalation via namespaces / `*` (critical)

See section 2 rows 1-2. Both bugs in the article exist here, and they compose.

### 3.5 Binding drift and adoption (medium)

`ensureBindings` only creates when missing (`accessgrant_controller.go:184-194`,
`232-242`). If a binding named `protem-grant-<grant>` already exists, or someone
adds subjects to a managed binding, it is adopted and never corrected.
Fix: `controllerutil.CreateOrUpdate` to the desired subjects/roleRef, and
refuse to adopt a binding lacking the `protem.dev/grant` label. `Owns()` the
bindings (owner refs work for cluster-scoped grant → namespaced binding) so
deletion is watched and re-created while active.

### 3.6 Status churn (low)

The grant reconciler writes `status.remainingSeconds` every 30s per active grant
(`accessgrant_controller.go:119-143`): an etcd write per grant per 30s for a
value the API already computes live (`server.go:308`). Delete
`remainingSeconds`; requeue only at expiry, plus periodic resync for drift.

### 3.7 Subject name must match the API server's username claim (medium)

The binding subject is the requester's email (`accessgrant_controller.go:151-154`).
That only works if kube-apiserver maps the OIDC username claim to email with no
prefix. On EKS (IAM ARN / access entries), GKE (Google email) and AKS (Entra
object ID or UPN) the username differs. Make the subject template configurable
(`--subject-template='oidc:{{.Email}}'` or a profile field) and document per
provider. Get this wrong and grants are silently useless: the same failure
class as the article's "born expired" bug.

### 3.8 API group name (medium, cheap now, expensive later)

The API group is `protem.dev` (`api/v1alpha1/groupversion_info.go:29`).
`*.k8s.io` groups are reserved for Kubernetes project APIs (KEP-1111); CRDs
there need an `api-approved.kubernetes.io` annotation. Rename to a domain you
own (e.g. `protem.dev`) **before** anyone depends on the CRDs.
Coordinate with the multi-cluster work since it adds types in the same group.
The samples already disagree with the code (`apiVersion: protem.dev/v1alpha1`
in `config/samples/`), a leftover of the kubebuilder `domain`/`group` split in
`PROJECT`; fix both in the rename.

### 3.9 Smaller items

- `config/samples/protem_v1alpha1_accessprofile.yaml` is the empty kubebuilder
  stub; write a real sample per approval type.
- No CRD validation beyond enums: add `MinLength`, duration patterns, and
  `x-kubernetes-validations` for `defaultDuration <= maxDuration`.
- Ticket regex is compiled on every reconcile and an invalid regex rejects
  every request; validate it on the `AccessProfile` instead (CEL can't, so
  surface it as a profile status condition from `AccessProfileReconciler`).
- `README.md` is the kubebuilder template.

### 3.10 The operator cannot create the bindings it promises (blocker)

Kubernetes RBAC escalation prevention: creating a (Cluster)RoleBinding to a
role requires either holding every permission in that role, or the `bind`
verb on it. `manager-role` (`config/rbac/role.yaml`) has CRUD on bindings but
no `bind`, and only read on roles. On a real cluster, binding a requester to
`view`, `edit` or `cluster-admin` is therefore rejected with "attempting to
grant RBAC permissions not currently held". The envtest suite does not catch
this because envtest's client is an admin.

Fix: add `bind` on `roles`/`clusterroles`, ideally restricted with
`resourceNames` to the roles profiles reference (a Helm/kustomize value, since
kubebuilder markers cannot be dynamic). Either way the operator becomes a
tier-0 component: whoever controls it or can write `AccessGrant` directly can
grant those roles. Restrict `create/update` on `accessgrants` to the operator's
ServiceAccount. Section 6 shows the authorizer mode that removes this power.

## 4. Roadmap

Each step is shippable on its own; do them in order.

1. **Close the escalation paths**: 3.1, 3.2, 3.3, 3.4. Add one test per path
   that fails on the current code (e.g. "approve after controller rejection
   creates no grant", "`targetNamespaces: [\"*\"]` on a namespaced profile
   creates no ClusterRoleBinding", "`X-Test-User` without `--dev-auth` → 401").
2. **Make the API boring and correct**: `metav1.Duration`, immutable specs,
   profile `scope`, global max duration, rename the API group (3.8).
3. **Correct reconciliation**: `bind` permission (3.10), `CreateOrUpdate` +
   `Owns()` bindings, drop `remainingSeconds`, configurable subject template.
4. **Audit**: Events + structured logs per transition, retention GC. Point to
   kube-apiserver audit logs as the authoritative record of binding writes.
5. **Usability**: extensions, namespace-owner approval via labels,
   Slack/email notification as a thin webhook sink (not a framework).
6. **kubectl approval**: `AccessApproval` object with approver identity stamped
   at admission, counted alongside web approvals (`crd-design.md` §6.3).
7. **Guardrails before 1.0**: `ValidatingAdmissionPolicy` samples for
   delegated profile authoring (with step 2), then a restrict-only
   `AccessPolicy` for request-time floors (`crd-design.md` §3).

## 5. Guidance for the multi-cluster work (in progress)

Not a review of unfinished code; constraints the design should satisfy:

- **Pull, not push.** An agent in each spoke watches the hub for its own
  `ClusterAccessGrant`s and applies bindings locally. The hub then needs no
  admin kubeconfigs for spokes, which removes the biggest blast radius in
  hub/spoke designs (breakglass and kube-jit both hold spoke credentials).
- **Spoke-side expiry.** The agent enforces `expiresAt` itself. A grant must
  expire on time even if the hub is unreachable; a hub outage must fail
  closed.
- **Same invariants per spoke.** Scope rules from 3.4 apply per cluster; a
  cluster selector must never widen namespace scope.
- **Clock skew.** Spoke compares against `expiresAt` in absolute time; keep a
  small negative margin rather than trusting hub-side countdowns.
- `internal/fleet/resolver.go` treats an empty `status.phase` as Ready (line 27).
  Prefer failing closed: unknown phase ⇒ not eligible.

## 6. Hybrid enforcement: authorizer webhook where possible, RoleBindings elsewhere

Feasible, and the seam already exists: the `AccessGrant` (or per-cluster
`ClusterAccessGrant`) is the single source of truth, and *enforcement* is just
how a target cluster turns active grants into allowed API calls. Requests,
profiles and approvals stay identical. The mode is a property of the cluster
(`TargetCluster.spec.enforcement: RoleBinding | Authorizer`, default
`RoleBinding`), because only the cluster owner knows whether kube-apiserver
config is reachable. Profiles never choose it.

### How authorizer mode works (breakglass-proven pattern)

1. For each profile, a **standing** binding grants the profile's role to a
   synthetic group, e.g. `protem:profile:<name>`, in the profile's namespaces (or
   cluster-wide for `scope: Cluster`). No human is ever in that group.
2. kube-apiserver runs `Node,RBAC,Webhook` via `--authorization-config`; the
   webhook sits **after** RBAC, so it is only asked when RBAC already said no.
3. On a SubjectAccessReview for user U: find U's active grants on this cluster;
   if the request's namespace is inside a grant's scope, re-ask the API server
   with a SubjectAccessReview for the same attributes and groups
   `[protem:profile:<name>]`. Return `allowed` if that passes, otherwise
   **no opinion**. Never return `deny`.
4. Rule evaluation is delegated to the API server's own RBAC through step 3.
   Do not reimplement RBAC rule matching in-process.

Mandatory cluster-side config for this mode, otherwise it is a backdoor:
- `AuthenticationConfiguration.userValidationRules` rejecting tokens whose
  groups start with `protem:`. Without it, an IdP group named
  `protem:profile:admin` gets standing access through step 1.
- Authorizer cache for allowed decisions disabled (`cacheAuthorizedRequests:
  false`), or expiry is late by the TTL.
- `matchConditions` skipping `system:` users, ServiceAccounts and the
  operator's own identity (avoids recursion on its step-3 SARs).

### What each mode gives

| Property | RoleBinding mode | Authorizer mode |
|---|---|---|
| Works on EKS/GKE/AKS | Yes | No (needs kube-apiserver flags) |
| Expiry if the operator/agent is down | **Fails open**: bindings stay until reconciled | **Fails closed**: decision computed per request; webhook down = no elevated access |
| Revocation latency | Reconcile loop (seconds) | Next request |
| Operator needs `bind`/escalation power | Yes, at runtime (3.10) | Only for the static synthetic-group bindings, which can be applied by GitOps instead, leaving the operator with no RBAC write |
| Tamper/drift surface | Per-user binding objects | None per user |
| Finer-than-RBAC rules (e.g. resourceName prefixes, Oz's use case) | No | Possible later (webhook sees the request attributes) |
| Runtime cost | One write per grant | In the API server's request path for every RBAC-denied human call; must be HA and low-latency |
| Long-lived streams (`exec`, `port-forward`, `watch`) after expiry | Survive (authorized at start) | Survive too: neither mode cuts open connections |

### Recommendation

- Build RoleBinding mode first and make it correct (sections 3-4); it is the
  only mode that works everywhere, and it is most users' clusters.
- Keep the seam clean *now* at no cost: enforcement code reads only grants
  (plus the profile role), and nothing in request/approval code knows how
  access is enforced. In the multi-cluster design, the spoke agent is the natural
  home for either enforcer.
- Add authorizer mode as the second adapter only once there is a real
  self-managed cluster (kubeadm, k3s, RKE2, Talos, Gardener) to test against.
  It doubles the security-critical surface and test matrix, so it needs its
  own e2e suite (kind with `--authorization-config`), not envtest.
- RoleBinding mode's fail-open expiry is its biggest weakness. Mitigate it with HA
  agents (leader election) plus a metric/alert on
  `now - expiresAt` for grants whose bindings still exist.

## 7. Is `TargetCluster` the right kind?

The concept is right: the hub needs a per-cluster object to select against
and to show health. Its job should be narrower than an inventory, though:
**"how protem enforces on cluster X"**. Inventory (which clusters exist)
has a standard; Protem's per-cluster settings do not.

### The standard to align with, not adopt yet

SIG-Multicluster's `ClusterProfile` (`multicluster.x-k8s.io/v1alpha2`,
[KEP-4322](https://github.com/kubernetes/enhancements/blob/master/keps/sig-multicluster/4322-cluster-inventory/README.md),
[overview](https://multicluster.sigs.k8s.io/concepts/cluster-profile-api/)) is
published by OCM, GKE Fleet and KubeFleet, and consumed by Argo CD, Kueue,
Headlamp, Sveltos and multicluster-runtime
([implementations](https://multicluster.sigs.k8s.io/implementations/cluster-inventory-api-implementations/)).
Do not replace `TargetCluster` with it:
- It is still alpha, and most Protem users will not run a cluster manager.
- It is owned by the cluster manager, so Protem settings cannot live on it.
- Its `accessProviders` model is the hub reaching *into* spokes, which the
  pull-agent design deliberately avoids.

Do keep `TargetCluster` shaped like it (`displayName`, labels for selection,
`conditions`), so adding an optional `spec.clusterProfileRef` (import
labels/registration from an existing inventory) later is additive.

### Changes to the type

| Field | Change | Why |
|---|---|---|
| `spec.labels` | **Delete**; select on `metadata.labels` only | Two label sources merged in `fleet/resolver.go:42-48`; `metadata.labels` is what `kubectl -l`, `LabelSelector` and every tool already use |
| `spec.enforcement` | **Add** `RoleBinding \| Authorizer` | Section 6; a property of the target API server |
| `spec.subject` | **Add** username template/prefix | Section 3.7: identity mapping differs per cluster (EKS vs GKE vs AKS), so it belongs here, not global |
| `spec.paused` | **Add**; replaces `Maintenance` phase | Maintenance is intent (spec); nobody sets that phase today |
| `status.phase` | **Replace** with `conditions` (`Ready`, `AgentConnected`) | API conventions prefer conditions; today only the heartbeat handler writes `Ready` (`internal/api/server.go:333`) and **nothing ever writes `Disconnected`**, so a dead agent's cluster stays `Ready` forever |
| `lastHeartbeat`, `agentVersion` | Move to a `coordination.k8s.io` **Lease** | See below |

### Heartbeat and agent channel: use the hub's Kubernetes API, not the REST API

Observed in the in-progress code (`internal/enforcer/agent.go`, `server.go`),
in order of severity:

1. **Unauthenticated control channel.** The agent polls `GET /api/grants?cluster=`
   and posts `POST /api/fleet/heartbeat` with no credentials. Anyone who can
   reach the hub can mark any cluster `Ready` and read every grant. If the
   agent's hub URL is plain HTTP, a network attacker can inject grants and the
   agent will create RoleBindings from them.
2. **Grants with no `targetClusters` go to every spoke.** The filter
   (`server.go:362`) only applies when the list is non-empty, so a hub-local
   grant is applied on every cluster. Empty must mean "hub only".
3. **Fails open when the hub is unreachable.** `SyncOnce` returns before the
   local sweep (`agent.go:83-86`), so bindings outlive `expiresAt` for as long
   as the hub is down.
4. Empty namespaces default to `default` (`agent.go:108-110`); 3s polling.

Proposed shape (the Open Cluster Management klusterlet/ManifestWork pattern, built
from native pieces):

- **One namespace per cluster on the hub** (`protem-cluster-<name>`), created by
  the `TargetCluster` controller. It holds:
  - a **Lease**, renewed by the agent. The hub derives the `Ready` condition
    from `renewTime`, exactly like node heartbeats in `kube-node-lease`;
  - **one namespaced grant object per (grant, cluster)**, fanned out by the
    hub from the approved `AccessGrant`. The agent reports apply state on it;
    the hub aggregates into `AccessGrant.status.clusters[]`. This replaces
    `ClusterAccessGrant`, which today duplicates `AccessGrant.spec.targetClusters`
    and is not used by any controller.
- **The agent authenticates to the hub API server** with a ServiceAccount whose
  RBAC covers only its own namespace: watch grants, update their status,
  renew its Lease. A compromised spoke can't see or touch other clusters'
  grants. TLS and authn come from the API server, and the custom REST
  endpoints for the agent go away.
- **Watch, don't poll**: run a controller-runtime manager against the hub
  kubeconfig.
- **Expiry enforced locally**: stamp `protem.dev/expires-at` on every binding
  the agent creates, and sweep on a local timer that runs whether or not the
  hub is reachable.

With this, `TargetCluster` stays small: identity + enforcement settings +
conditions. The Lease covers liveness, and the per-cluster namespace is the security boundary.
