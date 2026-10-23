# Social Media MVP — Technical Specification

This document is the authoritative implementation contract for the MVP. It takes precedence over older examples in the feature, endpoint, database, and architecture documents where they differ.

## Authentication

- Access tokens are JWTs signed with HS256.
- Access tokens expire after 15 minutes.
- Required claims: `iss`, `sub`, `aud`, `role`, `iat`, `nbf`, `exp`, and `jti`.
- JWT headers contain `alg: HS256`, `typ: JWT`, and a `kid`.
- Verification validates the algorithm, signature, issuer, audience, expiry, not-before time, key ID, and required claims.
- Roles are `user` and `admin`.
- Passwords use bcrypt with cost 12.
- Refresh tokens are opaque, cryptographically random tokens with at least 256 bits of entropy.
- Refresh tokens expire after 7 days and are rotated on every successful refresh.
- Only SHA-256 refresh-token hashes are stored in DynamoDB.
- Reuse of a consumed refresh token revokes the entire session family.
- JWT signing keys are stored as SSM `SecureString` parameters and selected by `kid`.

Authentication routes:

| Method | Path | Authentication |
|---|---|---|
| POST | `/api/v1/auth/register` | Public |
| POST | `/api/v1/auth/login` | Public |
| POST | `/api/v1/auth/refresh` | Refresh token |
| POST | `/api/v1/auth/logout` | Refresh token |

Registration always assigns `role = user`. Administrator accounts are provisioned manually in DynamoDB by authorized AWS operators using a bcrypt cost-12 hash and consistent uniqueness records. The application has no public role-promotion or admin-creation endpoint.

## Refresh sessions

Table: `social-media-{env}-refresh-sessions`.

| Attribute | Type | Description |
|---|---|---|
| `tokenHash` | String, partition key | SHA-256 refresh-token digest |
| `sessionId` | String | Session-family identifier |
| `userId` | String | Token owner |
| `status` | String | `active`, `consumed`, or `revoked` |
| `createdAt` | Number | Unix timestamp |
| `expiresAt` | Number | Absolute expiration |
| `consumedAt` | Number | Rotation timestamp |
| `ttl` | Number | DynamoDB cleanup TTL |
| `version` | Number | Optimistic concurrency version |

A GSI on `sessionId` supports session-family revocation. Refresh rotation uses an atomic DynamoDB transaction. TTL is cleanup only; application expiration checks remain mandatory.

## Request validation

All JSON endpoints require `Content-Type: application/json`, reject unknown fields and duplicate JSON keys, and accept exactly one JSON object with no trailing values. JSON bodies are limited to 64 KiB.

- Username: 3–30 lowercase ASCII letters, digits, or underscores.
- Email: valid address, maximum 254 bytes, normalized case-insensitively.
- Password: 12–72 bytes.
- Display name: 1–80 Unicode code points.
- Post content: 1–500 Unicode code points after trimming.
- Advertisement content: 1–1,000 Unicode code points.
- Advertisement URL: HTTPS, valid hostname, no credentials or fragments, maximum 2,048 bytes.
- Feed limit: integer 1–50, default 20.
- Feed cursor: Base64URL token, maximum 4,096 bytes.
- IDs: UUIDv7.
- Authorization: exactly one Bearer token.

Malformed JSON and invalid query syntax return `400`. Valid JSON with invalid fields returns `422`. Oversized bodies return `413`.

## Feed contract

The feed is ordered by descending `(createdAt, postId)`. `limit` is the maximum number of organic posts, not the total number of response items.

- A page with 20 posts may contain 20 posts and 1 ad.
- A page with fewer than 20 remaining posts contains those posts and no additional ad merely because the page ended.
- One ad is inserted after each completed block of 20 organic posts across the pagination session.
- If no eligible ad exists, only posts are returned.
- Ads do not consume the organic post limit.

The cursor is an authenticated, opaque token:

```text
base64url(deterministic-json-payload).base64url(HMAC-SHA256(payload, key))
```

