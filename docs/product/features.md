# Theads-like Social Feed — Feature Plan

> Authentication, validation, feed pagination, infrastructure, CI/CD, and MVP acceptance decisions are governed by [mvp-technical-spec.md](mvp-technical-spec.md).

## Product goal

Build a simple Threads-like social application in Go on AWS. The feed contains two different entity types—posts and advertisements—and returns them as one ordered list. The initial feed rule is:

> Return up to 20 posts followed by 1 advertisement when an eligible advertisement exists.

The mixed feed is the central university assignment requirement and is therefore a mandatory part of the MVP, not a later enhancement. The first release should demonstrate this clearly while keeping the infrastructure small enough for an MVP.

## Planned architecture

### MVP services

- **AWS Lambda** — stateless Go application handlers
- **Amazon API Gateway** — HTTP API entry point
- **Amazon DynamoDB** — storage for users, posts, ads, and basic feed data

### Deferred services

- **Redis** — caching, rate limiting, and potentially precomputed feeds
- **SQS** — asynchronous jobs and reliable background processing
- **S3** — image/media storage and static assets
- **AWS Distro for OpenTelemetry (ADOT)** — Lambda telemetry collection and OpenTelemetry trace export to AWS X-Ray

---

## Phase 1: MVP

> **Core MVP requirement:** The MVP must support advertisements and return a single feed that mixes two entity types—posts and ads—with up to 20 posts followed by 1 eligible ad whenever one is available.

### Users

- Register a user with a unique ID, display name, username, email, password hash, role, and creation timestamp.
- Provide `POST /api/v1/auth/register` to create an account and return an HS256 JWT access token plus an opaque refresh token.
- Provide `POST /api/v1/auth/login` to authenticate with email/username and password, then return an HS256 JWT access token plus an opaque refresh token.
- Provide `POST /api/v1/auth/refresh` with mandatory refresh-token rotation.
- Provide `POST /api/v1/auth/logout` to revoke the refresh-token session family.
- Hash passwords with bcrypt cost 12; never store or return plaintext passwords.
- Include the user's role in the JWT claims, for example `role: "user"` or `role: "admin"`.
- Validate JWT signatures, expiration, issuer, subject, and role claims on protected endpoints.
- Retrieve a user profile.
- Update basic profile information.

### Authentication and authorization

- Use a signed JWT access token in the `Authorization: Bearer <token>` header.
- Issue a cryptographically random opaque refresh token and store only its SHA-256 hash in DynamoDB.
- Include `iss`, `sub`, `aud`, `role`, `iat`, `nbf`, `exp`, and `jti` claims in access tokens.
- Use HS256 with `kid`-selected signing keys stored as SSM `SecureString` parameters.
- Assign the default `user` role during registration.
- Restrict advertisement creation and management to users with the `admin` role.
- Allow authenticated users to create and delete their own posts.
- Return `401 Unauthorized` for missing or invalid tokens and `403 Forbidden` for insufficient roles.

### Posts

- Create a text post.
- Retrieve a post by ID.
- Delete a post created by the current user.
- Store author information or an author reference with every post.

### Advertisements

- Create and manage advertisements for testing and administration purposes.
- Store ad ID, advertiser name, text, target HTTPS URL, status, start/end dates, and creation timestamp.
- Retrieve advertisements with `status: active` that are eligible for the requested feed.
- Mark an ad as an ad in the API response; it must never be indistinguishable from a user post.

### Mixed feed

The mixed feed is the primary MVP feature and must be implemented and tested before the MVP is considered complete.

- Provide an endpoint such as `GET /feed`.
- Return a single ordered list containing both posts and ads.
- Use a discriminated response shape, for example:

  ```json
  {
    "id": "...",
    "type": "post",
    "createdAt": "...",
    "author": {},
    "content": "..."
  }
  ```

  Ads should use `"type": "ad"` and include ad-specific fields such as `advertiser` and `targetUrl`.
- Sort feed entities by creation time descending.
- Return up to 20 posts followed by 1 eligible active ad when an ad exists.
- If fewer than 20 posts remain on a non-empty final page, return those posts followed by 1 eligible ad when one exists; an empty feed has no ad.
- If no eligible ads exist, return the available posts without failing the request.
- Support cursor-based pagination so clients can request the next section of the feed.
- Keep the ordering deterministic for the same cursor and data snapshot.
- Include enough metadata for clients to render the feed, such as `nextCursor` and `hasMore`.

