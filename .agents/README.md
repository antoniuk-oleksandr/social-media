# Hermes Agent Skills

These skills guide Hermes while implementing the Social Media MVP. The `.agents/skills/social-media-mvp/references/` directory contains exact repository-owned copies of every document that was used to define the project. Hermes can therefore operate from `.agents/` alone.

## Skills

- `mvp-contract`: authentication, validation, feed behavior, errors, and MVP boundaries.
- `endpoint-implementation`: how to implement the endpoint tasks from the API catalog.
- `go-architecture`: Clean Architecture, package-by-feature, interfaces, DTOs, mappers, and Fx composition.
- `dynamodb`: table schemas, keys, transactions, pagination, and repository error translation.
- `quality-and-delivery`: tests, coverage, Zap logging, Terraform, SSM, and GitHub Actions.
- `social-media-mvp`: complete self-contained skill with routed references for all product and technical documentation.
- `hermes-dev`: implementation role for one assigned endpoint task.
- `hermes-tester`: validation role for the shared workspace.
- `hermes-reviewer`: read-only review role.
- `hermes-workflow`: sequential Dev → Tester → Reviewer orchestration rules.

The Hermes Docker configuration is at `.agents/hermes/config.yaml`, and the self-contained operating procedure is at `skills/social-media-mvp/references/hermes.md`.

Hermes should trust this repository and read the relevant skill before changing code. For a complete feature task, use `social-media-mvp` and load the relevant files under its `references/` directory.
