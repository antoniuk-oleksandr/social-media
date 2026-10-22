# Repository agent rules

This repository's project instructions live under `.agents/skills/`. Before changing Go code, read `.agents/skills/social-media-mvp/SKILL.md` and the relevant skill it routes to. The copies under `docs/` are supporting documentation; the repository-owned `.agents/` references are the implementation contract.

## Non-negotiable Go composition rule

- Use `go.uber.org/fx` for application composition.
- Every executable entry point under `cmd/` must construct its graph with `fx.New` and register dependencies with `fx.Provide` (or an explicitly justified Fx module).
- Do not manually instantiate repositories, services, handlers, routers, or infrastructure in `main`, `run`, or another composition root.
- Feature packages may expose ordinary Go constructors and must remain testable without calling `fx.New` themselves.
- If the current task touches an entry point and Fx is absent, add the dependency and migrate the composition root before implementing the feature.

## Mandatory pre-completion gate

Before reporting completion, verify all of the following:

1. `go.mod` declares `go.uber.org/fx`.
2. The changed composition root contains `fx.New` and `fx.Provide`.
3. `go test -race ./...` and `go vet ./...` pass, or failures are reported without claiming completion.
4. The diff contains no new manual dependency graph in an entry point.

Treat a missing Fx graph as a blocker, not as a stylistic suggestion.

## Build artifacts

- Generated application and Lambda build artifacts must live under `/build`.
- Do not create generated binaries under the repository root or under
  `infra/local`.
- Local Lambda builds use `build/auth/bootstrap`; use `just auth-run` or
  `just auth-request` for SAM-based invocation.
