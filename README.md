# Go API Starter

## Optional rate limiting

Rate limiting is disabled by default to preserve current behavior. When enabled, the API applies an in-memory token bucket keyed by the source IP from `RemoteAddr`; each successful request consumes one token. By default, each IP receives 10 requests per second with a burst of 20. Configuration applies to one process, resets on restart, and is not synchronized across instances.

| Variable | Default | Rule when enabled |
| --- | --- | --- |
| `RATE_LIMIT_ENABLED` | `false` | boolean |
| `RATE_LIMIT_REQUESTS_PER_SECOND` | `10` | positive refill rate, in requests per second |
| `RATE_LIMIT_BURST` | `20` | positive integer token bucket capacity |
| `RATE_LIMIT_MAX_CLIENTS` | `10000` | positive integer maximum number of tracked IPs |
| `RATE_LIMIT_ENTRY_TTL` | `10m` | positive duration after which an inactive bucket is removed |

When the table is full, requests from IPs that already have entries continue normally; new IPs receive 429 until expired entries are cleaned up. Cleanup happens when a request arrives; no background goroutine is created. IP keys are normalized from `RemoteAddr` with the port removed; `X-Forwarded-For` and `X-Real-IP` are not trusted. Behind a reverse proxy, multiple users may share a bucket keyed by the proxy IP. Enable this in a real deployment only after determining how the proxy will be handled; trusted proxies and distributed limits are outside the current scope. This is not an account quota or a complete DDoS mitigation.

Exact `GET`/`HEAD` requests to `/health` and `/ready` are exempt when those routes exist; the limiter does not create a readiness route. CORS preflight is handled before the limiter, and 429 responses to allowed origins still include CORS headers. When the limit is exceeded, the API returns JSON `rate_limited` with `Retry-After`; HEAD has no body. To enable it locally:

```powershell
$env:RATE_LIMIT_ENABLED = 'true'
go run ./cmd/api
```

With Compose, set `RATE_LIMIT_ENABLED=true` in `.env`, then recreate the API service. To mitigate an issue, set `RATE_LIMIT_ENABLED=false` and restart or recreate the service. Check safely with a local burst and a call to `/health`; do not run load tests against a live service.

## Optional CORS

CORS is disabled by default and preserves routing behavior (including `OPTIONS /health` returning 405). Configuration uses environment overrides for the JSON profile; an empty environment value uses the profile default. Lists are comma-separated, with whitespace trimmed around each item.

| Variable | Default | Rule when enabled |
| --- | --- | --- |
| `CORS_ENABLED` | `false` | boolean |
| `CORS_ALLOWED_ORIGINS` | empty | required, specific HTTP/HTTPS origins, e.g. `http://localhost:3000,https://example.com` |
| `CORS_ALLOWED_METHODS` | `GET,HEAD` | comma-separated HTTP methods; case-sensitive |
| `CORS_ALLOWED_HEADERS` | empty | request headers allowed in preflight; compared case-insensitively |
| `CORS_EXPOSED_HEADERS` | empty | response headers additionally exposed to JavaScript |
| `CORS_ALLOW_CREDENTIALS` | `false` | enable deliberately for trusted origins |
| `CORS_MAX_AGE` | `10m` | non-negative whole-second duration; `0s` disables preflight caching |

An origin must not include a path (including a trailing `/`), query, fragment, or userinfo. Wildcards, regular expressions, and `null` are not supported; if present, the port must be from 1 through 65535. Method and header lists do not support wildcards. When enabled, invalid configuration causes startup to fail and identifies the invalid variable. When disabled, the other CORS settings are ignored. A request origin must exactly match the allowlist.

Local PowerShell example:

```powershell
$env:CORS_ENABLED = 'true'
$env:CORS_ALLOWED_ORIGINS = 'http://localhost:3000'
$env:CORS_ALLOWED_HEADERS = 'Authorization,Content-Type'
go run ./cmd/api
```

