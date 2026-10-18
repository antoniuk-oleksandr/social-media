# Theads-like Social Feed — API Endpoints

> The authoritative MVP contracts are defined in [mvp-technical-spec.md](../product/mvp-technical-spec.md). This document provides the endpoint catalog and examples.

## Conventions

- Base path: /api/v1
- Content type: application/json
- Protected requests use Authorization: Bearer <access-jwt>.
- IDs are opaque strings. Timestamps use ISO 8601 UTC.
- Resource IDs must be valid UUIDv7 values; shortened IDs in examples are illustrative only.
- 401 means missing or invalid authentication. 403 means insufficient role.

Common error response:

    {
      "error": {
        "code": "VALIDATION_ERROR",
        "message": "email is required",
        "requestId": "req_123"
      }
    }

---

## Phase 1: MVP

The MVP includes authentication, users, posts, ads, and the primary mixed feed. The feed returns up to 20 posts followed by 1 eligible ad when one exists.

### Authentication

#### POST /api/v1/auth/register

Authentication: none

Request:

    {
      "username": "alice",
      "email": "alice@example.com",
      "password": "secure-password",
      "displayName": "Alice"
    }

Response: 201 Created

    {
      "user": {
        "id": "usr_123",
        "username": "alice",
        "email": "alice@example.com",
        "displayName": "Alice",
        "role": "user",
        "createdAt": "2026-10-18T12:00:00Z"
      },
      "accessToken": "eyJhbGciOiJIUzI1NiIs...",
      "refreshToken": "cryptographically-random-opaque-token",
      "tokenType": "Bearer",
      "expiresIn": 900,
      "refreshExpiresIn": 604800
    }

The default registration role is user. Errors: 422 invalid fields, 409 duplicate username/email.

#### POST /api/v1/auth/login

Authentication: none

Request:

    {
      "login": "alice@example.com",
      "password": "secure-password"
    }

Response: 200 OK

    {
      "user": {
        "id": "usr_123",
        "username": "alice",
        "displayName": "Alice",
        "role": "user"
      },
      "accessToken": "eyJhbGciOiJIUzI1NiIs...",
      "refreshToken": "cryptographically-random-opaque-token",
      "tokenType": "Bearer",
      "expiresIn": 900,
      "refreshExpiresIn": 604800
    }

    The access JWT uses HS256 and contains iss, sub, aud, role, iat, nbf, exp, and jti claims. Error: 401 invalid credentials.

#### POST /api/v1/auth/refresh

Authentication: none

Request:

    {
      "refreshToken": "existing-opaque-refresh-token"
    }

Response: 200 OK — a new access JWT and a new opaque refresh token. The old refresh token is consumed atomically.

Errors: 400 invalid request, 401 invalid, expired, revoked, or reused refresh token.

#### POST /api/v1/auth/logout

Authentication: refresh token

Request:

    {
      "refreshToken": "existing-opaque-refresh-token"
    }

Response: 204 No Content. The refresh-token session family is revoked.

### Users

#### GET /api/v1/users/{userId}

Authentication: optional

Response: 200 OK

    {
      "id": "usr_123",
      "username": "alice",
      "displayName": "Alice",
      "role": "user",
      "createdAt": "2026-10-18T12:00:00Z"
    }

Error: 404 user not found.

#### PATCH /api/v1/users/me

Authentication: user

Request:

    {
      "displayName": "Alice Smith",
      "username": "alice_smith"
    }

Response: 200 OK — updated user object.

Errors: 422 invalid fields, 401 unauthenticated, 409 username already exists.

### Posts

#### POST /api/v1/posts

Authentication: user

Request:

    {
      "content": "Hello from the MVP!"
    }

Response: 201 Created

    {
      "id": "pst_123",
      "authorId": "usr_123",
      "content": "Hello from the MVP!",
      "createdAt": "2026-10-18T12:35:00Z"
    }

Errors: 422 invalid content, 401 unauthenticated.

#### GET /api/v1/posts/{postId}

Authentication: optional

Response: 200 OK — post object in the same shape as POST /posts.

Error: 404 post not found.

#### DELETE /api/v1/posts/{postId}

Authentication: post owner

Request body: none

Response: 204 No Content.

Errors: 401 unauthenticated, 403 not the owner, 404 post not found.

#### GET /api/v1/posts

Authentication: optional

Query: limit (default 20, max 100), cursor. Results are sorted by createdAt descending.

Response: 200 OK

    {
      "items": [],
      "nextCursor": "eyJ0Ijoi...",
      "hasMore": true
    }

### Advertisements

#### POST /api/v1/ads

