# Product

## Platform

web

## Users

Three kinds of people use the same UI, in this order of priority for layout (confirmed by the owner, 2026-11-24):

1. **Requesters:** engineers who need a role on one or more clusters for a few hours, often in the middle of an incident. Their job is to get in with the fewest steps and to know how long they have.
2. **Approvers:** on-call leads and team leads who are asked to decide on someone else's request. Their job is to see what is waiting, judge what the access allows, and decide in seconds, sometimes from a phone.
3. **Administrators:** platform and security engineers who own profiles (what can be requested, by whom, approved by whom), the fleet of clusters, and the audit trail.

It must feel right for a team with a handful of clusters and for an enterprise with hundreds of clusters, hundreds of profiles and hundreds of requests a day.

## Product Purpose

Protem gives people temporary Kubernetes access across many clusters. A person requests a role for a set time with a reason; the profile's rules decide whether it is granted at once or needs approval; the access is an ordinary RoleBinding that is removed on its own when the time is up. Success is that nobody holds standing privileged access "just in case".

## Positioning

- Access is plain Kubernetes RBAC, so it needs no changes to a cluster's API server and is built for managed clusters.
- Everything is a Kubernetes custom resource on one hub cluster: no database, and profiles can live in Git. Objects managed by a GitOps tool are read-only in the UI.
- An agent on each cluster pulls its grants from the hub. The hub holds no credentials for any other cluster, and each cluster's owner lists the roles that may ever be granted there.
- Access ends on time even when the hub is unreachable.

## Operating Context

- People reach the UI through a login proxy and their organisation's identity provider; who they are and which groups they are in comes from that login.
- Used mostly in a desktop browser. A phone must work for approving a request while on call.
- A profile makes people eligible, by group; a request for a profile is granted automatically or goes to named approvers, depending on the cluster and the requester. This is the same model as Azure PIM's "eligible assignment, activate, approve".
- Requests, grants and their audit events refresh on their own every few seconds; nobody should need to reload.
- Administration from the UI is optional (`--ui-management`); many installs keep profiles in Git and use the UI read-only.
- The UI ships inside the operator's single binary. It may call only its own API.

## Capabilities and Constraints

- Screens that exist: Home, My access, Request access (catalogue and form), Request detail, Grant detail with the permissions each cluster reported, Approvals, Profiles (list, detail with permission rules and risk flags, editor), Fleet (clusters, settings, add, connect agent, reissue, remove), Audit (event history with filters).
- Terms are fixed in `GLOSSARY.md`: hub, target cluster, agent, profile, request, grant. Never "spoke" or "enforcer" in anything a person reads.
- Not available today, so the UI must not imply them: notifications by chat or email, extending a grant, bulk approval, saved table views, export.
- Eligibility by named user (not only by group) is being added.

## Brand Commitments

- Name: Protem. Mark: "Interval", two brackets closing a square (`brand/`). Colour: Protem Blue `#1D5FA8`, with `#4888D5` on dark surfaces. Typeface: Public Sans, with IBM Plex Mono for identifiers and code.
- Light and dark themes.
- Wording rules: controls are a verb plus an object, headings are nouns, sentence case, errors say what happened and what to do.

## Evidence on Hand

- A working product: the UI runs against a real hub with real requests, grants, clusters and audit events.
- Screenshots in `docs/guides/images/`, diagrams and a poster in `docs/architecture/images/`.
- No customers, testimonials, benchmarks or production deployments exist; none may be implied. It has run on kind, k3d, AKS and EKS only.

## Product Principles

1. The person's own tasks come first: what they can activate, what is waiting for them, what they hold now. Administration is a separate area.
2. Before anyone approves, they can see exactly what the access allows.
3. Time is always visible: how long access lasts, how long is left, how long a request has waited.
4. The UI never shows a control the person cannot use, and never offers what the hub's permissions would refuse.
5. It reads the same at ten clusters and at a thousand: lists search, filter and page rather than grow.

## Accessibility & Inclusion

WCAG 2.2 AA: keyboard access to every control, visible focus, text contrast of at least 4.5:1, targets large enough for touch, reduced-motion respected. Status is never carried by colour alone.
