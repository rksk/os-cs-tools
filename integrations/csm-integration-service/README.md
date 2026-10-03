# CSM Integration Service

Go backend service exposing Project search, Account search, their Contacts
sub-resource, and a subset of Case operations, for third-party (M2M) consumers.

## Quick Start

```bash
# from operations/csm-integration-service
go run ./cmd/server/main.go
```

The server automatically loads `.env` from the working directory on startup (silently
ignored if absent).

Server starts at `http://localhost:8080`.

## Overview

- Default port: `8080`
- Runtime: Go `1.26+`
- Entry point: `cmd/server/main.go`
- Authentication and authorization:
  - Incoming requests: **authentication is the gateway's; authorization is per
    operation here.** This service is fronted by Choreo's API Manager gateway
    (subscription + M2M client-credentials app auth) — the app code performs no
    Bearer/JWT signature validation of its own. Unlike `apps/csm-portal/backend`
    (which authenticates its own end users), this service has no end-user identity
    to check. It does, however, require one OAuth2 scope per operation (for
    example `cases:write`, `contacts:read`, `vulnerabilities:sync`; the full list
    is in `openapi.yaml`'s security scheme and `cmd/server/routes.go`), read from
    the `scope` claim of the token the gateway forwards: no usable token is a
    401, a token without the operation's scope is a 403, and neither is forwarded
    upstream. `GET /health` is the only unscoped route. Set
    `REQUIRE_OPERATION_SCOPES=false` to switch the check off for local
    development only.
  - Outbound service calls: OAuth2 client credentials grant to the entity service
    (managed automatically) — always M2M, on every request, with no mechanism to
    carry an end-user identity. entity-service's ServiceNow-backed operations
    require a forwarded end-user identity token and will always reject a request
    from this service with 401 — this service can only ever serve entity-service
    data that doesn't require one (Postgres-backed operations). `PATCH
    /projects/{id}` and `POST /cases/{id}/comments` are known, deliberate
    exceptions: they're kept for API-shape completeness, but currently always
    401. `PATCH /cases/{id}` is a partial exception — a state/severity/
    workState update succeeds when entity-service runs on a Postgres data
    source; every other field that endpoint accepts is a 400 there instead
    (rejected as ServiceNow-only), and on a ServiceNow data source every
    field, including state/severity/workState, 401s the same as
    `UpdateProject`. See `CLAUDE.md` before adding any other endpoint that targets a
    ServiceNow-backed operation.

## Prerequisites

- Go `1.26+` — [install](https://go.dev/doc/install)

## Testing

```bash
go test ./...
go test -race ./...
go test -coverprofile=coverage.out ./... && go tool cover -html=coverage.out
```

Or use `make`:

```bash
make test    # vet + test
make build   # vet + test + compile
```

Handler tests use a mock entity client (`internal/handler/helpers_test.go`) and a
shared `upstreamErrors` table covering every `mapUpstreamError` status-code mapping.
Entity client tests spin up real `httptest.Server`s to exercise the OAuth2
client-credentials flow, error-body truncation, and correlation ID forwarding.
Middleware tests cover header injection, ID generation/preservation, and the
scope guard's 401/403/pass decisions. `cmd/server/routes_test.go` drives every
route in the scope table through a real mux against a stub upstream, asserting
401 without a token, 403 with every other scope, and 2xx with the required one.
`internal/apierror` (a two-line `Error()` method) has no dedicated tests.

### Run tests before every push (recommended)

```bash
git config core.hooksPath .githooks
# or, from this directory:
make setup
```

## Security Scanning

```bash
go install github.com/securego/gosec/v2/cmd/gosec@latest
gosec -fmt=text ./...
```

The scan should report **0 issues**. If a new finding appears, fix the root cause
before merging — do not suppress it without a code review.

## Configuration

Copy `.env.example` to `.env` and fill in the values:

### Entity service

| Variable | Description |
|---|---|
| `ENTITY_BASE_URL` | Base URL of the entity service |
| `ENTITY_TOKEN_URL` | OAuth2 token endpoint |
| `ENTITY_CLIENT_ID` | OAuth2 client ID |
| `ENTITY_CLIENT_SECRET` | OAuth2 client secret |
| `ENTITY_SCOPES` | Comma-separated OAuth2 scopes (optional) |

### Server

| Variable | Description |
|---|---|
| `PORT` | Server listen port (default `8080`) |
| `REQUIRE_OPERATION_SCOPES` | Enforce the per-operation scope check on every route except `GET /health` (default `true`). Set to `false` only for local development against a client that forwards no token; the server logs a warning at startup when it is off |

## Project Structure

```text
csm-integration-service/
├── cmd/server/main.go          # Entry point — routes + server startup
├── internal/
│   ├── apierror/                 # Typed upstream error type (4xx/5xx passthrough)
│   ├── entity/
│   │   ├── client.go             # OAuth2 HTTP client for the entity service
│   │   └── entity.go             # Entity service operations (accounts, projects, contacts, cases, opportunities, invoices, project-opportunity links)
│   ├── middleware/
│   │   ├── correlation.go        # X-CSM-Correlation-ID propagation + slog enrichment
│   │   ├── logger.go             # Per-request access log
│   │   └── security_headers.go   # X-Content-Type-Options, CSP, HSTS on every response
│   └── handler/
│       ├── response.go           # Shared writeError/writeJSON/mapUpstreamError + ErrMsg*
│       ├── accounts.go           # HTTP handlers for account endpoints
│       ├── projects.go           # HTTP handlers for project endpoints
│       ├── cases.go              # HTTP handlers for case endpoints
│       ├── vulnerabilities.go    # HTTP handler for the product-vulnerability sync endpoint
│       ├── opportunities.go      # HTTP handlers for opportunity endpoints
│       ├── invoices.go           # HTTP handlers for invoice endpoints
│       └── project_opportunity_links.go  # HTTP handler for project-opportunity link search
├── .choreo/component.yaml
├── openapi.yaml
└── .env.example
```

## API Endpoints

- `GET /health` — health check, no auth
- `GET /accounts/{id}` — get an account by ID
- `POST /accounts/search` — search accounts
- `POST /accounts/{id}/contacts/search` — search an account's contacts
- `GET /projects/{id}` — get a project by ID
- `POST /projects/search` — search projects
- `POST /projects/{id}/contacts/search` — search a project's contacts
- `PATCH /projects/{id}` — update project closure-state fields (ACP automation; currently always 401s, see Overview above)
- `PATCH /cases/{id}` — update a case's state, severity, or workState (succeeds on a Postgres data source; other fields 400 there, and every field 401s on a ServiceNow data source — see Overview above)
- `POST /cases/{id}/comments` — add a comment to a case (currently always 401s, see Overview above)
- `POST /incidents` — create an incident (ServiceNow data source; a 401 is possible if the target environment's M2M ServiceNow credential isn't configured, but this is not unconditional — see `CLAUDE.md`)
- `PATCH /incidents/{id}` — update an incident, e.g. push a work note. Unconditionally ServiceNow-backed with no Postgres fallback (unlike `PATCH /cases/{id}`): 503 on a Postgres data source, same conditional-401 behavior as `POST /incidents` on a ServiceNow one — see `CLAUDE.md`
- `POST /incidents/search` — search incidents (same conditional-401 behavior as `POST /incidents`, see `CLAUDE.md`)
- `POST /services/search` — search CMDB IT services (same conditional-401 behavior as `POST /incidents`, see `CLAUDE.md`)
- `POST /vulnerabilities/sync` — full-replace sync of product-vulnerability records (submit the complete current set on every call, not a delta)
- `GET /opportunities/{id}` — get an opportunity by ID
- `POST /opportunities/search` — search opportunities
- `GET /invoices/{id}` — get an invoice by ID
- `POST /invoices/search` — search invoices
- `POST /project-opportunity-links/search` — search project-opportunity links (no by-id fetch — the underlying data has no single-record endpoint)

All responses are raw JSON passthrough from the entity service — this service does not
reshape upstream response bodies.

## Run Locally

```bash
curl -X POST http://localhost:8080/accounts/search -d '{}'
curl http://localhost:8080/accounts/<id>
curl -X POST http://localhost:8080/accounts/<id>/contacts/search -d '{}'
curl -X POST http://localhost:8080/projects/search -d '{}'
curl http://localhost:8080/projects/<id>
curl -X POST http://localhost:8080/projects/<id>/contacts/search -d '{}'
curl -X POST http://localhost:8080/vulnerabilities/sync -d '[]'
curl -X PATCH http://localhost:8080/cases/<id> -d '{"state":"closed"}'
curl -X POST http://localhost:8080/cases/<id>/comments -d '{"type":"comment","content":"hi"}'
curl -X POST http://localhost:8080/opportunities/search -d '{}'
curl http://localhost:8080/opportunities/<id>
curl -X POST http://localhost:8080/invoices/search -d '{}'
curl http://localhost:8080/invoices/<id>
curl -X POST http://localhost:8080/project-opportunity-links/search -d '{}'
```
