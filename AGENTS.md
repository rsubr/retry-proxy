# Repository Guidelines

## Project Structure & Module Organization

`retry-proxy` is a Go service that accepts HTTP requests, persists them in SQLite, and retries delivery to upstreams. The entry point is `cmd/retry-proxy/main.go`. Internal packages are under `internal/`: `api` for HTTP handlers, `config` for YAML loading/defaults, `database` for SQLite setup, `jobs` for queue models/workers/cleanup, `routing` for route matching and rewrites, and `upstream` for outbound HTTP calls. Configuration examples live in `config.yaml`; release automation is in `.github/workflows/release.yml`.

## Build, Test, and Development Commands

- `go test ./...`: run all package tests.
- `go test -race ./...`: run tests with the race detector for concurrency-sensitive changes.
- `go build ./cmd/retry-proxy/`: build the local binary with default settings.
- `CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o retry-proxy ./cmd/retry-proxy/`: produce the stripped release-style binary.
- `go run ./cmd/retry-proxy/ config.yaml`: run locally against the sample configuration.
- `docker build -t retry-proxy .`: verify the production container build.

## Coding Style & Naming Conventions

Use standard Go formatting: run `gofmt` on changed `.go` files before committing. Keep package names short, lowercase, and aligned with their directory purpose. Export only APIs needed across packages. Prefer structured errors with `%w`, `context.Context` for request-scoped values, and `log/slog` for service logs. YAML fields use snake_case tags, matching `config.yaml`.

## Testing Guidelines

Tests should use Go's standard `testing` package and live beside the code as `*_test.go`. Name tests by behavior, for example `TestRouterMatchesPrefix`. Favor table-driven tests for routing, rewrite, retry, and config default cases. Database tests should use temporary SQLite files from `t.TempDir()` and avoid writing `queue.db` in the repository root.

## Commit & Pull Request Guidelines

Recent commits use concise imperative subjects such as `Add cleanup sweep to expire stale queued jobs` and `Fix build: commit cmd/ dir, align go.mod and Dockerfile versions`. Follow that style: describe the change in one line, optionally prefixing with `Fix`, `Add`, or `Update`. Pull requests should include the problem, implementation summary, test results such as `go test ./...`, and any configuration or migration impact. Link related issues when available.

## Security & Configuration Tips

Do not commit real upstream credentials, production database files, or local queue data. Treat `config.yaml` as an example; document new configuration fields in both `internal/config` defaults and `README.md`. Be careful with request bodies and headers in logs because this proxy may handle sensitive payloads.
