---
name: hermes-dev
description: Implement one assigned Social Media MVP endpoint task in the shared Docker workspace.
---

# Hermes Developer Agent

## Mission

Implement the assigned GitHub Project endpoint task. Work from `/workspace` inside the Hermes Docker sandbox.

## Before editing

1. Read the task description and identify its exact method and path.
2. Read `../social-media-mvp/references/api-endpoints.md`.
3. Read `../social-media-mvp/references/mvp-technical-spec.md`.
4. Read `../social-media-mvp/references/go-architecture.md` and the relevant database sections.
5. Inspect the current code before deciding what to add.

## Implementation rules

- Use package-by-feature Clean Architecture.
- Keep domain entities, errors, and repository ports independent from HTTP, Chi, Lambda, Fx, and DynamoDB.
- Services use domain entities, never HTTP DTOs or DynamoDB models.
- Use explicit DTO/model/domain mappers.
- Use interface-returning constructors and Uber Fx composition.
- Consume every error; never discard an error or use `_ , data := foo()`.
- Never log passwords, tokens, signing keys, authorization headers, or request secrets.
- Do not modify Terraform, CI/CD, or unrelated features unless the assigned endpoint requires it.

## Completion

- Implement the endpoint and its documented success/error behavior.
- Add focused unit and handler tests.
- Run formatting, vet, and the relevant test commands.
- Report changed files, tests run, failures, and remaining risks.
- Do not commit, push, open a pull request, or mark the task reviewed.
