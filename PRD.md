Product Requirements Document (PRD)

Project Name

Durable HTTP Retry Proxy

Version

1.0

Purpose

A standalone Go service that accepts HTTP requests, persists them to SQLite, immediately acknowledges receipt, and asynchronously forwards them to configured upstream APIs.

The service provides durable retries for rate-limited or temporarily unavailable APIs. Requests survive process restarts, machine reboots, and application crashes.

The service functions as a lightweight API gateway with:

- Route matching
- Path rewriting
- Regex rewriting
- Prefix stripping
- Durable request persistence
- Retry scheduling
- Retry-After support

The service is intentionally simple:

- Single binary
- Single SQLite database
- Single process
- No clustering
- No external dependencies beyond SQLite and YAML libraries

---

Goals

Functional Goals

The system must:

1. Accept arbitrary HTTP requests.
2. Support GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS.
3. Persist requests before acknowledging receipt.
4. Return HTTP 202 Accepted immediately.
5. Route requests to configured upstream APIs.
6. Support path prefix matching.
7. Support regex route matching.
8. Support path stripping.
9. Support regex-based path rewrites.
10. Retry transient failures.
11. Respect Retry-After headers.
12. Persist all queued requests in SQLite.
13. Recover queued requests after restart.
14. Support concurrent worker processing.
15. Expose health endpoint.
16. Expose job status endpoint.
17. Operate entirely from YAML configuration.

---

Non-Goals

The system will NOT include:

- Synchronous reverse proxying
- Waiting for upstream response before replying
- Web UI
- Authentication
- Authorization
- TLS certificate management
- Rate limiting
- Prometheus
- OpenTelemetry
- Distributed clustering
- Multi-node operation
- Kafka
- RabbitMQ
- Redis
- Hot configuration reload
- Request batching

---

High-Level Architecture

                 +----------------+
                 |  Application   |
                 +--------+-------+
                          |
                          |
                          v
                 +----------------+
                 | Retry Proxy    |
                 | HTTP Server    |
                 +--------+-------+
                          |
                          |
                          v
                 +----------------+
                 | SQLite Queue   |
                 +--------+-------+
                          |
                 +--------+--------+
                 |                 |
                 v                 v
            Worker 1         Worker N
                 |                 |
                 +--------+--------+
                          |
                          v
                 +----------------+
                 | Upstream APIs  |
                 +----------------+

---

Technical Constraints

Language

Go 1.24+

Dependencies

Only the following external dependencies are allowed:

gopkg.in/yaml.v3
modernc.org/sqlite

Everything else must use the Go standard library.

Forbidden Dependencies

chi
gorilla/mux
gin
echo
fiber
cobra
viper
zap
logrus
prometheus
otel

---

HTTP API

Request Ingestion

All non-system paths are treated as proxy requests.

Example:

POST /crm/customers/123

If accepted:

HTTP/1.1 202 Accepted
Content-Type: application/json

{
  "accepted": true,
  "job_id": 123
}

The request is considered accepted only after successful persistence to SQLite.

If persistence fails:

HTTP/1.1 500 Internal Server Error

---

Internal Endpoints

Health

Request:

GET /_proxy/health

Response:

{
  "status": "ok"
}

Checks:

- SQLite connectivity
- Configuration loaded

---

Job Status

Request:

GET /_proxy/jobs/{id}

Response:

{
  "id": 123,
  "state": "completed",
  "retry_count": 2,
  "response_code": 200,
  "created_at": "2026-01-01T12:00:00Z",
  "completed_at": "2026-01-01T12:01:30Z"
}

404 if job not found.

---

Configuration

YAML

Example:

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

---

Route Matching

Route Resolution

Routes are evaluated in declaration order.

First matching route wins.

If no route matches:

404 Not Found

---

Prefix Match

Example:

match:
  prefix: /crm

Matches:

/crm
/crm/
/crm/users
/crm/users/123

---

Regex Match

Example:

match:
  regex: "^/legacy/(.*)$"

Uses Go regexp package.

---

Rewrite Processing

Rewrite execution order is fixed.

Step 1

Match route.

Step 2

Apply strip_prefix if enabled.

Example:

Input:

/crm/users/123

Output:

/users/123

---

Step 3

Apply regex rewrite rules sequentially.

Example:

Rule:

pattern: "^/customer/(.*)$"
replacement: "/api/v2/customers/$1"

Input:

/customer/123

Output:

/api/v2/customers/123

---

Step 4

Append rewritten path to target base URL.

Example:

Base URL:

https://api.crm.com

Path:

/api/v2/customers/123

Query:

active=true

Final URL:

https://api.crm.com/api/v2/customers/123?active=true

---

Request Preservation

The following must be preserved:

HTTP method
Path
Query string
Request body
Headers

