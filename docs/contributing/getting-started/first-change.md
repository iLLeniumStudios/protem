# Make your first change

This walks a clean checkout of Protem through the checks a real change has to pass, ending with the same commands continuous integration runs.

## Clone and run the tests

```
git clone https://github.com/iLLeniumStudios/protem.git
cd protem
make test
```

`make test` first runs `manifests`, `generate`, `fmt` and `vet`, then downloads the `envtest` Kubernetes binaries for the version in `go.mod` and runs `go test ./internal/... -v -coverprofile cover.out`. Expect it to end with `PASS` and a per-package summary; the controller and API packages run real envtest suites, so this takes longer than a unit test run.

## Run the linter

```
make lint
```

This downloads `golangci-lint` (pinned to the version in the `Makefile`) if it is not already present, then runs it over the module. A clean run prints no issues and exits `0`.

## Check the web UI

```
cd web
bun install
bunx tsc --noEmit
bun test src
bun run build
```

Each step should finish without errors: the type-check reports no type errors, `bun test src` runs the UI's test suite, and `bun run build` produces a production build.

## Regenerate generated files

Several files in the repository are generated from source: CRD manifests and RBAC from kubebuilder markers, TypeScript types from `internal/api/views.go`, the Helm chart's CRDs and ClusterRole from `config/`, and the reference documentation from the CRDs and chart values. After changing any of their sources, regenerate all of them from the repository root:

```
make manifests generate generate-ts helm-sync docs-reference
```

Commit whatever these produce alongside your change. CI fails a pull request whose generated files are out of date rather than regenerating them for you.

## What CI checks

Every push and pull request runs four jobs (`.github/workflows/test.yml`) plus a separate lint job (`.github/workflows/lint.yml`):

- **Go** — runs `make test`, then runs `make manifests generate generate-ts` again and fails if that changes anything under `api`, `config/crd`, `config/rbac` or `web/src/generated`.
- **Web UI** — installs with `bun install --frozen-lockfile`, then runs `bunx tsc --noEmit`, `bun test src` and `bun run build`.
- **Helm chart** — runs `make manifests helm-sync` and fails if that changes anything under `deploy/helm`, then runs `make helm-lint`.
- **Container images** — builds the operator image and the agent image (the `enforcer` build target) with Docker Buildx, without pushing either.
- **golangci-lint** — the separate workflow that runs the same linter as `make lint`.

A change that passes every command above locally passes CI for the same reason: these are the commands CI itself runs.
