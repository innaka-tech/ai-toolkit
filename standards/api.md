# APIs

An API is a contract other people build on: design it first, version it, and never break it silently.

## Contract
- Described in a machine-readable spec (OpenAPI 3.x for HTTP, protobuf for gRPC, AsyncAPI for events) kept in the repository and checked in CI.
- Resource-oriented URLs with plural nouns (`/v1/shipments/{id}`), lowercase and hyphenated; verbs only for true actions (`POST /v1/shipments/{id}:cancel` or `/cancel`).
- Methods keep their HTTP meaning: GET is safe, PUT and DELETE are idempotent, POST creates or acts.
- JSON with consistent field casing (pick camelCase or snake_case once); timestamps in RFC 3339 UTC; money as integer minor units plus currency code; ids as opaque strings.

## Errors
- Correct status codes (400 validation, 401 unauthenticated, 403 forbidden, 404 not found, 409 conflict, 422 semantic, 429 rate limited, 5xx server).
- Body in RFC 9457 Problem Details: `type`, `title`, `status`, `detail`, plus field-level `errors`; a stable machine-readable error code; never stack traces.

## Behaviour
- Pagination for every list (cursor-based for large or changing sets) with a maximum page size.
- Filtering and sorting through documented query parameters only.
- Idempotency keys for POSTs that create money, orders, or messages; safe retries with exponential backoff and jitter for clients.
- Timeouts on every outbound call; circuit breaking for dependencies that can hang.
- Rate limits per client, returned in headers; 429 with `Retry-After`.

## Security
- Authentication on every endpoint unless documented as public (OAuth 2.1/OIDC or signed tokens; API keys only for server-to-server, scoped and rotatable).
- Object-level authorisation on every request (OWASP API Security Top 10: BOLA is the most common API flaw).
- Validate request bodies against the schema; reject unknown fields where mass assignment is a risk.
- Webhooks are signed (HMAC with timestamp) and verified; replay window enforced.

## Versioning
- Additive changes (new optional fields, new endpoints) are not breaking. Removing or renaming fields, changing types or meaning is breaking: new major version, deprecation notice (`Deprecation`/`Sunset` headers) and a published migration window.
