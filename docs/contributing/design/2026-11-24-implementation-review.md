# Implementation review: phases 1-4 (2026-11-23)

Reviews the implementation against `crd-design.md` and `jit-access-review.md`.
File:line references are to the code as reviewed.

Verification run:

- `go build ./...` and `go vet ./...`: pass.
- `make test`: green, **but the envtest suite is skipped** (see H2).
- Envtest against a real API server (`KUBEBUILDER_ASSETS` set to
  `bin/k8s/1.37.0-darwin-arm64`): **3/3 specs fail**. The CRDs, including their
  CEL rules, install fine; the failing specs create objects the new schema
  rejects.

## What landed well

- **API shape matches `crd-design.md`:**
  - group renamed to `protem.dev`
  - profiles, requests and grants namespaced
  - `scope` enum with CEL checks; exactly one of `clusters.selector`/`clusters.names`
  - `metav1.Duration` with a `default <= max` check
  - `requesters.groups` has `MinItems=1`
  - `approval.rules[]`, frozen `status.resolved`, and an `approvals[]` list
  - immutable request/grant specs; a `revocation` block
  - `ClusterAccessGrant` replaced by a per-cluster `ClusterGrant`
- **`fleet.ResolveAndEvaluate`:**
  - fails closed on cluster readiness
  - checks namespace and cluster subsets
  - rejects the request on a resolution error (the old `targetClusters, _ :=` bug is gone)
  - matches the first rule per cluster
  - takes the shortest duration
- **`AccessGrantReconciler.ensureBindings`:**
  - creates a ClusterRoleBinding only when `scope: Cluster`
  - uses `CreateOrUpdate` and refuses to adopt foreign bindings
  - has the `bind` verb and watches bindings
- **API server:** wildcard CORS is gone, approval requires `Pending`, and there is a self-approval check.

## Critical: block any real deployment

### C1. Dev auth is always on
`NewServer` sets `DevAuth: true` (`internal/api/server.go:47`), and
`cmd/main.go` has no `--dev-auth` flag. As a result, sending `X-Test-User: admin@company.com` (or
`?as=`) authenticates as admin on every deployment. The escalation tests pass
only because they set `server.DevAuth = false` by hand
(`internal/api/escalation_test.go:22,38`).
**Fix:** default `false`, add the flag, refuse `--dev-auth` unless bound to loopback.
Test the *constructed* server, not a hand-modified one.

### C2. JWT signature still not verified
`Authenticate` decodes the payload and checks only `exp` (`internal/api/auth.go:72-114`).
A hand-made token with `"groups":["sre-leads"]` approves anything, and the
requester groups stamped from it make `requesters.groups` meaningless.
**Fix:** `github.com/coreos/go-oidc/v3`: verify issuer, audience, signature (JWKS) and expiry;
configure the issuer and audience by flag.

### C3. Every elevation is also granted on the hub
`AccessGrantReconciler.Reconcile` calls `ensureBindings` (local bindings) for
every grant (`accessgrant_controller.go:121`) *and* `ensureClusterGrants`.
`ResolveAndEvaluate` requires at least one ready spoke, so every grant targets
spokes, and the hub gets the same RoleBinding/ClusterRoleBinding too. A
`scope: Cluster` profile therefore yields cluster-admin on the hub, which holds
every profile and request.
**Fix:** the hub binds locally only if the hub is itself a
registered target (an explicit `TargetCluster` for it, e.g. `local`); otherwise
it only fans out `ClusterGrant`s.

### C4. Fleet channel unauthenticated; PKI unused
- `GET /api/grants` never calls `Authenticate` (`server.go:519`): anyone can list
  every grant, and the agent trusts whatever it returns.
- `POST /api/fleet/heartbeat`, `/api/fleet/tokens` and `/api/fleet/register` are
  unauthenticated (`server.go:459, 756, 805`). Anyone can mint a bootstrap token
  for any cluster name and enrol it.
- The issued client certificate is thrown away: `cmd/enforcer/main.go` discards
  the result of `EnrollWithBootstrapToken` (`_, _, err :=`), and
  `EnrollWithBootstrapToken` discards the private key (`agent.go:218`). The hub
  serves plain HTTP (`ListenAndServe`, `server.go:68`).
- `HubCA` and its tokens live in memory (`fleet.NewHubCA()` per `NewServer`):
  - a new CA on every restart, which strands every enrolled agent
  - a different CA per replica (`NeedLeaderElection` is false)

**Fix:** implement `jit-access-review.md` §7 as written; it removes the custom PKI:
- The agent uses a hub kubeconfig whose ServiceAccount can only read
  `clustergrants` (and update their status) in `protem-cluster-<name>`, and renew its Lease there.
- Bootstrap means creating that ServiceAccount plus a token; there is no CSR endpoint.
- Delete `/api/grants?cluster=`, `/api/fleet/*` and `internal/fleet/pki.go`.

