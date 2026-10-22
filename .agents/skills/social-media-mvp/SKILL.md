---
name: social-media-mvp
description: Implement and review the Social Media MVP using the complete repository-owned product, API, architecture, database, and delivery contracts.
---

# Social Media MVP

The repository `docs/` directory is authoritative. Read the relevant files there before implementation; the local `references/` copies are supporting material only.

## Required routing

Before implementing a task, read the relevant references:

- API endpoint task: [`docs/api/endpoints.md`](../../docs/api/endpoints.md)
- Authentication, feed, validation, errors, infrastructure, or acceptance behavior: [`docs/product/mvp-technical-spec.md`](../../docs/product/mvp-technical-spec.md)
- Go package structure, boundaries, constructors, Fx, handlers, tests, logging, and CI/CD: [`docs/architecture/go-architecture.md`](../../docs/architecture/go-architecture.md)
- DynamoDB tables, keys, indexes, transactions, and access patterns: [`docs/architecture/database.md`](../../docs/architecture/database.md)
- Product scope and deferred phases: [`docs/product/features.md`](../../docs/product/features.md)
- Hermes local-agent workflow and Docker workspace: [`docs/operations/hermes.md`](../../docs/operations/hermes.md)
- Documentation map: [`docs/README.md`](../../docs/README.md)
- Hermes implementation workflow: [`hermes-workflow/SKILL.md`](../hermes-workflow/SKILL.md)

Read the complete relevant document before coding. When documents conflict, `docs/product/mvp-technical-spec.md` is authoritative for MVP contracts and `docs/api/endpoints.md` is authoritative for endpoint catalog details, subject to the technical specification's explicit precedence rule.

For implementation tasks executed by Hermes, the workflow skill is mandatory:
the parent delegates `hermes-dev`, then `hermes-tester`, then
`hermes-reviewer`. The reviewer-driven fix loop must be followed before the
task can be reported complete.

## Non-negotiable implementation rules

- Use package-by-feature Clean Architecture.
- Keep domain entities, errors, and ports independent from HTTP, Lambda, Chi, Fx, and DynamoDB.
- Keep DTOs and DynamoDB models in their own layers and map between every layer.
- Services use domain entities, never HTTP DTOs or persistence models.
- Use interface-returning constructors and Uber Fx composition; do not introduce manual DI.
- This is a hard gate: `go.mod` must declare `go.uber.org/fx`, and every executable composition root touched by the task must use `fx.New` and `fx.Provide`. Manual construction of the dependency graph in `main` or `run` fails review and must be migrated before feature work is considered complete.
- Keep each Lambda composition root in one `func main`: register configuration, AWS clients, adapters, repositories, services, handlers, and routers directly with `fx.Provide`; populate the HTTP handler; start/stop Fx; then call `lambda.Start(algnhsa.New(...))`. Do not replace this documented shape with `http.ListenAndServe`, helper composition functions, or module-only wiring. Verify the gate with repository search for `fx.New`, `fx.Provide`, and `lambda.Start`, plus `go test -race ./...` and `go vet ./...`.
- Consume every error. Never use `_ , data := foo()` or discard an error.
- Preserve the documented authentication, validation, cursor, feed-ad, status, error, logging, testing, Terraform, SSM, and CI/CD contracts.
- Do not implement deferred product phases as part of an MVP task.

## Local infrastructure

Start manual local persistence with `infra/local/docker-compose.yaml`. Host processes use the published local DynamoDB endpoint; containers attached to `social-media-local` use `http://dynamodb-local:8000`. Do not make integration tests depend on this running service; Testcontainers owns test lifecycle and isolation.

## Completion

Use the acceptance criteria in the relevant references and endpoint task. Verify formatting, vet, race-enabled tests, the applicable 90% coverage threshold, and the relevant integration or E2E tests before reporting completion.
