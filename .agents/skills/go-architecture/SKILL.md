---
name: go-architecture
description: Preserve the project's package-by-feature Clean Architecture and framework-independent service boundaries.
---

# Go Architecture

The authoritative local copy is [`go-architecture.md`](../social-media-mvp/references/go-architecture.md).

## Package shape

Use package by feature consistently:

```text
internal/
├── auth/
│   ├── domain.go
│   ├── fx/
│   │   └── fx.go
│   ├── service.go
│   ├── handler.go
│   ├── router.go
│   └── dynamodb/
│       └── repository.go
├── user/
├── post/
├── ad/
└── feed/
```

Each feature owns its domain entities, domain errors, repository interfaces, service, handler, router, DTOs, mappers, and adapters. A domain model must never contain DynamoDB tags.

## Boundary rules

- `domain.go` contains domain entities, domain errors, and repository/service ports.
- DTOs are HTTP transport structs with JSON tags.
- Models are persistence structs with DynamoDB tags.
- Mappers translate DTO ↔ domain and domain ↔ model.
- Services accept and return domain entities, never HTTP DTOs or DynamoDB models.
- Repositories are interfaces in the feature boundary; DynamoDB implementations live under the feature's `dynamodb` package.
- Constructors return interfaces where the dependency is consumed, for example `func NewUserRepo(...) UserRepo`.
- Use Uber Fx for composition. Do not add manual composition roots to Lambda entry points.
- Keep constructors in feature/platform packages, but wire the documented Lambda graph directly in each command entry point.
- Keep each Lambda `cmd/*/main.go` as one `func main`: direct `fx.Provide` registrations, `fx.Populate`, Fx lifecycle, and `lambda.Start(algnhsa.New(...))`. Never use `http.ListenAndServe` for API entry points.
- Keep framework and database dependencies out of domain packages.

## Entry points

Use separate API-group entry points under `cmd/`: `auth`, `users`, `posts`, `ads`, and `feed`. Each entry point builds its feature graph through Fx and adapts the Chi router with `algnhsa`.

## Configuration

Use typed Go configuration loaded from environment variables. Environment variables contain configuration and SSM parameter names; secrets are loaded from SSM SecureString during cold-start initialization and cached for the warm invocation lifetime.
