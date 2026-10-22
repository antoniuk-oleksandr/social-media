---
name: endpoint-implementation
description: Implement one MVP HTTP endpoint task using the documented layered, package-by-feature architecture.
---

# Endpoint Implementation Workflow

Read [`api-endpoints.md`](../social-media-mvp/references/api-endpoints.md) for the selected endpoint and [`mvp-technical-spec.md`](../social-media-mvp/references/mvp-technical-spec.md) for cross-cutting rules.

## Required flow

1. Confirm the HTTP method, path, authentication, request, response, status codes, and error codes in the endpoint catalog.
2. Identify the feature package: `auth`, `user`, `post`, `ad`, or `feed`.
3. Define or reuse domain entities, value rules, errors, and repository ports in the feature's domain file.
4. Define HTTP DTOs with JSON tags. Do not pass DTOs into services.
5. Define DynamoDB persistence models with DynamoDB tags. Do not pass models into services.
6. Add mappers between DTOs, domain entities, and persistence models.
7. Implement business rules in the service using domain entities and interfaces.
8. Implement the DynamoDB adapter behind the repository interface and translate database errors into domain errors.
9. Implement the handler with strict JSON decoding, request validation, authentication, request IDs, and explicit domain-error mapping.
10. Register the route in the feature router and the correct Lambda entry point.
11. Add unit, integration, and endpoint tests appropriate to the behavior.
12. Enforce the Fx composition gate: the entry point must use `fx.New` plus `fx.Provide`; do not hand-wire adapters, services, handlers, or routers. If the existing entry point is manual, migrate it as part of the task before adding endpoint behavior.

## Handler rules

- Accept exactly one JSON object and reject malformed JSON, duplicate keys, unknown fields, trailing values, and oversized bodies.
- Consume every returned error. Never use `_ , data := foo()` or discard an error.
- Keep error responses in the documented `{ "error": { "code", "message", "requestId" } }` shape.
- Services must not import HTTP, Chi, Lambda, DynamoDB SDK types, or DTO packages.

## Completion checklist

- The task's endpoint contract is implemented without changing undocumented behavior.
- Success and failure status codes match the catalog.
- Authorization and ownership rules are tested.
- The service uses domain entities and repository ports.
- Mappers cover every boundary.
- Logs contain useful identifiers but no secrets.
- `go test -race ./...`, formatting, vet, and the relevant coverage checks pass.
- `go.mod` contains `go.uber.org/fx`, and repository search confirms every touched composition root uses `fx.New` and `fx.Provide`.