To use cookies/credentials, deliberately set `CORS_ALLOW_CREDENTIALS=true` and use `fetch(url, { credentials: 'include' })` in the browser; cookie policies still apply. CORS does not replace authentication, authorization, or CSRF protection, and does not block non-browser clients. An actual request from a denied origin still proceeds over HTTP, but does not receive CORS permission headers. Responses to errors from allowed origins also include CORS headers.

A preflight is an OPTIONS request containing both `Origin` and `Access-Control-Request-Method`: a valid request returns 204 with no body; a denied origin, method, or header returns 403 without CORS permission headers. Other OPTIONS requests continue through routing. CORS middleware wraps the limiter and recovery middleware so preflight is handled first and CORS headers remain on error responses. `Vary` distinguishes origin and preflight attributes while preserving existing `Vary` values.

HTTP smoke checks after enabling, from another terminal:

```powershell
curl.exe -i http://localhost:8080/health -H "Origin: http://localhost:3000"
curl.exe -i -X OPTIONS http://localhost:8080/health -H "Origin: http://localhost:3000" -H "Access-Control-Request-Method: GET" -H "Access-Control-Request-Headers: Authorization"
```

Expect 200 with the health body and the specific allowed origin; preflight should return 204 with no body. After setting `CORS_ENABLED=false` and restarting, repeat the checks: GET should still return 200, OPTIONS should return 405, and no CORS permission headers should be present. With Compose, edit `.env` and recreate the service to apply the new environment. To mitigate an issue, disable CORS or restore the previous configuration, then check `/health` again. `go test ./cmd/api -run TestCORSSmoke -v` checks actual and preflight requests over a socket with CORS enabled and disabled; it does not replace testing cross-origin behavior in a real browser.

This Go project skeleton follows `architect.txt` and was created directly in the `server` directory.

```text
server/
├── cmd/
│   ├── api/main.go
│   └── worker/main.go
├── internal/
│   ├── order/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── order.go
│   ├── product/
│   │   ├── handler.go
│   │   ├── service.go
│   │   ├── repository.go
│   │   └── product.go
│   └── platform/
│       ├── database/
│       ├── logger/
│       └── config/
│           ├── config.go
│           ├── config_test.go
│           └── profiles/
│               ├── dev.json
│               ├── test.json
│               └── prod.json
├── pkg/
├── Dockerfile
├── compose.yaml
├── .dockerignore
├── .env.example
└── go.mod
```

## Component responsibilities

- `cmd/api`: API server entry point; the chi router registers HTTP routes.
- `cmd/worker`: future entry point for background tasks.
- `internal/order`, `internal/product`: example module names retained from `architect.txt`.
- `handler.go`: receives requests and returns responses.
- `service.go`: handles business logic.
- `repository.go`: accesses data.
- `order.go`, `product.go`: define each module's model.
- `internal/platform`: internal configuration, database connections, and logging.
- `pkg`: code intended for reuse and import by other projects.

The API server provides a `GET /health` health endpoint, returning JSON `{"status":"ok"}` with HTTP 200. The worker and business models have not been implemented. PostgreSQL is an optional dependency. Directories without code use `.gitkeep` so they can be tracked by Git.

## Run the API server

```sh
go run ./cmd/api
```

The server listens on `:8080` by default. Set the `HTTP_ADDR` environment variable to change the address, for example in PowerShell:

```powershell
$env:HTTP_ADDR = "127.0.0.1:8081"
go run ./cmd/api
```

Check it with `curl http://localhost:8080/health` (use the configured port if it has changed). The main router and `/health` and `/ready` routes are registered in `cmd/api/handler.go`. Middleware runs in this order: CORS → rate limit → recovery → router. Unknown routes return HTTP 404; unsupported methods at `/health` or `/ready` return HTTP 405. GET and HEAD are registered explicitly.

