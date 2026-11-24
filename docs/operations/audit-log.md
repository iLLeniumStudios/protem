# Audit log

The shape of every audit event the operator writes, where the log goes, and how long it is kept.

The operator writes every audit event of an AccessRequest, and of the AccessGrant it produces, as one line of
JSON. The log outlives the objects: once a request or grant is deleted, its history is still in the log.

## Events

- `REQUEST_SUBMITTED`: the requester created the request.
- `APPROVAL_GIVEN`: an approver approved it (one line per approver).
- `REQUEST_APPROVED`: the request has every approval it needs, or needed none.
- `REQUEST_REJECTED`: an approver rejected it, or it failed validation.
- `REQUEST_CANCELLED`: the requester cancelled it.
- `REQUEST_TIMED_OUT`: nobody decided in time.
- `ACCESS_GRANTED`: the grant was created and access started.
- `ACCESS_REVOKED`: someone ended access early.
- `ACCESS_EXPIRED`: access ended at its expiry time.

Example:

```json
{"kind":"ProtemAudit","id":"team-a/req-1#3","timestamp":"2026-11-01T09:05:00Z","eventType":"REQUEST_APPROVED","namespace":"team-a","request":"req-1","requester":"alice@company.com","profile":"db-admin","clusters":["cluster-a"],"ticket":"INC-7","reason":"Fix INC-7"}
```

`id` is `<namespace>/<request>#<sequence>`. `actor` is who did it and is left out when the system did.

## Where it goes

Audit lines go to the standard output of the manager pod. The operator's ordinary logs go to standard error,
so standard output carries nothing else.

Collecting the manager container's output and selecting lines containing `"kind":"ProtemAudit"` is enough to
route it anywhere with your cluster's log pipeline (Fluent Bit, Vector, Loki, a cloud logging agent, ...). The
operator does not store the log anywhere else; a pod's output is lost when the pod goes unless something
collects it.

## Delivery

Each event is written at least once. The operator writes the line, then records on the request
(`status.auditedEvents`) that it did. If it stops between the two, it writes the same event again after the
restart, with the same `id`, so a reader deduplicates on `id`.

## Retention

`--history-retention` (default `0`) deletes finished requests, and with them their grants, this long after
their last audit event. `0` keeps everything forever; any other value must be at least `1m`.

A request is finished when it was rejected, cancelled or timed out, or when it was approved and its grant is
gone, revoked or expired. Nothing is deleted before all its events have been written to the log.

## The UI audit page

The UI builds its audit page from the requests and grants that still exist. Once a request is deleted, by hand
or by `--history-retention`, its events are only in the log.
