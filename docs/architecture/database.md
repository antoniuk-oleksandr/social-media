# Theads-like Application — Database Design

> The authoritative MVP contracts are defined in [mvp-technical-spec.md](../product/mvp-technical-spec.md). This document provides the schema and access-pattern detail.

## Database decisions

- Database: Amazon DynamoDB.

## Local development

Manual local development runs DynamoDB Local through [`infra/local/docker-compose.yaml`](../../infra/local/docker-compose.yaml). Start it from the repository root:

```bash
docker compose -f infra/local/docker-compose.yaml up -d
```

The service is named `dynamodb-local` on the Docker network `social-media-local` and listens on port `8000`. Host processes connect to the published local DynamoDB endpoint; containers attached to `social-media-local` connect to `http://dynamodb-local:8000`.

The Compose network is dedicated to this local stack and the host port is bound to loopback. The named Docker volume `social-media-dynamodb-local-data` persists local data between ordinary `down` and `up` operations. Use `docker compose -f infra/local/docker-compose.yaml down -v` only when intentionally deleting local data.

This stack is not a substitute for integration-test isolation. Testcontainers owns the lifecycle of integration-test DynamoDB Local containers and tests must use isolated tables.
- MVP uses separate logical tables for users, posts, advertisements, and optional feed state.
- Every feed entity has an entityType value: post or ad.
- Resource IDs are canonical UUIDv7 strings. Persisted and accepted resource IDs use DynamoDB String values.
- Timestamps use ISO 8601 UTC strings. Numeric epoch timestamps are stored for sorting.
- Passwords are stored only as bcrypt hashes using cost 12.
- Access tokens are HS256 JWTs. Refresh tokens are opaque random tokens; only their SHA-256 hashes are stored in DynamoDB.
- Redis, S3, and SQS are deferred from the MVP.

## Common attributes

| Attribute | Type | Description |
|---|---|---|
| id | String | Entity identifier |
| entityType | String | Entity discriminator |
| createdAt | String | ISO 8601 creation timestamp |
| createdAtEpoch | Number | Epoch timestamp for sorting |
| updatedAt | String | ISO 8601 update timestamp |
| version | Number | Optional optimistic-concurrency version |

---

## Phase 1: MVP tables

### Users

Table: theads-users

| Attribute | Type | Key | Description |
|---|---|---|---|
| userId | String | Partition key | User ID |
| email | String | — | Normalized login email |
| username | String | — | Normalized unique username |
| displayName | String | — | Public display name |
| passwordHash | String | — | bcrypt password hash |
| role | String | — | user or admin |
| status | String | — | active, blocked, or deleted |
| createdAt | String | — | Creation timestamp |
| updatedAt | String | — | Last update timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| EmailIndex | email | — | Find a user during login |
| UsernameIndex | username | — | Find profiles |

Access patterns:

- Get a user by userId.
- Find a user by email for login.
- Find a user by username.
- Update the authenticated user's profile.

### User uniqueness

Email addresses and usernames are normalized before storage. Registration uses a dedicated uniqueness table, `theads-uniques`, or equivalent transactional sentinel records:

| Attribute | Type | Key | Description |
|---|---|---|---|
| uniqueKey | String | Partition key | `EMAIL#<normalized-email>` or `USERNAME#<normalized-username>` |
| userId | String | — | User that owns the value |
| createdAt | String | — | Reservation timestamp |

User creation reserves both values and creates the user in one transaction. A conditional put prevents duplicate email or username registration.

### Posts

Table: theads-posts

| Attribute | Type | Key | Description |
|---|---|---|---|
| postId | String | Partition key | Post ID |
| createdAtEpoch | Number | — | Creation time in epoch milliseconds; used by feed indexes |
| entityType | String | — | Always post |
| authorId | String | — | User who created the post |
| content | String | — | Text content |
| status | String | — | active, deleted, or moderated |
| createdAt | String | — | Creation timestamp |
| updatedAt | String | — | Last update timestamp |
| feedPartition | String | — | Recommended value: GLOBAL |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| FeedPostsIndex | feedPartition | createdAtEpoch | Query global posts in reverse chronological order |
| AuthorPostsIndex | authorId | createdAtEpoch | List posts created by a user |

Feed queries use ScanIndexForward = false. `createdAtEpoch` is stored in epoch milliseconds. If two entities share the same millisecond, the application uses the opaque entity ID as a deterministic tie-breaker when constructing the cursor.