The router uses chi's default behavior and does not add path-normalization middleware: `/%68ealth`, `//health`, and `/health/` return JSON 404. This is an intentional change from ServeMux: `/%68ealth` previously returned 200 and `//health` redirected to `/health`. To roll back the router, restore the route registration and chi dependency, then rerun the HTTP smoke tests; no data migration or configuration needs restoring.

Press Ctrl+C to stop the server; in-flight requests have up to 10 seconds to finish. After deployment, check `/health`; if it fails, stop the new release and run the previous binary with its previous configuration.

## Dev / test / prod configuration

`internal/platform/config` loads a profile based on `APP_ENV` (`dev`, `test`, `prod`), defaulting to `dev`. Files in `internal/platform/config/profiles/*.json` are embedded in the binary and do not depend on the working directory. Rebuild the binary after editing a profile. Non-empty environment variables override profile values; empty variables use profile values. `.env` is not loaded automatically. `go test` does not automatically set `APP_ENV=test`.

| Variable | dev / prod | test |
| --- | --- | --- |
| `HTTP_ADDR` | `:8080` | `127.0.0.1:0` |
| `HTTP_READ_HEADER_TIMEOUT` | `5s` | `5s` |
| `HTTP_READ_TIMEOUT` | `15s` | `15s` |
| `HTTP_WRITE_TIMEOUT` | `15s` | `15s` |
| `HTTP_IDLE_TIMEOUT` | `60s` | `60s` |
| `HTTP_SHUTDOWN_TIMEOUT` | `10s` | `10s` |

The test profile listens only on loopback and lets the operating system select an available port; the startup log shows the actual port. Dev/prod retain the current HTTP defaults. Only implemented components are configured; there is no DB or worker configuration yet.

PowerShell example (change `prod` to `dev` or `test` as needed):

```powershell
$env:APP_ENV = "prod"
$env:HTTP_ADDR = "127.0.0.1:8081"
$env:HTTP_SHUTDOWN_TIMEOUT = "20s"
go run ./cmd/api
```

An invalid `APP_ENV`, malformed `host:port` address, port outside 0–65535, or non-positive timeout makes the API exit before opening a listener. Timeouts use Go duration syntax, for example `500ms`, `5s`, or `1m`. Do not store secrets in profiles because they are committed and embedded in the binary; provide secrets through the environment or a secret management system when a component requires them.

After deployment, call `GET /health` at the configured address/port and check for HTTP 200 with `{"status":"ok"}`. If it fails, stop the new release and restore the previous binary and environment variables. In PowerShell, remove an override with `Remove-Item Env:HTTP_ADDR` (and similarly for other variables).

## Outbound HTTP client

`internal/platform/httpclient.New(configuration.HTTPClient)` returns an `*http.Client` with its own transport, configured using standard `net/http` settings. It does not modify or share `http.DefaultClient` or `http.DefaultTransport`. No dependency or logging is added. No module currently makes outbound calls, so startup and health checks do not create a client or make sample network requests. When a real caller is added, initialize the client once at the entry point and pass the same pointer to the module constructor; the client and pool can be used concurrently by multiple goroutines. Do not modify the client or transport after use begins.

The dev/test/prod profiles use the same defaults below. Non-empty environment values override the profile; `.env.example` and Compose support all of these variables.

| Variable | Default | Meaning |
| --- | --- | --- |
| `HTTP_CLIENT_TIMEOUT` | `30s` | Entire request, including connection, redirects, and body reading |
| `HTTP_CLIENT_CONNECT_TIMEOUT` | `5s` | Establishing a TCP connection, including name resolution |
| `HTTP_CLIENT_TLS_HANDSHAKE_TIMEOUT` | `5s` | TLS handshake |
| `HTTP_CLIENT_RESPONSE_HEADER_TIMEOUT` | `10s` | Waiting for headers after sending the request; excludes body reading |
| `HTTP_CLIENT_IDLE_CONN_TIMEOUT` | `90s` | How long an idle connection remains in the pool |
| `HTTP_CLIENT_MAX_IDLE_CONNS` | `100` | Maximum total idle connections |
| `HTTP_CLIENT_MAX_IDLE_CONNS_PER_HOST` | `10` | Maximum idle connections per host |
| `HTTP_CLIENT_MAX_CONNS_PER_HOST` | `50` | Maximum connections per host, including connecting, active, and idle connections |

