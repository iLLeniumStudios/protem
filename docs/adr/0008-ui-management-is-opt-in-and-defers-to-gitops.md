# UI management is opt-in and defers to GitOps

Editing `AccessProfile` and `TargetCluster` objects from the UI is off by default, and even when turned on, an object a GitOps tool manages stays read-only in the UI.

**Status:** Accepted, 2026-11-23.

## Context

Many Protem installs manage profiles and clusters as code, applied by Argo CD, Flux or plain `kubectl apply`. A UI that can write these objects is convenient for installs that do not use GitOps, but dangerous for ones that do: an edit made in the UI would be silently reverted on the GitOps tool's next sync, with no indication to whoever made it that their change did not stick.

## Decision

`--ui-management` (`readonly` or `enabled`, default `readonly`) gates every configuration write route. In `readonly` mode, `registerRoutes` in `internal/api/server.go` registers no `POST`/`PUT`/`PATCH`/`DELETE` handler for profiles or clusters at all (guarded by `if s.Management`), so a bug in the UI cannot reach a write path that does not exist.

Even with management enabled, each write first checks whether the specific object is GitOps-managed. `managedBy()` in `internal/api/permissions.go` inspects the object for the Argo CD tracking annotation or label, the Flux Kustomize/Helm-controller labels, the `app.kubernetes.io/managed-by: Helm` label, or Protem's own `protem.dev/managed-by` label, in that order, and returns the owner's name. `refuseManaged()` in `internal/api/management.go` calls this before any profile or cluster write and returns `409 Conflict` naming the owner ("change it in Git instead") rather than performing the write.

## Consequences

- A fresh install only ever shows read views for profiles and clusters until an operator deliberately enables management and grants the separate write ClusterRole (`config/rbac/ui_management_role.yaml`).
- An object under GitOps management is visibly locked in the UI ("Managed by GitOps") instead of accepting an edit that would vanish on the next sync.
- Detection is label/annotation-based: an object managed by a GitOps tool through some other mechanism, or one whose tracking metadata was stripped, is not protected by this check and can be edited from the UI like any other object.
- `AccessGrant` and `ClusterGrant` are never UI-writable regardless of this setting; a hand-made grant would bypass approval entirely; this is a separate rule from the GitOps lock.