Authentication: admin

Request:

    {
      "advertiser": "Example Company",
      "content": "Try our new product!",
      "targetUrl": "https://example.com",
      "startsAt": "2026-10-18T00:00:00Z",
      "endsAt": "2026-11-18T00:00:00Z",
      "status": "active"
    }

Response: 201 Created

    {
      "id": "ad_123",
      "type": "ad",
      "advertiser": "Example Company",
      "content": "Try our new product!",
      "targetUrl": "https://example.com",
      "status": "active",
      "startsAt": "2026-10-18T00:00:00Z",
      "endsAt": "2026-11-18T00:00:00Z",
      "createdAt": "2026-10-18T12:40:00Z"
    }

Errors: 422 invalid schedule/content, 401 unauthenticated, 403 not an admin.

#### GET /api/v1/ads/{adId}

Authentication: admin

Response: 200 OK — ad object.

Errors: 401 unauthenticated, 403 not an admin, 404 ad not found.

#### GET /api/v1/ads

Authentication: admin. Only administrators can access the /api/v1/ads resource group. Regular users receive 403 Forbidden.

Query: status, limit, cursor. Administrators may request any status; the default is active.

Response: 200 OK — paginated list of ad objects.

#### PATCH /api/v1/ads/{adId}

Authentication: admin

Request:

    {
      "content": "Updated ad text",
      "status": "paused"
    }

Response: 200 OK — updated ad object.

#### DELETE /api/v1/ads/{adId}

Authentication: admin

Request body: none

Response: 204 No Content.

### Mixed feed

#### GET /api/v1/feed

Authentication: optional for MVP

The feed is the exception to the admin-only ads resource rule: regular users may receive eligible advertisements as feed items, but they cannot call any endpoint in the /api/v1/ads resource group.

Query: limit (default 20, max 50 organic posts), cursor. The feed may return one additional ad after a completed block of 20 organic posts.

Composition rules:

1. Read posts in deterministic reverse chronological order using createdAtEpoch descending.
2. Insert 1 eligible active ad after every completed block of 20 organic posts across the pagination session.
3. If fewer than 20 posts remain on the final page, return those posts without an additional ad merely because the page ended.
4. If no eligible ad exists, return the available posts without failing.
5. Every entity must have type set to post or ad.
6. The opaque cursor must preserve a snapshot timestamp and the last evaluated post/ad keys to prevent duplicates or skipped entities.

Response: 200 OK

    {
      "items": [
        {
          "type": "post",
          "data": {
            "id": "pst_020",
            "author": {
              "id": "usr_123",
              "username": "alice",
              "displayName": "Alice"
            },
            "content": "Post number 20",
            "createdAt": "2026-10-18T12:00:00Z"
          }
        },
        {
          "type": "ad",
          "data": {
            "id": "ad_001",
            "advertiser": "Example Company",
            "content": "Try our new product!",
            "targetUrl": "https://example.com",
            "createdAt": "2026-10-18T11:00:00Z"
          }
        }
      ],
      "nextCursor": "eyJzbmFwc2hvdCI6IjIwMjYtMTAtMThUMTI6MDA6MDBaIiwicG9zdEtleSI6Li4ufQ==",
      "hasMore": true
    }

Error: 400 invalid cursor or limit. Expired cursors return `CURSOR_EXPIRED`; invalid signatures or incompatible parameters return `INVALID_CURSOR`.

---

## Phase 2: Core social features

#### POST /api/v1/users/{userId}/follow

Authentication: user. Request body: none. Response: 201 Created with followerId, followedId, createdAt.

#### DELETE /api/v1/users/{userId}/follow

Authentication: user. Response: 204 No Content.

#### GET /api/v1/feed/following

Authentication: user. Query: limit, cursor. Response: 200 OK using the same mixed-feed envelope and up-to-20-posts-plus-1-ad rule as the MVP feed.

#### POST /api/v1/posts/{postId}/likes

Authentication: user. Request body: none. Response: 201 Created with postId, userId, createdAt.

#### DELETE /api/v1/posts/{postId}/likes

Authentication: user. Response: 204 No Content.

#### POST /api/v1/posts/{postId}/comments

Authentication: user

Request:

    {
      "content": "Nice post!",
      "parentCommentId": null
    }

Response: 201 Created — comment object.

#### GET /api/v1/posts/{postId}/comments

Authentication: optional. Query: limit, cursor. Response: 200 OK — paginated comments.

#### POST /api/v1/posts/{postId}/reposts

Authentication: user

Request:

    {
      "comment": "Worth reading"
    }

