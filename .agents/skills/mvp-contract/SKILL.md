---
name: mvp-contract
description: Implement the Social Media MVP according to its authentication, validation, feed, error, and deployment contracts.
---

# Social Media MVP Contract

## Authority

The authoritative local copies are [`mvp-technical-spec.md`](../social-media-mvp/references/mvp-technical-spec.md) and [`api-endpoints.md`](../social-media-mvp/references/api-endpoints.md).

## Mandatory behavior

- Use HS256 JWT access tokens with `iss`, `sub`, `aud`, `role`, `iat`, `nbf`, `exp`, and `jti` claims.
- Validate the fixed algorithm, signature, issuer, audience, time claims, required claims, and `kid`.
- Use bcrypt cost 12 for passwords.
- Use cryptographically random opaque refresh tokens; persist only SHA-256 hashes.
- Rotate refresh tokens atomically. Reuse revokes the complete session family.
- Normalize usernames and emails and enforce both as unique with transactional reservations.
- Use one advertisement status field: `draft`, `active`, `paused`, or `expired`.
- Sort posts by `createdAtEpoch` descending with a post ID tie-breaker.
- Use authenticated cursor pagination with snapshot state. Never silently discard DynamoDB continuation state.
- The feed limit counts organic posts. A complete block returns 20 posts plus one eligible ad; an incomplete final page returns fewer posts without an extra ad.
- Return typed feed items with `type: post` or `type: ad`.
- Map expected domain errors to the documented HTTP response and never expose DynamoDB errors.

## Security rules

- Never log passwords, JWTs, refresh tokens, signing keys, or raw request secrets.
- Public registration can only create `user` accounts.
- Login failures must not reveal whether a username or email exists.
- Refresh cursors are authenticated, user-bound, size-bound, and expiry-bound.

## Scope

Implement only the MVP unless the task explicitly targets a later phase. Media attachments, follows, likes, comments, Redis, SQS, S3, and tracing are deferred features.