Its payload contains version, key ID, authenticated user ID, snapshot timestamp, last evaluated key, organic count, ad seed, shown ad IDs, limit, issue time, and expiry. It is signed but not encrypted and must contain no secrets. The user and limit are bound to the cursor.

The first request establishes `snapshotAt`. Later pages exclude posts created after it and use `(createdAt, postId)` ordering. The snapshot is a timestamp boundary, not an immutable database snapshot.

Eligible ads have `status = active`, are within their start/end window, and are not deleted or disabled. Ads are sorted by ID; selection starts from a deterministic seed offset and skips previously shown ads until the eligible set is exhausted. Repetition is then permitted.

## Domain errors and responses

All errors use:

```json
{
  "error": {
    "code": "USERNAME_TAKEN",
    "message": "Username is already registered",
    "requestId": "request-identifier"
  }
}
```

Required mappings include:

| Condition | HTTP | Code |
|---|---:|---|
| Malformed JSON | 400 | `INVALID_JSON` |
| Invalid query | 400 | `INVALID_QUERY` |
| Invalid cursor | 400 | `INVALID_CURSOR` |
| Expired cursor | 400 | `CURSOR_EXPIRED` |
| Missing authentication | 401 | `UNAUTHORIZED` |
| Invalid access token | 401 | `INVALID_TOKEN` |
| Invalid refresh token | 401 | `INVALID_REFRESH_TOKEN` |
| Insufficient role | 403 | `FORBIDDEN` |
| Not found | 404 | `NOT_FOUND` |
| Username conflict | 409 | `USERNAME_TAKEN` |
| Email conflict | 409 | `EMAIL_TAKEN` |
| Invalid state transition | 409 | `INVALID_STATE` |
| Invalid fields | 422 | `VALIDATION_FAILED` |
| Body too large | 413 | `PAYLOAD_TOO_LARGE` |
| Rate limited | 429 | `RATE_LIMITED` |
| Internal error | 500 | `INTERNAL_ERROR` |
| Dependency unavailable | 503 | `SERVICE_UNAVAILABLE` |

Infrastructure errors are logged internally and never returned directly.

## Infrastructure and deployment

Manual local development uses `infra/local/docker-compose.yaml` to run DynamoDB Local. The service is reachable through the published local endpoint from host processes and as `http://dynamodb-local:8000` from containers attached to the dedicated `social-media-local` Docker network. The Compose stack is not used by production deployments or as a shared dependency for integration tests; Testcontainers owns integration-test database lifecycle and isolation.

Terraform under `infra/dev` manages the initial development Lambda function, API Gateway HTTP API v2, DynamoDB, IAM, CloudWatch, Lambda environment variables, and the private application media S3 bucket. The environment currently uses local Terraform state, with remote state migration deferred until the deployment workflow is established. A future `infra/prod` stack will manage production separately.

Lambda functions use `provided.al2023`, `arm64`, handler `bootstrap`, 256 MB memory, and a 10-second timeout. The initial dev stack uses local Terraform state. Production state must use a protected, encrypted, versioned remote backend before production deployment.

SSM Parameter Store stores signing keys and other production secrets as `SecureString` values. Go configuration reads environment variables and resolves configured SSM parameter names during cold-start initialization. Feature packages receive typed configuration through constructors.

GitHub Actions runs tests, coverage enforcement, static analysis, ARM64 builds, and ZIP packaging. Terraform owns infrastructure lifecycle. GitHub Actions deploys compiled Lambda ZIPs to already-created functions using AWS OIDC and the narrowly scoped deployment role.

## Testing and coverage

- Ginkgo and Gomega: unit tests for domain and application packages.
- Testcontainers with DynamoDB Local or an isolated AWS environment: repository integration tests.
- Resty: E2E tests against the deployed development API.
- Minimum 90% combined statement coverage for the complete Go project (`./...`).
- Handler coverage is reported separately; integration, E2E, Lambda bootstrap, generated code, and Terraform are outside the 90% threshold.

The MVP acceptance flow covers registration, duplicate usernames, login, 45 posts, admin ad creation, feed pagination, cursor tampering, refresh rotation and replay revocation, ownership checks, and Lambda redeployment without infrastructure changes.