Durations and connection counts must be positive; the per-host idle limit must not exceed either the total idle limit or the per-host total connection limit. Both the config loader and constructor validate values before use. The overall timeout does not need to equal the sum of component timeouts: the request is canceled by whichever comes first—the context deadline, overall timeout, or current phase timeout. Waiting for a pool slot and reading the body are also subject to the overall timeout/context; the idle timeout only manages resources between requests. TCP keep-alive is `30s`, the `100-continue` wait is `1s`, and HTTP/2 negotiation is enabled as in the standard transport.

TLS retains the default certificate/hostname verification and does not attach credentials or a cookie jar. Proxy settings use `http.ProxyFromEnvironment` (`HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`). Redirects follow the default `http.Client` behavior and stop after 10 consecutive requests. There is no retry/backoff loop and no retry based on status. The standard transport may still automatically resend idempotent requests in some errors involving reused connections; this is `net/http` behavior. HTTP 4xx/5xx responses are returned unchanged for the caller to handle.

Example lifecycle at the entry point (imports `server/internal/platform/config` and `server/internal/platform/httpclient`); pass `outboundClient` to the module that needs it and keep it alive until the modules have stopped:

```go
configuration, err := config.Load()
if err != nil {
    return err
}
outboundClient, err := httpclient.New(configuration.HTTPClient)
if err != nil {
    return err
}
defer outboundClient.CloseIdleConnections()
```

Example caller function using an injected client (imports `context`, `fmt`, `io`, `net/http`, `time`); `endpoint` must be controlled by the application or its destination validated at the trust boundary if it comes from a user:

```go
func fetch(ctx context.Context, client *http.Client, endpoint string) ([]byte, error) {
    requestContext, cancel := context.WithTimeout(ctx, 5*time.Second)
    defer cancel()
    request, err := http.NewRequestWithContext(requestContext, http.MethodGet, endpoint, nil)
    if err != nil {
        return nil, err
    }
    response, err := client.Do(request)
    if err != nil {
        return nil, err
    }
    defer response.Body.Close()
    if response.StatusCode != http.StatusOK {
        return nil, fmt.Errorf("unexpected HTTP status: %d", response.StatusCode)
    }
    const maxBodyBytes = 1 << 20
    body, err := io.ReadAll(io.LimitReader(response.Body, maxBodyBytes+1))
    if err != nil {
        return nil, err
    }
    if len(body) > maxBodyBytes {
        return nil, fmt.Errorf("response body exceeds limit")
    }
    return body, nil
}
```

Callers must always close the body and choose a read limit appropriate to the use case. Reading to EOF and then closing allows connection reuse; closing early when the status/body is unsuitable may discard the connection. Do not drain an unbounded body. `CloseIdleConnections` releases idle connections but does not cancel in-flight requests; stop callers or cancel their contexts before ending the client lifecycle. Do not log raw `net/http` errors because they may contain URLs; do not log tokens, headers, query strings, bodies, or URLs containing credentials.

Local checks that do not call the Internet: `go test ./internal/platform/config ./internal/platform/httpclient`, `go test ./...`, `go vet ./...`, `go build ./...`. After deployment, check that `/health` returns HTTP 200 and `{"status":"ok"}`. If there is a failure, restore the previous binary/image and environment; remove `HTTP_CLIENT_*` overrides to return to profile defaults.

## Error handling

The API returns consistent JSON errors, for example:

```json
{"error":{"code":"not_found","message":"Resource not found"}}
```