Access patterns:

- Create a post.
- Get a post by postId. The base table uses postId as its only primary-key attribute.
- Delete a post owned by the current user.
- Query global posts for feed construction.
- List posts by author.

### Advertisements

Table: theads-ads

Regular users do not access this table through the ads API group. The feed Lambda reads eligible ads internally.

| Attribute | Type | Key | Description |
|---|---|---|---|
| adId | String | Partition key | Advertisement ID |
| createdAtEpoch | Number | — | Creation time in epoch milliseconds; used by feed indexes |
| entityType | String | — | Always ad |
| advertiser | String | — | Advertiser name |
| content | String | — | Ad text |
| targetUrl | String | — | Target HTTPS URL |
| startsAtEpoch | Number | — | Start of eligibility window |
| endsAtEpoch | Number | — | End of eligibility window |
| status | String | — | draft, active, paused, or expired |
| createdBy | String | — | Admin user ID |
| createdAt | String | — | Creation timestamp |
| updatedAt | String | — | Last update timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| ActiveAdsIndex | status | createdAtEpoch | Find active ads in creation-time order; filter by eligibility window |

The feed queries status = active and checks startsAtEpoch <= now <= endsAtEpoch.

Access patterns:

- Create, update, and delete an ad as an admin.
- Retrieve an ad as an admin.
- Query eligible ads internally for feed construction.

### Feed state

Table: theads-feed-state

This table is not required for the MVP. The first implementation calculates the feed directly from posts and ads and uses an opaque signed cursor. Server-side feed state may be introduced later if needed.

| Attribute | Type | Key | Description |
|---|---|---|---|
| feedId | String | Partition key | Feed snapshot ID |
| cursorId | String | Sort key | Cursor state ID |
| postPosition | Number | — | Number of posts consumed |
| adPosition | Number | — | Number of ads consumed |
| createdAtEpoch | Number | — | Cursor creation time |
| expiresAtEpoch | Number | — | TTL timestamp |

TTL attribute: expiresAtEpoch.

The client cursor is an opaque signed token containing the feed snapshot timestamp and the last evaluated post and ad keys. It must not expose internal DynamoDB keys directly to clients.

### MVP feed model

Posts and ads remain in separate tables. The feed Lambda returns one typed list:

    {
      "items": [
        {
          "id": "0192c5f4-7b2a-7abf-8def-1234567890ab",
          "type": "post",
          "authorId": "0192c5f4-7b2a-7abf-8def-1234567890ab",
          "content": "Post number 20",
          "createdAt": "2026-10-18T12:00:00Z"
        },
        {
          "id": "0192c5f4-7b2a-7abe-8def-1234567890ab",
          "type": "ad",
          "advertiser": "Example Company",
          "content": "Try our new product!",
          "targetUrl": "https://example.com",
          "createdAt": "2026-10-18T11:00:00Z"
        }
      ],
      "nextCursor": "opaque-cursor",
      "hasMore": true
    }

The feed algorithm reads up to 20 posts in reverse chronological order and inserts one eligible ad after a completed block of 20 organic posts. If fewer than 20 posts remain on a non-empty final page, it returns those posts followed by one eligible ad when one exists. An empty feed has no ad. Ads are not copied into the posts table.

---

## Phase 2: Core social features

### Follows

Table: theads-follows

| Attribute | Type | Key | Description |
|---|---|---|---|
| followerId | String | Partition key | User who follows |
| followedId | String | Sort key | User being followed |
| createdAt | String | — | Follow timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| FollowedByIndex | followedId | createdAt | Find followers of a user |

### Likes

Table: theads-likes

| Attribute | Type | Key | Description |
|---|---|---|---|
| postId | String | Partition key | Post being liked |
| userId | String | Sort key | User who liked it |
| createdAt | String | — | Like timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| UserLikesIndex | userId | createdAt | List posts liked by a user |

The composite primary key prevents the same user from liking the same post twice.

### Comments

Table: theads-comments

| Attribute | Type | Key | Description |
|---|---|---|---|
| postId | String | Partition key | Post containing the comment |
| commentKey | String | Sort key | createdAtEpoch#commentId |
| commentId | String | — | Comment ID |
| authorId | String | — | Comment author |
| content | String | — | Comment text |
| parentCommentId | String | — | Parent comment for replies |
| status | String | — | active, deleted, or moderated |
| createdAt | String | — | Creation timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| AuthorCommentsIndex | authorId | createdAt | List comments by author |
| ParentCommentsIndex | parentCommentId | createdAt | List replies to a comment |

