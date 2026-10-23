# Hermes Agent Skills

These skills guide Hermes while implementing the Social Media MVP. The `.agents/skills/social-media-mvp/references/` directory contains exact repository-owned copies of every document that was used to define the project. Hermes can therefore operate from `.agents/` alone.

The root `AGENTS.md` is the discovery bridge for Hermes and other coding agents. `.agents/` is a repository skill library, not a filename Hermes auto-loads by itself; the bridge explicitly routes agents into these skills and makes the Fx composition gate mandatory.

## Skills

- `mvp-contract`: authentication, validation, feed behavior, errors, and MVP boundaries.
- `endpoint-implementation`: how to implement the endpoint tasks from the API catalog.
- `go-architecture`: Clean Architecture, package-by-feature, interfaces, DTOs, mappers, and Fx composition.
- `dynamodb`: table schemas, keys, transactions, pagination, and repository error translation.
- `quality-and-delivery`: tests, coverage, Zap logging, Terraform, SSM, and GitHub Actions.
- `social-media-mvp`: complete self-contained skill with routed references for all product and technical documentation.
- `hermes-dev`: implementation role for one assigned endpoint task.
- `hermes-tester`: adds or corrects tests and validates the shared workspace
  without changing production behavior.
- `hermes-reviewer`: read-only review and finding-classification role; fixes are
  delegated to Dev or Tester.
- `hermes-workflow`: mandatory sequential Dev → Tester → Reviewer orchestration
  and reviewer-driven fix loop.

The Hermes Docker configuration is at `.agents/hermes/config.yaml`, and the self-contained operating procedure is at `skills/social-media-mvp/references/hermes.md`.

Hermes should trust this repository and read the relevant skill before changing code. For a complete feature task, use `social-media-mvp` and load the relevant files under its `references/` directory.

Generated binaries and local Lambda artifacts belong under the repository
root's `build/` directory. The local SAM template uses `build/auth/` and the
supported commands are `just build` and `just auth-request`.