- Unknown route: HTTP 404, code `not_found`.
- Unsupported method at `/health`: HTTP 405, code `method_not_allowed`, header `Allow: GET, HEAD`.
- Panic in a handler before a response is sent: HTTP 500, code `internal_error`.
- `HEAD` returns the corresponding status/headers with no body.

`cmd/api/handler.go` manages responses and recovery with `net/http`. Panics are logged through `slog`; panic values, URLs, headers, and request bodies are not logged to avoid exposing data. If a response has already started, recovery aborts the request with `http.ErrAbortHandler`; it does not append a JSON error to a partial response. An intentional panic with `http.ErrAbortHandler` is preserved. Recovery applies only to the goroutine handling the HTTP request. Response write errors are logged; the handler does not retry writing when the connection may already be closed. Listen/serve/shutdown errors are wrapped with `%w` to preserve the cause for `errors.Is/As`. Business modules and the worker are currently empty and have no business error handling.

Check with `go test ./...` and `go vet ./...`. After deployment, check that `/health` returns 200 and `{"status":"ok"}`, an unknown route returns JSON 404, and `POST /health` returns JSON 405. If a check fails, restore the previous binary/image and configuration, then check again.

## Logging

`internal/platform/logger` uses `log/slog` and writes to stdout, not to files. The `dev` and `test` profiles use `TextHandler` at DEBUG level; `prod` uses `JSONHandler` at INFO level. Each production record is a single line of JSON for Docker collection and forwarding to Loki; the project does not configure Loki.

Set `LOG_LEVEL` to `DEBUG`, `INFO`, `WARN`, or `ERROR` to override the level. Names are case-insensitive; slog offsets such as `INFO+2` are supported. An empty value uses the profile; an invalid value makes the API exit before opening a listener. Compose passes `LOG_LEVEL` from the terminal or `.env` into the container.

Initialize the logger at the entry point and pass `*slog.Logger` to modules that need it:

```go
applicationLogger := logger.New(configuration.Environment, configuration.LogLevel)
orderLogger := applicationLogger.With("module", "order")
orderLogger.Info("Order created", "order_id", orderID)
```

Import the package as `server/internal/platform/logger`. `With` creates a child logger that retains its parent's fields without changing the parent. The API uses fields `service`, `env`, and `addr`; internal HTTP errors also go through the same handler at ERROR level. Configuration errors use the startup logger at INFO level, formatted according to `APP_ENV`.

Pass only messages and fields known to be safe. Do not log passwords, tokens, cookies, authorization headers, secrets, entire requests/headers/configuration, or uncontrolled user data. Check error contents before logging them. The logger does not automatically redact sensitive data in messages, fields, or objects; each calling module is responsible for choosing what may be logged.

After deployment, check that `/health` returns HTTP 200 and that the startup log appears on stdout (JSON when `APP_ENV=prod`), for example with `docker compose logs api`. If there is a failure, restore the previous image and environment, then recreate the container as described in the rollback guidance below.

## Run with Docker

Requires Docker with Linux containers and Docker Compose. The Dockerfile builds Go in a separate stage, then packages the binary and CA certificates in a `scratch` image running as UID/GID `65532:65532`. The image contains no shell or `.env` file.

Create local configuration from the sample (only if `.env` does not already exist):

```powershell
Copy-Item .env.example .env
docker compose up --build -d
curl.exe --fail http://localhost:8080/health
docker compose logs api
docker compose down
```

Set `APP_ENV` in `.env` to `dev`, `test`, or `prod`. Compose reads this file and passes the variables declared under `environment` into the container; the application reads them with `os.Getenv`. Terminal variables take precedence over values in `.env`. Empty timeout values use the JSON profile. `HTTP_PORT` only changes the host port; Compose fixes `HTTP_ADDR=:8080` inside the container, including for the test profile, so port mapping works. The host port binds to `127.0.0.1` for local use.

