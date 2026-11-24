# An admission policy enforces requester identity for kubectl

A request created with `kubectl` must name its own creator as the requester. A `ValidatingAdmissionPolicy` checks this at the API server, with one exemption for the operator itself.

**Status:** Accepted, 2026-11-23.

## Context

The UI creates `AccessRequest` objects through the operator, which fills in `spec.requester` from the person's verified login, so it is always correct there. A request created directly with `kubectl` has no such guarantee: `spec.requester` is whatever the manifest says, and approval rules, the self-approval check, and the eventual binding all trust that field. Without a check, anyone able to create an `AccessRequest` could name someone else as the requester and have access granted in that person's name.

The operator itself needs an exception: it creates requests on behalf of whoever is logged in to the UI, so it cannot be bound by "the creator is the requester".

## Decision

`config/admission/requester_identity.yaml` installs a `ValidatingAdmissionPolicy` named `requester-identity` on `CREATE` of `accessrequests`. Its `matchConditions` skip the check entirely for any caller whose RBAC grants the custom verb `delegate` on `accessrequests` in the request's namespace — the operator holds this verb and nobody else does by default. For every other caller, it requires:

- `spec.requester.username` equals the caller's authenticated username,
- `spec.requester.email` is empty or equal to that same username,
- every group in `spec.requester.groups` is one of the caller's own groups.

The policy fails closed (`failurePolicy: Fail`, `validationActions: [Deny]`), and it requires Kubernetes 1.30+ for `ValidatingAdmissionPolicy`'s stable API. It is installed by default (`config/default` references `config/admission`; the Helm chart's `admissionPolicy.enabled` defaults to `true`).

## Consequences

- A request made directly with `kubectl` can only ever name its own creator, closing the identity-spoofing path the landscape review flagged as critical (3.2).
- The operator's own ServiceAccount must hold `delegate` on `accessrequests` (`config/rbac/role.yaml`) or every UI-created request is rejected by this same policy.
- Running on Kubernetes older than 1.30, or with the admission policy disabled, removes this protection for anyone who can create `AccessRequest` objects directly; the UI path is unaffected either way, because the operator always fills in the requester itself.
- Granting the `delegate` verb to anyone besides the operator lets them name any requester, so it is as sensitive as write access to `AccessProfile`.
