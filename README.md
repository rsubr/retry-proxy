# retry-proxy

A durable HTTP retry proxy. Accepts requests, persists them to SQLite, returns `202 Accepted` immediately, then forwards to upstream APIs with automatic retries.

Requests survive process restarts and machine reboots.

---

## How it works

```
Client → retry-proxy → SQLite queue → workers → upstream API
```

1. Request arrives
2. Persisted to SQLite
3. `202 Accepted` returned to client
4. Background workers forward to upstream
5. Failed requests retried with exponential backoff
6. Respects `Retry-After` headers

---

## Install

### Download binary

```bash
# Linux amd64
curl -L https://github.com/rsubr/retry-proxy/releases/latest/download/retry-proxy-linux-amd64 -o retry-proxy
chmod +x retry-proxy

# Linux arm64
curl -L https://github.com/rsubr/retry-proxy/releases/latest/download/retry-proxy-linux-arm64 -o retry-proxy
chmod +x retry-proxy
```

### Docker

```bash
docker pull rsubr/retry-proxy:latest
```

### Build from source

```bash
CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o retry-proxy ./cmd/retry-proxy/
```

---

## Configuration

Create `config.yaml`:

```yaml
listen: ":8080"

database:
  path: "./queue.db"

worker:
  concurrency: 10
  poll_interval: 5s

http:
  timeout: 30s

retry:
  max_duration: 10m

  retry_status_codes:
    - 429
    - 500
    - 502
    - 503
    - 504

  backoff:
    strategy: exponential
    initial: 5s
    max: 60s

routes:

  - name: crm
    match:
      prefix: /crm
    rewrite:
      strip_prefix: true
    target:
      base_url: https://api.crm.com

  - name: billing
    match:
      prefix: /billing
    rewrite:
      strip_prefix: true
      regex:
        - pattern: "^/customer/(.*)$"
          replacement: "/api/v2/customers/$1"
        - pattern: "^/invoice/(.*)$"
          replacement: "/api/v1/invoices/$1"
    target:
      base_url: https://billing.example.com

  - name: legacy
    match:
      regex: "^/legacy/(.*)$"
    rewrite:
      regex:
        - pattern: "^/legacy/(.*)$"
          replacement: "/v3/$1"
    target:
      base_url: https://new-api.example.com
```

### Configuration reference

| Field | Default | Description |
|-------|---------|-------------|
| `listen` | `:8080` | Address to listen on |
| `database.path` | `./queue.db` | SQLite database file path |
| `worker.concurrency` | `10` | Number of parallel workers |
| `worker.poll_interval` | `5s` | How often idle workers poll for new jobs |
| `http.timeout` | `30s` | Upstream request timeout |
| `retry.max_duration` | `10m` | How long to retry before marking job expired |
| `retry.retry_status_codes` | `429,500,502,503,504` | HTTP status codes that trigger retry |
| `retry.backoff.strategy` | `exponential` | Backoff strategy (only `exponential` supported) |
| `retry.backoff.initial` | `5s` | Initial backoff delay |
| `retry.backoff.max` | `60s` | Maximum backoff delay |
| `cleanup.max_age` | `168h` | Queued jobs older than this are marked `expired` |
| `cleanup.interval` | `1h` | How often the cleanup sweep runs |

---

## Run

```bash
./retry-proxy config.yaml
```

Docker:

```bash
docker run -d \
  -p 8080:8080 \
  -v $(pwd)/config.yaml:/config.yaml \
  -v $(pwd)/data:/data \
  rsubr/retry-proxy:latest
```

---

## Routes

Routes are evaluated in order. First match wins.

### Prefix match

```yaml
match:
  prefix: /crm
```

Matches `/crm`, `/crm/`, `/crm/users`, `/crm/users/123`.

### Regex match

```yaml
match:
  regex: "^/legacy/(.*)$"
```

Uses Go `regexp` syntax.

### Strip prefix

```yaml
rewrite:
  strip_prefix: true
```

`/crm/users/123` → `/users/123`

### Regex rewrite

```yaml
rewrite:
  regex:
    - pattern: "^/customer/(.*)$"
      replacement: "/api/v2/customers/$1"
```

Applied after strip_prefix. Rules run in order.

---

## HTTP API

### Send a request

Any non-system path is treated as a proxy request.

```bash
curl -X POST http://localhost:8080/crm/customers/123 \
  -H "Content-Type: application/json" \
  -d '{"name": "Acme Corp"}'
```

Response:

```json
HTTP/1.1 202 Accepted

{
  "accepted": true,
  "job_id": 42
}
```

Returns `202` after persisting to SQLite. Returns `500` if persistence fails. Returns `404` if no route matches.

### Check job status

```bash
curl http://localhost:8080/_proxy/jobs/42
```

Response:

```json
{
  "id": 42,
  "state": "completed",
  "retry_count": 1,
  "response_code": 200,
  "created_at": "2026-01-01T12:00:00Z",
  "completed_at": "2026-01-01T12:00:35Z"
}
```

Job states: `queued`, `processing`, `completed`, `failed`, `expired`

### Health check

```bash
curl http://localhost:8080/_proxy/health
```

```json
{"status": "ok"}
```

---

## Retry behaviour

- Network errors (connection refused, timeout, DNS failure, TLS errors) are retried automatically
- HTTP status codes in `retry_status_codes` trigger retry
- Other status codes mark the job completed (including 4xx errors)
- `Retry-After` response header is respected (seconds or HTTP date format)
- Jobs exceeding `max_duration` are marked `expired`
- On startup, any `processing` jobs from a previous crash are re-queued

### Backoff example

With `initial: 5s` and `max: 60s`:

```
attempt 1 → 5s
attempt 2 → 10s
attempt 3 → 20s
attempt 4 → 40s
attempt 5 → 60s
attempt 6 → 60s
...
```

---

## What is preserved

Forwarded to upstream:

- HTTP method
- Path (after rewrite)
- Query string
- Request body
- Headers (except `Host`, `Content-Length`, `Connection`, `Transfer-Encoding`)

---

## Logging

JSON output via `log/slog`:

```json
{"level":"INFO","job_id":42,"route":"crm","attempt":2,"status":429}
```

---

## Design

- Single binary
- Single SQLite database
- No external dependencies beyond SQLite and YAML
- No clustering
- No web UI
- No authentication