`STOP_GRACE_PERIOD` is how long Docker waits before forcibly stopping the container; set it higher than `HTTP_SHUTDOWN_TIMEOUT` (the application default is `10s`). After changing environment variables, run `docker compose up -d --no-build` to recreate the container with the new configuration; `docker compose restart` does not update environment variables.

You can create `.env.test` or `.env.prod` from the sample and use the same built image:

```powershell
docker compose --env-file .env.test up -d --no-build
```

Set `APP_ENV=test` in `.env.test`; the filename does not select the profile. Personal env files are excluded from Git and the Docker build context. `.env.example` contains sample values only, no secrets. When adding a secret, declare the corresponding variable in Compose and read/validate it in the application; do not put secrets in JSON, the Dockerfile, or build args. No current application component requires a secret.

For deployment, use the same image and provide environment variables through the deployment platform. For example, run a built image with an env file provided by the deployment system:

```sh
docker run -d --name go-api-starter-api --env-file .env.prod -e HTTP_ADDR=:8080 -p 127.0.0.1:8080:8080 --stop-timeout 30 go-api-starter:local
```

Compose currently serves local use; configure ingress/public ports on the deployment platform. The smoke test must return HTTP 200 and `{"status":"ok"}` from `/health`. For each release, tag the image with `IMAGE_TAG` and retain the previous image and environment. If the smoke test fails, restore the old tag/environment, run `docker compose up -d --no-build`, and check `/health` again.