The following must NOT be forwarded:

Host
Content-Length
Connection
Transfer-Encoding

---

Persistence

Database

SQLite using:

modernc.org/sqlite

Initialization:

PRAGMA journal_mode=WAL;
PRAGMA synchronous=NORMAL;
PRAGMA busy_timeout=5000;
PRAGMA foreign_keys=ON;

---

Database Schema

CREATE TABLE jobs (

    id INTEGER PRIMARY KEY AUTOINCREMENT,

    route_name TEXT NOT NULL,

    method TEXT NOT NULL,

    request_path TEXT NOT NULL,

    query_string TEXT,

    headers_json TEXT NOT NULL,

    body BLOB,

    state TEXT NOT NULL,

    retry_count INTEGER NOT NULL DEFAULT 0,

    next_retry_at DATETIME NOT NULL,

    deadline_at DATETIME NOT NULL,

    response_code INTEGER,

    response_headers_json TEXT,

    response_body BLOB,

    last_error TEXT,

    created_at DATETIME NOT NULL,

    updated_at DATETIME NOT NULL,

    completed_at DATETIME
);

CREATE INDEX idx_jobs_state_retry
ON jobs(state, next_retry_at);

---

Job States

Allowed states:

queued
processing
completed
failed
expired

Definitions:

queued:
Ready for execution.

processing:
Claimed by a worker.

completed:
Successful upstream response.

failed:
Permanent failure.

expired:
Exceeded retry deadline.

---

Retry Logic

Retryable HTTP Status Codes

Configured via YAML.

Default:

429
500
502
503
504

---

Retryable Network Errors

Retry:

connection refused
connection reset
timeout
DNS lookup failure
TLS handshake failure
temporary network failure

---

Retry-After Header

Supported formats:

Retry-After: 120

and

Retry-After: Wed, 21 Oct 2026 07:28:00 GMT

If present, Retry-After overrides backoff calculation.

---

Backoff Strategy

Only strategy supported in v1:

exponential

Example:

5s
10s
20s
40s
60s
60s
60s

Maximum delay configurable.

---

Expiration

Jobs expire after:

retry:
  max_duration: 10m

Expired jobs transition to:

expired

No further retries occur.

---

Worker Model

Worker Startup

At startup:

for i := 0; i < cfg.Worker.Concurrency; i++ {
    go worker(...)
}

Workers are goroutines.

No external queue system.

---

Worker Loop

1. Find eligible queued job.
2. Atomically claim job.
3. Execute request.
4. Mark completed OR
5. Schedule retry OR
6. Mark failed.
7. Repeat.

---

Claiming

Workers must not process the same job twice.

Atomic state transition required.

Example:

UPDATE jobs
SET state='processing'
WHERE id=?
AND state='queued';

Worker continues only if exactly one row updated.

---

Startup Recovery

On startup:

UPDATE jobs
SET state='queued'
WHERE state='processing';

Reason:

Previous process may have crashed while processing.

All in-flight jobs return to queue.

---

Logging

Use:

log/slog

JSON output.

Example:

{
  "level":"INFO",
  "job_id":123,
  "route":"crm",
  "attempt":4,
  "status":429
}

---

HTTP Client

Use:

net/http

Single shared client.

Configuration:

http:
  timeout: 30s

Applied to upstream requests.

---

Build Requirements

Build command:

CGO_ENABLED=0 go build \
  -trimpath \
  -ldflags="-s -w"

Output:

retry-proxy

Single executable.

No external runtime dependencies.

---

Project Structure

cmd/
└── retry-proxy/
    └── main.go

internal/

├── config/
│   └── config.go

├── database/
│   └── sqlite.go

├── jobs/
│   ├── model.go
│   ├── repository.go
│   └── worker.go

├── routing/
│   ├── matcher.go
│   └── rewrite.go

├── upstream/
│   └── client.go

└── api/
    └── handlers.go

---

Acceptance Criteria

1. Requests are persisted before acknowledgment.
2. HTTP response is always 202 for accepted requests.
3. Requests survive process restart.
4. Requests survive machine reboot.
5. Prefix matching works.
6. Regex matching works.
7. Prefix stripping works.
8. Regex rewriting works.
9. Query strings are preserved.
10. Retry-After is honored.
11. Retryable status codes are configurable.
12. Concurrent workers process jobs correctly.
13. Duplicate processing does not occur during normal operation.
14. Startup recovery returns processing jobs to queue.
15. Binary builds with CGO disabled.
16. Only YAML and SQLite external libraries are used.
17. Service runs as a single self-contained executable.One implementation detail I would add for the coding agent: store request bodies exactly as received (raw bytes) and store headers as JSON-encoded map[string][]string. Avoid any schema that attempts to normalize headers into separate tables; it adds complexity without providing value for this use case.
