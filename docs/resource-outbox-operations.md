# Durable resource operations and reconciliation

RCP stores resource operations and their outbox events in the existing Ent database.
No Redis or message broker is required. API startup starts one worker and performs an
initial resource reconciliation; subsequent scans run approximately once per minute.

## API contract

The production handlers enqueue these requests and return **202 Accepted**:

- `POST /api/v1/compute/instances`
- `DELETE /api/v1/compute/instances/:id`
- `POST /api/v1/storage/containers`
- `DELETE /api/v1/storage/containers/:name?force=true`
- `POST /api/v1/block-storage/volumes`
- `DELETE /api/v1/block-storage/volumes/:id`

The response is `{"operation_id":"<uuid>","status":"PENDING"}` with a `Location`
header. Creation responses no longer contain the created resource. Clients must poll
`GET /api/v1/operations/:id`, then refresh their resource lists after `SUCCEEDED`.
The operation's `resource_id` becomes the provider ID when it is observed; while
pending, it may be a preallocated correlation identifier rather than a provider ID.

Send an `Idempotency-Key` (at most 128 characters) on retryable requests. Reusing a
key for the same owner and request returns the existing operation. Reusing it with
a different payload returns 409. Without a key each request is a new operation.
Keys are owner-scoped and request payloads are compared after handler normalization.
Deletion ownership is checked before enqueueing. Overlapping unfinished operations
for the same known resource return 409. Authorization remains in the existing handlers.

Operation statuses:

- `PENDING`: operation and execute event committed together.
- `RUNNING`: worker is executing the request.
- `VERIFYING`: provider effect is still in progress or a retry is needed.
- `UNKNOWN`: a VM/volume create was dispatched but its result cannot yet be identified.
- `SUCCEEDED`: actual provider state confirmed and local changes committed.
- `FAILED`: permanent failure or retry budget exhausted; inspect before taking action.

`GET /api/v1/operations/:id/events` exposes durable followup events to the owner.
Administrator-only endpoints:

- `GET /api/v1/admin/operations`
- `GET /api/v1/admin/reconciliation`
- `GET /api/v1/admin/notifications`

The admin endpoints accept `page` and `limit` (default 100, maximum 200). Notifications
are durable records for console polling; this implementation does not send email or
Slack messages. Provider error details stay in the DB and server logs, not public DTOs.

## Delivery and failure recovery

The worker polls every second, claims up to ten events per loop using conditional DB
updates, and fences completion with a random lease token. A claim expires after two
minutes. External requests have a 30-second context deadline. Resource DB changes,
operation completion and the `resource.changed` followup event commit in one transaction.
Instance metadata updates also atomically enqueue a `resource.recheck` event.

Failures retry with exponential delays up to 64 seconds, up to 20 delivery attempts.
Successful verification polls run every five seconds until the provider reaches its
terminal state. Inspect long-running `VERIFYING` operations; provider creation has
no fixed duration and is not treated as complete just because its API accepted a request.
Do not hold DB transactions open while calling OpenStack.

Nova/Cinder creates carry `rcp_operation_id` and `rcp_owner_id` metadata. Before creating,
the worker looks for an existing resource with these tags. Immediately before sending
a create request it durably records dispatch intent. After a crash or uncertain response,
it only discovers existing matches and **does not blindly repeat creation**. Multiple
matches require review. A crash between intent recording and request transmission can
therefore require manual review even though nothing was created. This deliberately
prefers avoiding duplicate resources over automatic recreation. Unknown failures retain
the resource lock; no automated force-retry or destructive cleanup endpoint is provided.

Swift creates use one preallocated UUID as the container name. PUT retries use the same
UUID and matching metadata is verified before local registration. Deletions retain local
ownership rows until an individual provider GET/HEAD confirms 404. A force-delete request
is pinned to that UUID, not to a display name that might later be reused. Cinder ownership
and attachment state are rechecked before deleting; attached/busy volumes fail safely.

Outbox delivery is at-least-once. A successful effect can run again if the DB commit fails;
provider identifiers, metadata discovery, idempotent deletes and unique local identifiers
make those retries recoverable. This is not a distributed transaction with OpenStack.
Keep the DB backup and completed operation history: request idempotency depends on it.

## Existing inconsistencies

Reconciliation reads all pages within the configured project/region. It does not claim
visibility into other projects. A failed component query marks that provider unknown;
previous resource observations retain their original `observed_at`. Consumers must check
both observation freshness and provider status, rather than display old values as current.

- Existing VM rows: refresh provider name, status, image and flavor while retaining user notes
  and ownership. Fixed IP remains a live lookup in the existing compute API.
- Missing VM/container: confirm with individual GET/HEAD, then record `missing_in_provider`.
  Retain the local row; never recreate or delete based solely on a missing list entry.
- Unregistered VM: import only when provider ownership metadata points to an existing RCP
  user and image/flavor references are available. Otherwise retain it for admin review.
- Unregistered Swift container: retain and report. Tagged creates are recovered by
  their durable operation metadata; legacy UUIDs do not establish a trustworthy owner or display name.
- Volumes: Cinder is already the inventory; this repository has no local volume table.
  Report volume error states and invalid/missing RCP owner metadata.
- Function data: check the metadata DB connection and registered SQLite files with read-only
  queries. Report missing, unreadable or unregistered UUID-named files. Never initialize a
  missing file, run migrations during inspection, or delete unregistered data.

Repeated identical observations refresh their timestamp but do not create another notification.
Observation changes, safe local repairs and their notification event commit together.
Late VM/volume/container completions can restore failed operations once an existing
resource is unambiguously confirmed, without issuing another create.
The existing admin instance responses now expose `status_source` and `status_verified`:
DB fallback values must be presented as unverified when the live lookup failed or omitted an ID.

## Deployment and verification

Ent schemas add three tables without dropping or changing existing resource columns.
The existing startup migration creates them. Back up the operating DB and the function data
before deployment, and deploy client support for the 202 contract together with the server.
The server alone cannot update the separate frontend repository.

Checks: `go test ./...`, `go test -race ./internal/domain/operations`, `go vet ./...`,
`go run entc.go`, `go generate ./cmd/api`, and `go mod tidy -diff`.
Tests exercise transaction rollback, restart recovery, leases, duplicate requests, ambiguous
VM creation, stable Swift UUIDs, existing mismatches and provider failures without destructive repair.
These tests use local SQLite and fake provider HTTP endpoints; production OpenStack and
PostgreSQL integration require deployment-environment validation.
