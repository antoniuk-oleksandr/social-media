---
name: social-media-mvp
description: Implement and review the Social Media MVP using the complete repository-owned product, API, architecture, database, and delivery contracts.
---

# Social Media MVP

This skill is self-contained. The complete source material is in the local `references/` directory; do not assume the repository's separate `docs/` directory is available to Hermes.

## Required routing

Before implementing a task, read the relevant references:

- API endpoint task: [`references/api-endpoints.md`](references/api-endpoints.md)
- Authentication, feed, validation, errors, infrastructure, or acceptance behavior: [`references/mvp-technical-spec.md`](references/mvp-technical-spec.md)
- Go package structure, boundaries, constructors, Fx, handlers, tests, logging, and CI/CD: [`references/go-architecture.md`](references/go-architecture.md)
- DynamoDB tables, keys, indexes, transactions, and access patterns: [`references/database.md`](references/database.md)
- Product scope and deferred phases: [`references/features.md`](references/features.md)
- Hermes local-agent workflow and Docker workspace: [`references/hermes.md`](references/hermes.md)
- Documentation map: [`references/README.md`](references/README.md)

Read the complete relevant reference before coding. When references conflict, `mvp-technical-spec.md` is authoritative for MVP contracts and `api-endpoints.md` is authoritative for endpoint catalog details, subject to the technical specification's explicit precedence rule.

## Non-negotiable implementation rules

- Use package-by-feature Clean Architecture.
- Keep domain entities, errors, and ports independent from HTTP, Lambda, Chi, Fx, and DynamoDB.
- Keep DTOs and DynamoDB models in their own layers and map between every layer.
- Services use domain entities, never HTTP DTOs or persistence models.
- Use interface-returning constructors and Uber Fx composition; do not introduce manual DI.
- Consume every error. Never use `_ , data := foo()` or discard an error.
- Preserve the documented authentication, validation, cursor, feed-ad, status, error, logging, testing, Terraform, SSM, and CI/CD contracts.
- Do not implement deferred product phases as part of an MVP task.

## Local infrastructure

Start manual local persistence with `infra/local/docker-compose.yaml`. Host processes use `http://127.0.0.1:8000`; containers attached to `social-media-local` use `http://dynamodb-local:8000`. Do not make integration tests depend on this running service; Testcontainers owns test lifecycle and isolation.

## Completion

Use the acceptance criteria in the relevant references and endpoint task. Verify formatting, vet, race-enabled tests, the applicable 90% coverage threshold, and the relevant integration or E2E tests before reporting completion.
