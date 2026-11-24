# Metrics

What the operator's metrics endpoint serves, and how to reach it.

Protem registers no metric of its own. The endpoint serves only
[controller-runtime](https://book.kubebuilder.io/reference/metrics.html)'s built-in collectors:
`controller_runtime_reconcile_total`, `controller_runtime_reconcile_errors_total`,
`controller_runtime_reconcile_time_seconds` and similar, per controller (`accessrequest`,
`accessgrant`, `accessprofile`, `targetcluster`, `audit`), plus `workqueue_*`,
`rest_client_requests_total`, `leader_election_*`, and the Go and process collectors. There is no
request-count, grant-count or audit-event metric; count those from the API (`/api/stats`) or the
audit log instead.

## Serving it

`--metrics-bind-address` (chart: always set when `metrics.enabled` is `true`, default `true`) sets
the address the endpoint binds to; `0` disables it. `--metrics-secure` (default `true`) serves it
over HTTPS on `:8443` and requires a bearer token the caller is authorized for (`get` on the
non-resource URL `/metrics`); set it `false` to serve plain HTTP instead. `--metrics-cert-path`,
`--metrics-cert-name` and `--metrics-cert-key` point it at a certificate instead of the
self-signed one it generates.

The chart's `metrics.enabled` adds the Service, and ClusterRoles `metrics-auth-role` (lets the
operator verify callers) and `metrics-reader` (bind this to whatever scrapes `/metrics`).
`config/prometheus/monitor.yaml` is a `ServiceMonitor` for the Prometheus Operator, scraping
`https://.../metrics` with a ServiceAccount token; it is not installed by default (uncomment the
`PROMETHEUS` lines in `config/default/kustomization.yaml`), and the chart has no equivalent
template, so add your own `ServiceMonitor` modelled on it if you use the chart.

See [Operator flags](../reference/operator-flags.md) and
[Chart values](../reference/chart-values.md) for the exact flags and values.