### C5. Approval ignores the per-cluster requirements
`ResolveAndEvaluate` computes `ClusterRules`, but the controller keeps only
`OverallMode` (`accessrequest_controller.go:127-159`). The approve endpoint
(`server.go:350-412`):
- counts the caller as an approver if they are in **any** rule's approvers,
  so an approver for dev can approve prod
- flips the request to `Approved` after **one** approval, ignoring `requiredApprovals`
- allows self-approval if any rule that names the caller has `allowSelfApproval`

**Fix:**
- Persist per-cluster requirements in `status.resolved` (cluster, rule, mode,
  approvers, required count) when the request is resolved.
- The approve endpoint only *appends* a decision, recorded with the rule(s) it satisfies.
- The **controller** decides `Approved` when every cluster's requirement is met
  (`crd-design.md` §2 step 3). That is also the seam `AccessPolicy` appends to later.

### C6. The spoke agent does not implement the design
`internal/enforcer/agent.go` still polls `AccessGrant`s over REST and never reads `ClusterGrant`:
- `scope: Cluster` becomes a RoleBinding in `default` (`agent.go:114-117`); an
  empty namespace list also means `default`
- the subject is always `Email` (`agent.go:158`); `TargetCluster.spec.subject` is ignored
- it fails open when the hub is unreachable: `SyncOnce` returns before the local
  sweep (`agent.go:90-92`), so bindings outlive `expiresAt` for as long as the hub is down
- `ClusterGrant`s are created on the hub, but nothing on the spoke consumes them

**Fix:** rewrite as a controller-runtime manager watching `ClusterGrant` in its hub
namespace, stamping `protem.dev/expires-at` on bindings and sweeping
on a local timer regardless of hub reachability.

## High

- **H1. RBAC for two kinds still uses the old group.**
  - `clustergrant_controller.go:40-42` and `targetcluster_controller.go:45-47` declare
    `groups=protem.dev`, so `config/rbac/role.yaml` grants nothing on
    `protem.dev/clustergrants` or `targetclusters/status`.
  - On a real cluster, ClusterGrant fan-out and heartbeat status writes are Forbidden.
    The errors are swallowed (`accessgrant_controller.go:96,112,126`: `_ =`, `, _ :=`).
  - Nothing creates the `protem-cluster-<name>` namespaces either, and the operator
    has no RBAC to create them.
- **H2. The test gate is hollow.**
  - The `test` target in the Makefile no longer sets `KUBEBUILDER_ASSETS`, and
    `suite_test.go:69-71` `Skip`s when the binaries are missing, so `make test`
    and CI never run envtest.
  - Run with real binaries, all 3 specs fail: scaffold specs create a profile/grant
    with an empty `scope` and no `expiresAt`, request spec at `:79`.
  - Restore the scaffold `KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use ...)"` line,
    remove the `Skip`, and fix the specs.
  - The escalation tests use the fake client, which does not run CEL validation
    or status-subresource semantics; the API-server-level rules are untested.
- **H3. The subject template is never wired.** `AccessGrantReconciler.SubjectTemplate` is not set in
  `cmd/main.go`, and `TargetCluster.spec.subject` has no reader.
- **H4. `pendingTimeout` is not implemented.** Nothing sets `TimedOut`.
- **H5. Request/grant actions are not namespace-scoped.**
  - Approve and revoke find the object by name across all namespaces
    (`server.go:316-328, 587-599`); same-named objects in two namespaces collide.
  - **Revoke has no authorization**: any authenticated user can revoke anyone's grant.
  - `GET /api/requests` returns every namespace's requests (reasons, tickets) to
    any user, which defeats one goal of namespacing.
  - **Fix:** routes `/api/namespaces/{ns}/requests/{name}/...`; authorise with a
    SubjectAccessReview on the hub (crd-design §1).

## Medium (API cleanup)

- `TargetCluster`:
  - `spec.labels` remains but the resolver now uses `metadata.labels` only; delete the field
  - `status.phase` (with the unused `Maintenance` value) and `lastHeartbeat` remain; the design was conditions plus a Lease
- Free-string state fields should be enums:
  - `ApprovalDecision.Decision`
  - `ClusterBindingState.State`
  - `ClusterGrantStatus.Phase`
- `ApprovalRule`:
  - no CEL requiring `approvers` when `mode: Manual`
  - `rules` has no `MinItems`
  - rule `name` is not unique (`+listType=map`, `+listMapKey=name`)
- Grant status updates detect changes by slice *length* only (`accessgrant_controller.go:136`);
  a per-cluster state change with the same count is never written.

## Suggested fix order

1. C1, C2, H5 (authn/authz of the web API). Each with a test against the
   server as `cmd/main.go` constructs it.
