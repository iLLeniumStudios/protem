# Kubernetes resources are the only store

Protem keeps no database of its own. Every profile, request and grant is a Kubernetes custom resource, and the audit trail is log lines with optional retention, not a separate store.

**Status:** Accepted, 2026-11-23.

## Context

An access-request system needs somewhere to hold its state (profiles, requests, grants) and somewhere to hold its history (what was requested, approved, revoked, by whom). Both could live in a dedicated database, which would let the hub query and retain history independently of the cluster it runs on.

A second store is also a second thing to secure, back up and keep consistent with the cluster, and it would duplicate what Kubernetes already does well: watch, list, RBAC-gate and version objects.

## Decision

State is CRDs only: `AccessProfile`, `AccessRequest`, `AccessGrant`, `ClusterGrant` and `TargetCluster` live in `etcd` through the Kubernetes API, read and written like any other Kubernetes object. There is no database driver or client anywhere in the module.

History is derived, not stored separately. `internal/audit/events.go` computes the event list for a request and its grant from their current state; `internal/controller/audit_controller.go` writes each new event as one JSON line (`Out io.Writer`, the operator wires this to stdout) and tracks how many lines it has written in `status.auditedEvents` so a restart does not repeat them. A finished request (and its grant) is deleted `--history-retention` after its last event; `0`, the default, keeps it forever.

## Consequences

- Nothing to back up beyond the cluster itself; disaster recovery is restoring the cluster's resources.
- The audit trail is only as durable as wherever the operator's stdout is shipped. Protem does not protect against a tampered log: see the security model's "What Protem does not protect against" section. Shipping it to a store you control is the operator's responsibility, documented in [the audit log reference](../operations/audit-log.md).
- `--history-retention` bounds how long a finished request's CRD (and the custom-resource storage behind it) is kept; it does not bound log retention, which is whatever the log pipeline keeps.
- Querying history means reading log lines, not running a query language against a database; there is no query interface beyond `GET /api/audit`, which reads the same derived events.