### API and reliability

- Define HTTP routes for user, post, ad, and feed operations.
- Define `POST /register` and `POST /login` authentication routes.
- Validate request bodies, IDs, cursors, and required fields.
- Return consistent JSON error responses and suitable HTTP status codes.
- Apply JWT authentication middleware at the API boundary for protected routes.
- Add structured logs for Lambda requests and feed composition decisions.
- Add unit tests for feed mixing, pagination, validation, and authorization rules.
- Define a minimal DynamoDB access layer so business logic is not coupled directly to request handlers.

### MVP non-goals

- No Redis.
- No SQS.
- No S3, media uploads, or attachment IDs in the MVP.
- No real-time updates or notifications.
- No recommendation algorithm.
- No advanced ad targeting, bidding, or campaign management.
- No likes, comments, reposts, or follows unless needed for a basic demonstration.
- No password reset flow, email verification, OAuth providers, or public administrator provisioning.

---

## Phase 2: Core social features

- Follow and unfollow users.
- Generate a followed-users feed rather than only a global feed.
- Like and unlike posts.
- Add comments and threaded replies.
- Add reposts/quotes.
- Add user and post search.
- Add report, block, and moderation workflows.
- Introduce a proper identity provider and token-based authentication.
- Add integration and contract tests for the public API.
- Provide observability with structured JSON logging using uber-go/zap.
- Send Lambda logs to Amazon CloudWatch Logs with a log group for each function.
- Add request IDs, correlation IDs, Lambda request IDs, route names, user IDs, status codes, and durations to log fields.
- Configure CloudWatch log retention, metric filters, dashboards, and alarms for errors, throttling, and latency.
- Run optional OSS Grafana in Docker on a small EC2 host and connect it to CloudWatch and X-Ray.
- Provide OpenTelemetry tracing across API Gateway, Lambda, and DynamoDB.
- Propagate W3C trace context and correlate trace IDs with Zap logs.
- Export OpenTelemetry traces through the AWS Lambda OpenTelemetry/ADOT integration to AWS X-Ray.
- Add trace sampling and avoid collecting sensitive request bodies, passwords, JWTs, or other secrets.

## Phase 3: Media and asynchronous processing

- Add **S3** for profile images, post images, and other media.
- Add media metadata and upload authorization endpoints.
- Add **SQS** for asynchronous tasks such as media processing, moderation checks, and notification delivery.
- Add retries, dead-letter queues, and idempotent background handlers.
- Add image resizing, validation, and content-type restrictions.

## Phase 4: Performance and scale

- Add **Redis** for feed caching, hot post caching, rate limiting, and short-lived session data.
- Precompute or cache feed segments where appropriate.
- Add cache invalidation rules when posts or ads change.
- Improve DynamoDB partition-key design and access patterns based on measured traffic.
- Add read/write capacity monitoring and cost controls.
- Add load tests and performance budgets for feed latency.

## Phase 5: Advertising and production readiness

- Add ad campaigns, budgets, schedules, and targeting constraints.
- Track ad impressions and clicks with an auditable event model.
- Preserve the 20-post-to-1-ad rule as a configurable feed policy.
- Add frequency caps and safeguards against repeatedly showing the same ad.
- Add dashboards, alarms, tracing, and operational runbooks.
- Add automated deployments, staging environments, database migration/versioning practices, and rollback procedures.
- Add privacy, retention, abuse prevention, and compliance controls.

---

## MVP completion criteria

The MVP is complete when:

1. A client can create users, posts, and active test advertisements.
2. A client can register and log in through `/api/v1/auth/register` and `/api/v1/auth/login`.
3. Both endpoints return signed JWT access tokens and opaque refresh tokens; access tokens contain the required claims and role.
4. Protected endpoints enforce token validity and role-based authorization.
5. The MVP supports creating and retrieving active advertisements.
6. `GET /feed` returns posts and ads in one typed list.
7. The feed returns 20 posts plus 1 ad after a completed block of 20 organic posts; a non-empty incomplete final page includes 1 eligible ad when available, while an empty feed has no ad.
8. Pagination does not duplicate or silently skip feed entities.
9. Missing ads and empty datasets are handled gracefully.
10. The Go service runs as Lambda handlers behind API Gateway and persists data in DynamoDB.
11. Automated tests cover authentication, authorization, ad handling, the mixed-feed algorithm, and its edge cases.
