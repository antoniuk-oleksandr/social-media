---
name: quality-and-delivery
description: Apply the project's testing, coverage, logging, Terraform, SSM, and GitHub Actions delivery requirements.
---

# Quality and Delivery

The full local requirements are in [`go-architecture.md`](../social-media-mvp/references/go-architecture.md), [`database.md`](../social-media-mvp/references/database.md), and [`mvp-technical-spec.md`](../social-media-mvp/references/mvp-technical-spec.md).

## Testing

- Use Ginkgo and Gomega for unit specifications.
- Use Testcontainers for DynamoDB integration tests, including transactions, conditional writes, refresh rotation, and pagination keys.
- Use Resty for E2E tests against the deployed API Gateway → Lambda → DynamoDB path.
- The combined domain and application-service statement coverage must be at least 90%; CI must fail below the threshold.
- Test handlers separately for validation, authentication, status codes, response shape, and error mapping.
- Use deterministic fixtures for feed and advertisement acceptance tests.

## Logging

- Use Uber Zap structured logging.
- Log request ID, route, user ID when known, status, duration, dependency failures, state transitions, and feed decisions.
- Never log passwords, tokens, signing keys, authorization headers, or sensitive request bodies.
- Consume and handle every logging or dependency error; do not discard return values.

## Infrastructure and secrets

- Terraform owns Lambda functions, API Gateway HTTP API, DynamoDB, IAM, CloudWatch log groups, Lambda configuration, and OIDC permissions.
- GitHub Actions owns Go tests, ARM64 compilation, ZIP packaging, code deployment, and smoke tests.
- Use SSM Parameter Store SecureString for JWT and feed cursor signing keys.
- Do not store secrets in Git, `.tfvars`, GitHub Actions secrets, or Lambda source artifacts.

## Delivery checks

Before considering a task complete, run formatting, `go vet`, race-enabled tests, the applicable coverage threshold, and the relevant integration or E2E suite. Keep infrastructure changes and application-code deployments within their documented ownership boundaries.