### Reposts

Table: theads-reposts

| Attribute | Type | Key | Description |
|---|---|---|---|
| userId | String | Partition key | User making the repost |
| postId | String | Sort key | Reposted post |
| comment | String | — | Optional quote text |
| createdAt | String | — | Repost timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| PostRepostsIndex | postId | createdAt | List users who reposted a post |

### Blocks

Table: theads-blocks

| Attribute | Type | Key | Description |
|---|---|---|---|
| blockerId | String | Partition key | User who blocks |
| blockedId | String | Sort key | User being blocked |
| createdAt | String | — | Block timestamp |

### Reports

Table: theads-reports

| Attribute | Type | Key | Description |
|---|---|---|---|
| reportId | String | Partition key | Report ID |
| createdAtEpoch | Number | Sort key | Report ordering |
| reporterId | String | — | User submitting the report |
| targetType | String | — | post, user, comment, or ad |
| targetId | String | — | Reported entity ID |
| reason | String | — | Report reason |
| details | String | — | Optional explanation |
| status | String | — | open, reviewing, resolved, or rejected |
| createdAt | String | — | Creation timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| ReportStatusIndex | status | createdAtEpoch | Moderation queue |
| ReportedTargetIndex | targetId | createdAtEpoch | Find reports for an entity |

---

## Phase 3: Images and videos

### Attachments

Table: theads-attachments

This table stores metadata only. Binary content is stored in S3.

| Attribute | Type | Key | Description |
|---|---|---|---|
| attachmentId | String | Partition key | Attachment ID |
| ownerId | String | — | Uploading user |
| attachmentType | String | — | post, comment, or avatar |
| parentId | String | — | Post ID, comment ID, or user ID |
| mediaType | String | — | image or video, detected by backend |
| contentType | String | — | Detected MIME type |
| sizeBytes | Number | — | Detected file size |
| s3Key | String | — | Private S3 object key |
| status | String | — | pending, processing, ready, or failed |
| url | String | — | Generated access URL when ready |
| createdAt | String | — | Creation timestamp |

GSIs:

| Index | Partition key | Sort key | Purpose |
|---|---|---|---|
| OwnerAttachmentsIndex | ownerId | createdAt | List a user's uploads |
| ParentAttachmentsIndex | parentId | createdAt | List attachments on a post, comment, or user |

The backend detects and validates media type, MIME type, and size. Client-provided metadata is not trusted.

---

## Phase 4: Performance and scale

Redis is not part of the DynamoDB schema. It may cache:

- Hot feed pages.
- Frequently requested posts and profiles.
- Rate-limit counters.
- Short-lived feed cursors.

DynamoDB remains the source of truth. Cache entries must be disposable and reconstructable.

---

## Data integrity and operational rules

- Use a uniqueness table or transactional sentinel records for unique email and username registration. GSI lookups alone are not sufficient for concurrent uniqueness enforcement.
- Use conditional writes for post and comment ownership checks.
- Use transactions when creating an entity and required relationship records together.
- Enable point-in-time recovery for production tables.
- Enable encryption at rest using AWS-managed KMS keys initially.
- Configure TTL only for temporary feed cursor state.
- Keep S3 object keys and signed URLs out of public user input.
- Never store plaintext passwords, JWT secrets, or private media in DynamoDB.
- Add DynamoDB alarms for throttled reads/writes, system errors, and consumed capacity.

### Refresh sessions

Table: `social-media-{env}-refresh-sessions`.

| Attribute | Type | Key | Description |
|---|---|---|---|
| tokenHash | String | Partition key | SHA-256 digest of the opaque refresh token |
| sessionId | String | — | Session-family identifier |
| userId | String | — | Token owner |
| status | String | — | active, consumed, or revoked |
| createdAt | Number | — | Unix timestamp |
| expiresAt | Number | — | Absolute expiration |
| consumedAt | Number | — | Rotation timestamp |
| ttl | Number | — | DynamoDB cleanup TTL |
| version | Number | — | Optimistic-concurrency version |

GSI: `SessionIdIndex` with `sessionId` as the partition key. Refresh rotation uses an atomic transaction to consume the old token and create its replacement. Reuse of a consumed token revokes the entire session family.