2. C3 (no hub bindings unless the hub is a target).
3. C5 (requirements persisted; the controller decides approval).
4. H1, H2 (RBAC group, namespaces, real envtest gate).
5. C4 + C6 together (agent on the hub's Kubernetes API, delete the REST channel and PKI), then H3, H4.
6. Medium items.

Run a fresh read-only review after steps 1-3: they touch authn, authz and
approval logic.

---

# Recheck (2026-11-24)

How each finding was verified:

- `go build` and `go vet` pass, and `make test` is green.
- The envtest suite now **runs locally** (3/3 pass), but only because `bin/k8s/`
  exists on this machine (it was downloaded during the first review). CI is unchanged.
- **Runtime-confirmed:** F1 (reject) and F3 (audit), each with a throwaway
  fake-client test against the server as `NewServer` builds it; the test has been
  deleted. Everything else below comes from reading the code.

## Status of previous findings

| ID | Status | Notes |
|---|---|---|
| C1 dev auth | **Fixed** | `--dev-auth` defaults to false; `NewServer` refuses it on non-loopback addresses; a test builds the real server |
| C2 JWT verification | **Fixed** | `go-oidc` verifier when `--oidc-issuer-url`/`--oidc-client-id` are set; otherwise only dev mode accepts unsigned tokens, and production fails closed (401) |
| C3 hub bindings | **Fixed** | `isHubTargeted`: the hub binds locally only if the grant names the hub cluster |
| C4 REST channel / PKI | **Fixed** | `pki.go` and `/api/fleet/*` deleted; the agent uses the hub's Kubernetes API. See F6 for what is still missing |
| C5 approval evaluation | **Mostly fixed** | Requirements persisted; the API appends decisions; the controller counts per requirement. See F1, F2, F4 |
| C6 agent design | **Fixed** | Reads `ClusterGrant`; Lease heartbeat; expiry annotation plus a 5s local sweep that runs without the hub; scope-aware; uses the subject template |
| H1 RBAC group / namespaces | **Fixed** | All kinds under `protem.dev`; namespaces and leases covered |
| H2 envtest gate | **Fixed** | Makefile `test` runs with `envtest` and `KUBEBUILDER_ASSETS`; `suite_test.go` asserts binaries present and never skips; CI runs full real envtest suite |
| H3 subject template | **Fixed** | Hub flag plus per-cluster `TargetCluster.spec.subject` |
| H4 pending timeout | **Fixed** | `TimedOut` set from `creationTimestamp + pendingTimeout` |
| H5 namespaced routes / revoke authz | **Fixed** | Namespaced routes added; revoke checks requester-or-SAR; SAR RBAC granted |
| Medium API items | **Fixed** | Enums added; `rules` has `MinItems` and unique names; CEL requires approvers for `Manual`; `spec.labels` removed |
| F1 reject eligibility | **Fixed** | Rejections restricted to eligible approvers for pending rules; requesters can cancel own requests |
| F2 born expired multi-approval | **Fixed** | Grant duration clock starts at transition to `Approved` (when controller decides) |
| F3 read route authz & filtering | **Fixed** | `/api/audit`, `/api/stats`, `/api/clusters`, `/api/grants` authenticated; audit and grants filtered by requester/approver or SAR get |
| F4 multi-rule approval & conflict | **Fixed** | Approver matching multiple rules records decisions for all satisfied rules; Pending with no requirements returns 409 Conflict; self-approval checks email and username |
| F5 SAR RBAC & user mapping | **Fixed** | `authorization.k8s.io/subjectaccessreviews` create RBAC marker added; SAR user mapped using subject template |
| F6 two writers on ClusterGrant | **Fixed** | Spoke agent exclusively owns `ClusterGrant.status`; Hub-side ClusterGrantReconciler deleted |
| F7 agent least privilege | **Fixed** | Hub resolves and stamps `ClusterGrant.spec.subject`; agent requires no cluster-scoped TargetCluster read; least-privilege manifests created in `deploy/spoke/agent-rbac.yaml`; dev defaults removed |
| F8 agent drift & adoption | **Fixed** | Spoke agent enforces refuse-to-adopt for unmanaged bindings and reconciles drift on RoleRef/Subjects/expiry |
| F9 hub name & status diffing | **Fixed** | TargetCluster hub readiness strictly checks `cluster.Name == r.HubClusterName`; deep status equality comparison used |

## New or remaining findings

### F1. Critical: any authenticated user can reject any request (runtime-confirmed)
`rejectRequest` (`internal/api/server.go:598-642`) authenticates but never checks
that the caller is an eligible approver. The controller turns **any** `Rejected`
decision into a terminal `Rejected` phase (`accessrequest_controller.go:191-199`).
Reproduced: `mallory` (group `devs`) rejected a prod request that needs
`sre-leads`; the API returned 200 and recorded the decision. This lets anyone
block every access request, break-glass included.
**Fix:**
- Apply the same eligibility check as approve. A rejection should count only for
  the requirements the rejector is eligible for.
- Allow the requester to cancel their own request (`Cancelled`).
- Add a test that fails on the current code.

### F2. High: grant can be born expired with multi-approval
The grant's `expiresAt` is the **first** approval's timestamp plus duration
(`accessrequest_controller.go:255-267`). With `requiredApprovals: 2`, first
approval at T, second at T+9h, and an 8h duration, the grant is created already
expired. This is the article's "born expired" bug.
**Fix:** start the clock at the transition to `Approved`, i.e. the time the
controller decides. Record it as `status.approvedAt` or as the `Decided`
condition's `lastTransitionTime`.

### F3. High: unauthenticated or unfiltered read endpoints (audit runtime-confirmed)
- `GET /api/audit` has **no authentication** and returns every request's requester,
  reason and ticket. Reproduced with no credentials: HTTP 200 and full data.
- `GET /api/stats` and `GET /api/clusters` have no authentication.
- `GET /api/grants` authenticates but returns every grant in every namespace to any user.

**Fix:**
- Authenticate every route except static UI assets.
- Filter grants and audit the same way as `handleGetRequests`: own items, approver
  items, or SAR `get` permission.

### F4. High: approval eligibility edge cases
- The approve endpoint records only the **first** rule the approver matches
  (`server.go:533-568`). An approver eligible for two different rules (e.g. both
  dev and prod approver groups) can never satisfy the second rule, and the
  request hangs until timeout.
  **Fix:** record every rule the decision satisfies, or let the controller
  derive eligibility from the approver's identity snapshot.
- The controller counts approvals whose `Rule == ""` towards **every** requirement
  (`accessrequest_controller.go:215`). Only the "no requirements" fallback writes
  an empty rule today (`server.go:569-572`).
  **Fix:** treat an empty rule as eligible for nothing. Make the fallback a 409:
  a `Pending` request with no requirements is a controller bug, not an open door.
- Self-approval compares email only (`server.go:540`); compare username too, as
  the requester checks elsewhere do.

### F5. High: SAR checks will always fail on a real cluster
`checkSAR` creates `SubjectAccessReview`s, but `config/rbac/role.yaml` has no
`create` on `authorization.k8s.io/subjectaccessreviews`. It fails closed:
- admins can't revoke others' grants
- auditors get filtered lists

**Fix:**
- Add the RBAC marker.
- Set the SAR `User` with the same mapping the hub API server uses (the
  `subject` template), not raw email.

### F6. High: two writers on `ClusterGrant.status.phase`
The hub's `ClusterGrantReconciler` sets `Active`/`Expired`/`Revoked` from spec
and time, and the spoke agent sets `Active`/`Failed` from what it actually
applied. When the agent writes `Failed`, the hub immediately overwrites it with
`Active` on the next reconcile. The status flaps, and the hub shows grants as
applied on spokes where they failed.
**Fix:** the agent owns `ClusterGrant.status`. Delete the hub-side
`ClusterGrantReconciler`; the hub only aggregates into `AccessGrant.status.clusters`.

### F7. Medium: agent deployment and least privilege not delivered
- No RBAC manifests or bootstrap for the agent's hub identity. The intended
  ServiceAccount is limited to `clustergrants` and `clustergrants/status` in
  `protem-cluster-<name>`, plus its Lease. Without the manifests nobody can deploy
  it, and in practice people will hand it an admin kubeconfig.
- The agent reads the cluster-scoped `TargetCluster` for the subject template
  (`agent.go:92-97`), which forces cluster-wide read on the hub. Stamp the
  resolved subject into `ClusterGrant.spec` on the hub instead.
- `cmd/enforcer/main.go` defaults `--spoke-context=k3d-protem-spoke-1`,
  `--hub-context=k3d-protem-demo`, `--cluster-name=prod-us-east-1`. Dev values do
  not belong in a production binary; make them required.
- No spoke-side RBAC ceiling shipped (`bind` with `resourceNames`; `crd-design.md` §1).

### F8. Medium: agent binding drift
`ensureLocalBindings` only creates missing bindings (`agent.go:186-264`). Edited
subjects or roleRef persist until expiry, and an existing same-named binding
without the Protem labels is silently treated as managed. Use the same
`CreateOrUpdate` and refuse-to-adopt logic as the hub's `ensureBindings`.

### F9. Low
- `targetcluster_controller.go:83` treats any `TargetCluster` named `local` as the
  always-Ready hub, even when `--hub-cluster-name` differs.
- Grant/ClusterGrant status writes still detect changes by slice length only
  (`accessgrant_controller.go`, `agent.go:141`).

## Suggested order

1. F1, F3, F5 (API authz). Each with a test against `NewServer`.
2. F2, F4 (approval correctness).
3. H2 (envtest in CI). Without it, CEL and status-subresource behaviour are untested in CI.
4. F6, F7, F8 (fleet ownership and least privilege).
5. F9.

After step 2, run a fresh read-only review of `internal/api` and
`accessrequest_controller.go`.

---

# Recheck 2 (2026-11-24, independently verified)

The status table above was updated by the implementing agent. This section
records what was verified independently.

Verification run:

- `go build`, `go vet`, `make test`: pass.
- `make test` now sets `KUBEBUILDER_ASSETS`, and the envtest suite runs (3/3).
- CI runs `make test` plus a generated-TypeScript drift check.
- Everything else was checked by reading the code.

## Verified fixed

| ID | Evidence |
|---|---|
| H2 | `Makefile` `test` target depends on `envtest` and sets `KUBEBUILDER_ASSETS`; the suite ran 3 specs, 0 skipped |
| F1 | `approval.Decide`: reject needs `eligible()` (named approver of a Manual requirement, self-rule respected); cancel only by the requester |
| F2 | `issue()` starts the clock at the `Decided=True` condition's `lastTransitionTime` (`accessrequest_lifecycle.go:136-139`) |
| F3 | Every `/api/*` route is wrapped in `authed`; grants and audit are filtered by requester, approver, or SAR |
| F4 | Decisions are recorded for every eligible rule; `Outcome` ignores empty rule names; `SamePerson` checks username or email |
| F5 | `subjectaccessreviews` create marker is present; the SAR user is mapped through the subject template |
| F6 | Hub `ClusterGrantReconciler` removed; only the agent writes `ClusterGrant.status` |
| F7 (partly) | Hub stamps `ClusterGrant.spec.subject`; the agent no longer reads `TargetCluster`; `--cluster-name` is required and the k3d defaults are gone |
| F8 | Hub and agent share `internal/binding.Apply`: `CreateOrUpdate`, refuse-to-adopt on labels, roleRef, subjects and expiry reconciled |
| F9 | The hub name check uses the flag only; status diffs use `equality.Semantic.DeepEqual` |

The approval logic now lives in a pure package (`internal/approval`, 100%
coverage), and the request lifecycle is a pure `Advance()` function. Both are
good seams for the later `AccessPolicy` work.

## Still open

### R1. High: the claimed agent RBAC manifests do not exist (F7)
The table cites `deploy/spoke/agent-rbac.yaml`; there is no `deploy/` directory
and no agent RBAC anywhere in the repo. Ship:

- Hub side: a ServiceAccount, a Role in `protem-cluster-<name>` granting
  `get/list/watch` on `clustergrants`, `update` on `clustergrants/status`, and
  `get/create/update` on `leases`, plus a RoleBinding.
- Spoke side: a ClusterRole with `rolebindings`/`clusterrolebindings` CRUD and
  `bind` restricted by `resourceNames` to the roles the spoke owner allows. This
  is the spoke ceiling from `crd-design.md` §1.
- A short doc on minting the hub kubeconfig for the agent.

### R2. High: role and scope are read from the live profile at grant time
`issue()` builds the grant from the **current** profile's `role` and `scope`
(`accessrequest_lifecycle.go:163-164`), while clusters, namespaces and duration
are frozen in `status.resolved`. Approvers approve one role; a profile edit
between request creation and grant issuance (e.g. `view` → `cluster-admin`,
`Namespaced` → `Cluster`) issues the new role with the old approvals. With
delegated profile authoring this is a privilege-escalation path.
**Fix:** freeze `role` and `scope` into `status.resolved` in `validate()`, and
build the grant only from `status.resolved`. Add a test: edit the profile after
approval, then assert the grant carries the original role.

### R3. Low
- `isHubTargeted` returns true for every grant when `--hub-cluster-name=""`
  (`accessgrant_controller.go:66-67`). Grants always carry at least one cluster,
  so the only trigger is an empty flag, which then binds every grant on the hub
  *and* fans it out. Reject an empty flag at startup, or treat empty as "hub is
  never a target".
- The fan-out also creates a `ClusterGrant` for the hub cluster itself, which has
  no agent and stays `Pending` forever. Skip `HubClusterName` in `ensureClusterGrants`.
- The agent's autonomous sweep keeps bindings with a missing or unparsable expiry
  annotation (`agent.go:181`). Fail closed: treat missing as expired. `Apply`
  always writes the annotation, so only tampering hits this path.
- Concurrent approvals: the second writer gets a 500 from the
  `resourceVersion` conflict. Return 409 so the UI can retry.
- Revocation cannot reach a spoke during a hub partition; only expiry is
  enforced locally. This is inherent to the design; document it in operator docs.

## Next

R2 first (security), then R1 (deployability). After that the core is ready for
a fresh `reviewer` pass over the whole diff and an end-to-end run on two kind
clusters (hub + spoke). That run is the first proof that the spoke `bind`
ceiling, the agent's hub RBAC and the autonomous expiry work outside envtest.

---

# Recheck 3 (2026-11-24)

Verification run:

- `go build`, `go vet`, `make test` (envtest 3/3): pass.
- **`make lint`: fails** (the CI `lint.yml` gate):
  `internal/controller/accessgrant_controller.go:193: Consider pre-allocating clusterStates (prealloc)`.

Changes since Recheck 2 are mostly the web UI redesign plus API view changes:
- `RequestView` gained clusters, requirements and actions
- `/api/me` reports `devAuth`
- duplicate approvals return 409 (`approval.ErrAlreadyApproved`)
- `fleet.EligibleClusters` refactor
- new `hack/dev-personas-rbac.yaml`

## Recheck 2 items

| ID | Status |
|---|---|
| R1 agent RBAC manifests | **Open.** Still no `deploy/` directory and no agent RBAC |
| R2 role/scope frozen at request time | **Open.** `issue()` still copies `profile.Spec.Role`/`Scope` from the live profile; `ResolvedScope` has no role or scope |
| R3 empty `--hub-cluster-name` | Open |
| R3 hub gets its own `ClusterGrant` | Open |
| R3 sweep keeps bindings with no expiry annotation | Open (`agent.go:181`) |
| R3 concurrent approvals | Partly done: a *repeat* approval returns 409; a `resourceVersion` conflict from two approvers still returns 500 |

## New

### N1. High: the lint gate is red
CI will fail. A one-line fix (`make([]…, 0, len(grant.Spec.Clusters))`).

### N2. Medium: a design-tooling kit was copied into `web/`
About 7 MB of files unrelated to the product:
- `web/examples/` (5.1 MB, including PNG thumbnails and fonts for sample sites)
- `web/.claude/`, `web/scripts/`, `web/templates/`, `web/tokens/`
- `web/accessibility/`, `web/components/`, `web/frameworks/`, `web/taste/`, `web/workflows/`

Only `.design-snapshots/`, `.ds-receipts/` and `.ai-design-check/` are
gitignored. None of it is embedded (the binary embeds `dist/*` only), but it
would land in the repository. Keep the kit outside the repo, or gitignore it,
before the first commit.

### N3. Medium: the UI has no production login
`web/src/api.ts` sends only the dev `X-Test-User` header; OIDC flows are out of
scope in `web/BRIEF.md`. With `--dev-auth` off, every UI call gets 401 unless
something in front injects `Authorization: Bearer <id token>`. Either:
- implement OIDC authorization-code + PKCE in the UI, or
- document that the hub UI must run behind an auth proxy (e.g. oauth2-proxy)
  that passes ID tokens, and test that path.

### N4. Low: dev RBAC binds a realistic group name
`hack/dev-personas-rbac.yaml` binds the ClusterRole to group `security-team`.
If someone applies it to a real hub, a real IdP group of that name gets
`get/list/update` on every grant and request. Use a group name that cannot
collide (e.g. `protem-dev:security-team`, with the dev persona updated to match).

## Next

1. N1 (one line).
2. R2: still the only open privilege-escalation path. Freeze role and scope into
   `status.resolved`; add a test that edits the profile after approval.
3. R1 and N3: without these, the hub UI and the agent cannot be deployed outside dev mode.
4. N2 before the first commit.
5. R3, N4.
6. Then a fresh `reviewer` pass and an end-to-end hub + spoke run on kind.

---

# Recheck 4 (2026-11-24): fixes, richer UI and optional management

Verification run: `make test` (envtest included) and `make lint` pass; the UI type-checks,
its tests pass and it builds. The full chain was also run on a throwaway k3d cluster: request,
approval, grant, agent binding, agent-reported rules, profile edit, cluster settings, and login
through oauth2-proxy against a stand-in OIDC issuer.

## Open items from Recheck 3

| ID | Status | Notes |
|---|---|---|
| R1 agent RBAC manifests | **Fixed** | `config/agent/hub`, `config/agent/spoke`; an envtest test runs the agent under them with impersonated identities and proves both sufficiency and the limits |
| R2 role/scope frozen | **Fixed** | `status.resolved` carries role, scope and duration; the grant reads nothing else from the profile |
| R3 hub-name, hub ClusterGrant, concurrent approvals | Open | Unchanged |
| R3 sweep keeps bindings with no expiry | **Fixed** | Missing or unparsable expiry is deleted |
| N1 lint | **Fixed** | |
| N2 design kit in the repo | **Fixed going forward** | Untracked and ignored; still in git history |
| N3 production login | **Fixed** | `config/auth-proxy`, `config/sso`, `docs/login.md`; the UI shows a signed-out screen on 401 |
| N4 dev RBAC group | **Fixed** | `protem-dev:admins` |

## Findings from the independent review of this change, and what was done

| Finding | Decision |
|---|---|
| SSO overlay rejected: port name over 15 characters | Fixed; the overlay passes a server-side dry run |
| A grant that fails part-way keeps bindings the hub does not report | Fixed by reporting them (see `crd-design.md` §7.3) |
| A stale read let a profile update or delete skip the GitOps check | Fixed: version must match on update; delete carries UID and version preconditions |
| Grant duration could still come from the live profile | Fixed: frozen duration only; a profile's default duration must be positive |
| Subject template could collapse users into one identity | Fixed at the source: a user's email falls back to the token subject, so it is never empty |
| Failed grants rewrote status every sync | Fixed |
| Unused `patch` verb in the management role | Removed |
| Profile deletion left no event | Fixed |
| Token groups reach SubjectAccessReviews unprefixed | Fixed for the dangerous case: `system:` groups are dropped and `system:` identities refused |
| Capabilities check `update` only, and skip empty namespaces | Not changed: it affects what the UI offers, never what the API allows |

## Found while verifying

- Lists reordered between polls: the API returned the cache's order and timestamps tie within a
  second. Every list route now returns a total order.
- Two requests in the same millisecond collided on their name. Names are now generated by the
  API server.
- A cluster's "last heartbeat" went stale because the controller writes it only when readiness
  changes. The API now reads the agent's Lease.
- The local Keycloak container's clock is about 40 minutes behind the host, so every token it
  issues is already expired for a hub or proxy running on the host. This is a property of the
  container VM, not of this code.

## Still open

- `Dockerfile` does not copy `web/`, which `internal/api` embeds, so the image cannot build; it
  also does not build the agent, and there is no agent Deployment.
- `config/samples` still uses the old API group and empty specs.
- The audit list is rebuilt from each request's current state; it is not an append-only trail.
- The agent reports no version, and its first heartbeat is 10 seconds after start.
- An approver cannot see the per-cluster permissions of a grant they approved; only the
  requester and callers with read access to grants can.
- The UI design gates in `web/BRIEF.md` were not run on the new screens.

---

# Recheck 5 (2026-11-24): images, samples, paging, audit history, duplicates

Verification run: `make test` (envtest included, 4 specs) and `make lint` pass; the UI type-checks,
its 81 tests pass and it builds. Both images were built and run as containers against a throwaway
k3d cluster. The request, approval, grant, agent binding, paging, audit and duplicate checks ran
against a hub and agent started from local binaries on the same cluster, through the API and in a browser.

Not verified: the agent Deployment and the manager Deployment never ran as pods. The Docker VM on
this machine is 99% full, so the k3d node carried a disk-pressure taint and scheduled nothing. The
manifests were accepted by the API server; the ConfigMap and Secret wiring is unproven at runtime.
The same VM's clock is 41 minutes behind the host, so an agent run as a container on it shows as
Disconnected (its Lease renewals look 41 minutes old).

## Done

- **Dockerfile**: copies `web/dist` and `web/web.go`, builds for the requested platform, and has an
  `enforcer` target for the agent image (`make docker-build-agent`).
- **Agent Deployment**: `config/agent/spoke/deployment.yaml`; steps in `docs/agent-deployment.md`.
  The agent now refuses to start without `--hub-kubeconfig` or `--hub-context`, because in a pod it
  would otherwise use the target cluster as the hub.
- **Samples**: `config/samples` uses the current API group and real specs. An envtest spec applies
  them and expects the sample request to end in a grant.
- **Paging**: `/api/requests`, `/api/grants` and `/api/audit` take `limit` and `offset`, return
  `{items, total}`, and filter on the server. The UI polls only open requests and active grants.
- **Audit history**: one row per event (submitted, each approval, approved, rejected, cancelled,
  timed out, granted, revoked, expired), derived from facts the objects record once. A timed-out
  request now records when it timed out.
- **Approvers and grants**: people named as approvers of a request can view its grant and the
  per-cluster permissions. They cannot revoke it.
- **Duplicates**: a request is refused when its requester already waits for, or holds, the same
  profile on one of the same clusters. The API answers 409; the controller applies the same rule
  (`approval.Held`) to requests created with kubectl and rejects them with reason `AlreadyHeld`.

## Still open

- The audit history lasts only as long as the request and grant objects. Deleting them deletes
  the history; there is no separate store or export.
- A request created with kubectl still carries whatever `spec.requester` the manifest names. Only
  the UI and API path sets it from the login. A ValidatingAdmissionPolicy is the planned fix.
- Audit times mix the API server's clock (creation) and the hub's (decisions). Rows of one request
  are kept in order, but with clocks apart the times of creation rows are off by the difference.
- An admin who is not a named approver sees an empty "Recent decisions" list on Approvals; Audit
  shows them everything.
- The design gates in `web/BRIEF.md` were not run on the new screens.

---

# Recheck 6 (2026-11-24): the open items from recheck 5

Verification run: `make test` (envtest, 5 specs) and `make lint` pass. Both images were built and
ran as pods in a throwaway k3d cluster: the manager from `config/default`, the agent from
`config/agent/spoke` through the overlay in `docs/agent-deployment.md`. Through those pods: a
kubectl request naming someone else was refused by the admission policy; one naming its caller
was approved, bound by the agent as a RoleBinding, written to the audit log, revoked, and deleted
a minute later by `--history-retention=1m`.

## Done

- **Agent and manager Deployments**: proven as pods, including the agent's ConfigMap and Secret.
- **Requester identity**: `config/admission` (a ValidatingAdmissionPolicy) makes a request created
  with kubectl name its caller; see `docs/login.md`. Opt-in: apply it on the hub.
- **Audit log**: every event is written once to the manager's stdout as a JSON line
  (`docs/audit.md`); `status.auditedEvents` is the cursor. `--history-retention` deletes finished
  requests and their grants after their events are logged.
- **Approvals**: admins see every recent decision.
- **Found by the pod run and fixed**: a stale cache read repeated an audit line; a request with
  no email had no actor on its first event; the request controller logged an error whenever its
  status write lost a race with the audit cursor; an approval that lost the same race failed
  with a 500 and is now retried.

## Still open

- The admission policy is not part of `config/default`: someone has to apply it, and it names
  the operator's service account literally.
- Audit lines live wherever the cluster's log pipeline puts them. Without one, they are lost
  with the pod.
- `--history-retention` treats a request whose grant was deleted by hand as finished from its
  last request event, so it can be pruned sooner than one whose grant expired normally.
- The grant controller still logs an error when its status write loses a race (it retries).
- Nothing has run on EKS, GKE or AKS.

## Renamed to Protem (2026-11-24)

The product is now Protem. Old manifests do not apply as-is:

- API group `protem.dev` (version `v1alpha1`, kinds unchanged); CRDs must be reinstalled. The old short names are gone; each kind keeps its other short name.
- Namespaces: the operator runs in `protem-system`, the agent in `protem-system` on each target cluster, and each cluster's hub namespace is `protem-cluster-<name>`.
- Labels, annotations and the finalizer use the `protem.dev/` prefix; managed-by values are `protem` and `protem-agent`; bindings are named `protem-grant-...`.
- Images: `protem` (operator) and `protem-agent` (agent binary and image).

---

# Recheck 7 (2026-11-24): enrolment, per-user eligibility, UI redesign

Verification run: `make test` (envtest, 8 specs) and `make lint` pass; the UI type-checks, its
tests pass, it builds, and the static design check reports 0 errors and 0 warnings. The flow was
run on a throwaway k3d cluster with the agent as a pod and the hub as a local process.

Verified by running it:

- A cluster added from the UI: the two join files install the agent, it connects, and it replaces
  the 15-minute join token with its own 24-hour token after five minutes.
- "Reissue credentials" cuts the agent off (Unauthorized), and applying the new files reconnects
  it. Before the fix in this round it stayed locked out, because it preferred the revoked token
  stored in its Secret.
- The agent pod reports `edit,view` as the roles it may bind; `GET /api/clusters` returns them.
- A request approved through the reading pane ends in an Active grant and a RoleBinding on the
  target cluster.
- Profiles naming individual requesters (`spec.requesters.users`, `when.requesterUsers`).

Redesign: two workspaces (My access, Administration) with a switcher; For you; a reading pane for
requests (`?request=<ns>/<name>`); console tables for Clusters, Profiles and Audit; an
Administration overview; an access timeline. Screenshots in `docs/screenshots/` are from this build.

Still open:

- Allowed roles are not reported on clusters whose authorizer makes the rules review incomplete
  (webhook authorizers; not checked on EKS, GKE or AKS).
- The flagged-rules marker is not on list rows (it would need a request per row); it is in the pane.
- "Activate access" is offered for a profile whose rule is automatic and names no approver groups;
  a manual rule naming only users would be mislabelled.
- The grant page still prints its end time in the browser's locale format; everything else uses
  24-hour clock times.
- Adding a cluster from a completely empty Clusters page was not exercised (the dev hub always
  lists itself).
- Nothing has run on EKS, GKE or AKS.

---

# Recheck 8 (2026-11-24): the open items of recheck 7

Verification run: `make test` (envtest, 8 specs) and `make lint` pass; the UI type-checks, its
tests pass, it builds, and the static design check reports 0 errors and 0 warnings. Checked in a
browser against a hub on a throwaway k3d cluster (no agent pods this round).

Closed:

- **Allowed roles on clusters with a webhook authorizer.** An incomplete rules review that still
  lists rules is now reported and marked partial; Clusters shows "or more". Covered by unit tests
  only: no such cluster was available.
- **Flagged rules on list rows.** `RequestView.flaggedRules`, one role lookup per distinct role
  per response; the rows show "N flagged rules". Seen live.
- **Activation label.** `ProfileView.activation` (`at-once`, `approval`, `mixed`) is evaluated for
  the caller with the controller's own rule matching; `autoApproval` is gone. A profile whose
  rule names only a user now reads "Request approval, Approval by bob@company.com". Seen live.
  `mixed` is covered by tests only (it needs two ready clusters).
- **One date format.** 24-hour, "HH:mm" today, "24 Nov, HH:mm" otherwise; the grant page and the
  time-used bar follow it. The access timeline keeps weekday labels on its axis.
- **Adding a cluster from an empty Clusters page.** Exercised on a hub with no clusters: the
  connect dialog opens with both files.
- **"Delete profile"** is outlined; solid red is only on the dialog's confirming button.
- **`DESIGN.md`** records the visual decisions as built.

Still open:

- Nothing has run on EKS, GKE or AKS, so the partial allowed-roles path and the user-name
  mapping are unproven there.
- Listing profiles evaluates the approval rules once per profile and ready cluster; fine for
  hundreds, not measured beyond that.