See [how Docker Compose sets environment variables](https://docs.docker.com/compose/how-tos/environment-variables/set-environment-variables/).

## Configuration checks and build

Requires Go 1.27 or later.

```sh
go build ./...
go test ./...
```

The module is temporarily named `server`; update `go.mod` when the official repository path is known. Dependencies are pinned in `go.mod` and `go.sum`.

## GitHub Actions CI

See the PostgreSQL section at the end of this document for running integration tests separately.

The sample workflow is at `templates/.github/workflows/ci.yml`, so CI does not run automatically when the repository is pushed to GitHub. Files in `templates/` retain the paths they would have from the project root at their intended destination.

To enable CI for a project using GitHub Actions, copy the sample from the project root with PowerShell (if a workflow with the same name already exists, inspect and merge it first):

```powershell
New-Item -ItemType Directory -Force .github/workflows | Out-Null
Copy-Item templates/.github/workflows/ci.yml .github/workflows/ci.yml
```

Commit and push `.github/workflows/ci.yml` to enable it. Once enabled, the workflow runs on every push and when a pull request is opened, reopened, or updated; it is not restricted by branch name. If using another CI platform, use the local checks below to build an equivalent pipeline.

The `Go CI` workflow and `Go checks` job run on Ubuntu: they check `gofmt` (read-only, failing if formatting is needed), `go vet ./...`, `go test -race ./...`, and `go build ./...`. The Go version comes from `go.mod`. The job has a 15-minute timeout; a new run cancels an older run for the same PR or ref. A failing step fails the job and later steps do not run; CI does not fix or commit code.

The workflow grants only `contents: read`, stores no credentials after checkout, and uses neither secrets nor the `pull_request_target` event. Current unit tests run independently and do not require a DB or external service. When adding integration tests that need services, isolate them with the `integration` build tag and run them separately with `go test -tags=integration ./...` in a suitable environment; do not include them in the default unit tests.

Caching is disabled when `go.sum` does not exist. When adding a dependency, run `go mod tidy` locally and commit both `go.mod` and `go.sum`; caching is then enabled based on the `go.sum` checksum. CI uses `GOFLAGS=-mod=readonly` to report missing dependency/checksum information instead of updating modules automatically to pass the check.

Equivalent checks from the project root using Bash (Linux, WSL, or Git Bash):

```bash
set -euo pipefail
export CGO_ENABLED=1
export GOFLAGS=-mod=readonly
unformatted=$(gofmt -l .)
if [[ -n "$unformatted" ]]; then
  printf 'Go files need formatting:\n%s\n' "$unformatted"
  exit 1
fi
go vet ./...
go test -race ./...
go build ./...
```

Go as specified in `go.mod` and a C compiler compatible with that Go version are required for the race detector (the Ubuntu runner includes GCC). If Windows lacks a compiler, use Linux/WSL with Go and GCC; `go test ./...` is not a substitute for the race check.

After a workflow has run on GitHub, an administrator can select the `Go checks` check in branch protection/rulesets for the branch to protect. This task does not change repository settings. A successful local command does not mean the workflow succeeded on GitHub.

See the [setup-go](https://github.com/actions/setup-go) and [checkout](https://github.com/actions/checkout) configuration.

## PostgreSQL and migrations

The DB is disabled by default (`DB_ENABLED=false`); no credentials are needed and no connection is opened. When enabled, the API opens a pool, pings before listening, and closes the pool after HTTP shutdown; configuration or connection errors cause startup to fail. The API does not run migrations automatically. `GET /health` remains a liveness check. `GET /ready` returns `200 {"status":"ok"}` when the DB is disabled or the ping succeeds, and `503 {"status":"unavailable"}` when the ping fails. HEAD has no body; other methods return 405. Driver errors are not exposed in startup logs, the migration CLI, or readiness responses.

| Environment variable | Default | Constraint when DB is enabled |
| --- | --- | --- |
| `DB_ENABLED` | `false` | boolean |
| `DB_PROVIDER` | `postgres` | only postgres is supported |
| `DATABASE_URL` | empty | postgres/postgresql URL with host, user, and database name |
| `DB_MAX_OPEN_CONNS` | `10` | > 0 |
| `DB_MAX_IDLE_CONNS` | `2` | 0 through max open |
| `DB_CONN_MAX_LIFETIME` | `30m` | > 0 |
| `DB_CONN_MAX_IDLE_TIME` | `5m` | > 0 and no greater than lifetime |
| `DB_CONNECT_TIMEOUT` | `5s` | > 0; driver connection timeout |
| `DB_PING_TIMEOUT` | `2s` | > 0; overall ping limit, including waiting for a pool connection |

Credentials are read only from the environment. Do not put a real URL in source, profiles, command-line arguments, or logs; do not dump the full configuration. Local `.env` files must not be committed; Compose reads this file, while `go run` reads the process environment. If the URL omits `sslmode`, the module sets `verify-full`. In production, use `sslmode=verify-full&sslrootcert=/path/to/ca.crt`, ensure the hostname matches the certificate, and mount the CA into the container if needed. Use `sslmode=disable` only for isolated local use, never production. URL-encode special characters in usernames/passwords.

The module uses `database/sql`, [pgx](https://github.com/jackc/pgx) v5.11.0, and [golang-migrate](https://github.com/golang-migrate/migrate) v4.20.1, pinned in `go.mod`/`go.sum` and compatible with this repository's Go 1.27 toolchain. It selects migrate's pgx adapter to share the driver. Changing providers may require a driver, SQL/schema, and new migrations; changing `DB_PROVIDER` does not move data or convert dialects.

### Local with Compose

Copy `.env.example` to `.env` and provide `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`. Do not leave the password empty. These values initialize a new volume; changing the environment does not change credentials in an existing volume.

```sh
docker compose --profile database up -d --wait postgres
```

Set `DB_ENABLED=true` and `DATABASE_URL` in `.env` using the pattern `postgres://<user>:<password>@postgres:5432/<database>?sslmode=disable`. To run the API on the host, set the corresponding variables in the process environment and use host `127.0.0.1` with port `POSTGRES_PORT` (default 5432).

```sh
docker compose up -d --build api
curl http://localhost:8080/health
curl http://localhost:8080/ready
```

The postgres service runs only when its profile is enabled or it is targeted explicitly; the API has no required `depends_on`. The `postgres_data` volume preserves data. Do not run `docker compose down -v` when the data must be kept. To run without DB again, set `DB_ENABLED=false` and recreate the API service; PostgreSQL data remains intact.

### Standalone migration commands

Run from the repository root. `create` does not require a DB. DB commands use the same environment as the API and require `DB_ENABLED=true`. The default migrations directory is `migrations/postgres`; set `MIGRATIONS_DIR` to use another path. No business migrations exist yet.

```sh
go run ./cmd/migrate create add_example
go run ./cmd/migrate status
go run ./cmd/migrate up
go run ./cmd/migrate down 1
```

`create` generates an empty timestamped `.up.sql`/`.down.sql` pair; fill in and review both before running them. Each version must be unique; do not edit an applied migration. `down` requires a positive step count; there is no implicit rollback of everything. In a built container, for example: `docker compose run --rm --entrypoint /migrate api status`. Migration SQL is copied into the image; rebuild after adding SQL. Run `up` only as a controlled deployment step before shifting traffic, then check `/ready`.

Migrate stores the version/dirty state in `schema_migrations` and uses a PostgreSQL advisory lock so runners on the same DB/schema cannot run concurrently. After initialization, the runner has a 10-second lock timeout and a 1-minute migration statement timeout; metadata initialization may wait for another runner to release the lock. `status` may create the metadata table if it does not exist. Wrap multi-step SQL in `BEGIN`/`COMMIT` where appropriate; do not assume every DDL statement is transactional. See the [dirty migration guidance](https://github.com/golang-migrate/migrate/blob/master/GETTING_STARTED.md).

If a migration fails, stop the runner, check `status`, and compare the actual schema with the SQL. Driver details are hidden to avoid exposing secrets; inspect DB diagnostics with appropriate operational permissions. Restore a backup or repair the schema in a controlled manner. Only after confirming the schema matches the intended version should you run `go run ./cmd/migrate force VERSION` (use `-1` when there are no migrations). `force` changes metadata only; it runs no SQL and changes no data. Do not force, retry, or roll back before understanding the data state.

### Backup, restore, and rollback

Before a destructive migration, stop writes or plan for consistency, then create a backup with `pg_dump --format=custom --file=backup.dump`, using securely provided credentials through `PGHOST`, `PGPORT`, `PGUSER`, `PGDATABASE`, and `PGPASSFILE`; protect the backup as sensitive data. Test `pg_restore --dbname=<separate-restore-database> backup.dump` and inspect the data before allowing the migration. Do not test a restore over an existing DB. In production, prefer a verified snapshot/PITR and expand/contract migrations so old and new binaries can operate together during the transition.

If deployment fails, roll back the image only if the schema remains compatible. Rolling back the binary does not undo the schema; `down` may lose data and is not a substitute for a backup. If restoration is needed, stop writes, restore to a new DB, verify schema/data, switch the connection secret in a controlled manner, and smoke-test `/health` and `/ready` before opening traffic. This project does not deploy or run migrations against a live DB.

### Isolated integration tests

Unit tests: `go test ./...`; static/build checks: `go vet ./...`, `go build ./...`. Integration tests are opt-in with a build tag and `TEST_DATABASE_URL` pointing to a dedicated PostgreSQL instance with permission to create schemas. Tests create and delete only randomly named schemas created by the test itself and use test migrations; they do not roll back the application schema. Do not point this variable to production or a local DB containing data that must be kept.

```powershell
$env:TEST_DATABASE_URL = 'postgres://<user>:<password>@127.0.0.1:<test-port>/<test-db>?sslmode=disable'
go test -tags=integration ./... -count=1
```

The tests verify queries, migration up/status/down, HTTP health/readiness over a socket, readiness 503 when the pool cannot respond, timeout, and recovery. Unit tests also use an unresponsive TCP listener to verify that startup failure is time-bounded. If `TEST_DATABASE_URL` is unset, integration tests report skip; that does not prove PostgreSQL was tested.
