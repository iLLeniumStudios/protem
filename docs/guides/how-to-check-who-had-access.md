# How to check who had access

Find out who held access, when, and why, in the place that fits the question.

- **Audit page** (`/audit`) — every event of every request and grant that still exists:
  submitted, approved, rejected, cancelled, timed out, granted, revoked, expired. Filter by
  event type or cluster, and search by email, profile, ticket, reason or comment.
- **Access timeline** (`/timeline`, admin) — every grant live right now, by cluster, with who
  holds it, its role, and when it ends; search by cluster, person or role, and zoom the time
  window.
- **The raw audit log** — the full history, including requests and grants already deleted. See
  [Audit log](../operations/audit-log.md) for its event shape and where it is written; this page
  only points to it, it doesn't restate the field list.

The Audit and Access timeline pages only cover requests and grants that still exist in the
cluster; once one is deleted (by hand, or by `--history-retention`), its events remain only in
the raw log.