Response: 201 Created — repost object.

#### GET /api/v1/search

Authentication: optional. Query: q, type (users or posts), limit, cursor. Response: 200 OK — paginated results.

#### POST /api/v1/reports

Authentication: user

Request:

    {
      "targetType": "post",
      "targetId": "pst_123",
      "reason": "spam",
      "details": "Repeated promotional content"
    }

Response: 201 Created — report object with id, target, status, and createdAt.

#### POST /api/v1/users/{userId}/block

Authentication: user. Request body: none. Response: 201 Created — block object.

#### DELETE /api/v1/users/{userId}/block

Authentication: user. Response: 204 No Content.

---

## Phase 3: Images and videos

Images and videos belong to the resource they are attached to. There is no standalone media resource group. S3 stores the files and SQS may process thumbnails, transcoding, moderation, and status updates asynchronously.

#### POST /api/v1/posts/{postId}/attachments

Authentication: post owner

Request:

    {
      "fileName": "photo.jpg"
    }

Response: 201 Created

    {
      "attachmentId": "att_123",
      "uploadUrl": "https://s3.example/...",
      "expiresAt": "2026-10-18T14:00:00Z",
      "status": "pending"
    }

The backend determines the actual media type, MIME type, and size from the uploaded object. It accepts only images and videos and rejects unsupported types or files that exceed configured size limits. Client-provided metadata is not trusted.

#### POST /api/v1/posts/{postId}/attachments/{attachmentId}/complete

Authentication: post owner

Request:

    {}

Response: 202 Accepted

    {
      "attachmentId": "att_123",
      "status": "processing"
    }

#### GET /api/v1/posts/{postId}/attachments

Authentication: optional

Response: 200 OK

    {
      "items": [
        {
          "attachmentId": "att_123",
          "mediaType": "image",
          "contentType": "image/jpeg",
          "status": "ready",
          "url": "https://cdn.example/photo.jpg"
        }
      ]
    }

#### POST /api/v1/users/me/avatar

Authentication: user

Request:

    {
      "fileName": "avatar.jpg"
    }

Response: 200 OK

    {
      "userId": "usr_123",
      "avatar": {
        "attachmentId": "att_456",
        "mediaType": "image",
        "status": "ready",
        "url": "https://cdn.example/avatar.jpg"
      }
    }

#### DELETE /api/v1/users/me/avatar

Authentication: user

Request body: none

Response: 204 No Content.

#### POST /api/v1/comments/{commentId}/attachments

Authentication: comment owner

Request:

    {
      "fileName": "comment-photo.jpg"
    }

Response: 201 Created — attachment object with attachmentId, uploadUrl, expiresAt, status, and mediaType.

#### POST /api/v1/comments/{commentId}/attachments/{attachmentId}/complete

Authentication: comment owner. Request body: none. Response: 202 Accepted — attachmentId and status processing.

#### GET /api/v1/comments/{commentId}/attachments

Authentication: optional. Response: 200 OK — list of comment attachment objects.

Post creation and comment creation may accept completed attachment IDs:

Post:

    {
      "content": "A trip video",
      "attachmentIds": ["att_123"]
    }

Comment:

    {
      "content": "Here is a photo",
      "attachmentIds": ["att_456"],
      "parentCommentId": null
    }

Attachment IDs must refer to completed image or video uploads owned by the authenticated user. The API should enforce attachment count and size limits.

---

## Phase 4: Performance and scale

Redis is introduced behind existing endpoints for caching and rate limiting. No new public endpoints are required.

---

## MVP endpoint summary

| Method | Path | Auth | Purpose |
|---|---|---|---|
| POST | /auth/register | None | Create account and return access/refresh tokens |
| POST | /auth/login | None | Authenticate and return access/refresh tokens |
| POST | /auth/refresh | Refresh token | Rotate refresh token and issue access token |
| POST | /auth/logout | Refresh token | Revoke refresh-token session family |
| GET | /users/{userId} | Optional | Retrieve profile |
| PATCH | /users/me | User | Update own profile |
| POST | /posts | User | Create post |
| GET | /posts/{postId} | Optional | Retrieve post |
| GET | /posts | Optional | List posts |
| DELETE | /posts/{postId} | Owner | Delete own post |
| POST | /ads | Admin | Create ad |
| GET | /ads/{adId} | Admin | Retrieve ad |
| GET | /ads | Admin | List ads |
| PATCH | /ads/{adId} | Admin | Update ad |
| DELETE | /ads/{adId} | Admin | Delete ad |
| GET | /feed | Optional | Mixed feed: up to 20 posts, then 1 ad |
