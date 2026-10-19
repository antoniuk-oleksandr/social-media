---
name: dynamodb
description: Implement DynamoDB persistence, transactional uniqueness, refresh rotation, and cursor-based access patterns without leaking database details.
---

# DynamoDB Persistence

The authoritative local copies are [`database.md`](../social-media-mvp/references/database.md) and [`mvp-technical-spec.md`](../social-media-mvp/references/mvp-technical-spec.md).

## Persistence rules

- Put DynamoDB tags only on persistence models.
- Keep models and SDK types inside repository adapters.
- Map model values to domain entities before returning from a repository.
- Translate conditional failures, missing items, transaction conflicts, and dependency failures into domain errors.
- Services must not inspect DynamoDB error types.

## Required MVP access patterns

- Users: lookup by ID and transactional username/email reservations.
- Posts: lookup by ID and reverse chronological feed query using `createdAtEpoch` plus a unique ID tie-breaker.
- Advertisements: admin CRUD and eligibility query for active, time-valid ads.
- Refresh sessions: hash-key lookup, atomic consume-and-replace rotation, family lookup, conditional revocation, and TTL cleanup.
- Feed: preserve the exact `LastEvaluatedKey` in the signed cursor. Do not filter a page after querying in a way that loses continuation state.

## Transactions and concurrency

- Enforce uniqueness with transactional reservation records, not a GSI alone.
- Use conditional writes for ownership, state transitions, token consumption, and revocation.
- A concurrent refresh using the same token must not result in two successful rotations.
- DynamoDB TTL is cleanup only; application logic must reject expired records.
