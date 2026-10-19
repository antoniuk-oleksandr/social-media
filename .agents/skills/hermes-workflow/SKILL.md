---
name: hermes-workflow
description: Orchestrate the Social Media MVP Dev → Tester → Reviewer workflow in one shared Docker project workspace.
---

# Hermes Three-Agent Workflow

## Workspace

- Run Hermes using the Compose `hermes` service; no host Hermes installation is required.
- The Hermes container mounts the repository at `/workspace`.
- DynamoDB Local is reachable at `http://dynamodb-local:8000` through the `social-media-local` network.
- Trust the repository so project-local skills are discoverable.

## Stage order

Run stages sequentially for each GitHub Project endpoint task:

1. `hermes-dev` implements the task.
2. `hermes-tester` validates the same workspace.
3. `hermes-reviewer` performs a read-only review.

Do not run developer and reviewer agents concurrently against the same workspace. Delegated agents share the parent Docker container by default, so concurrent edits can collide.

## Delegation context

Every child receives explicit context because delegated agents start with a fresh conversation. Include:

- GitHub issue number and exact endpoint.
- Expected scope and acceptance criteria.
- Workspace path `/workspace`.
- Which previous stage completed and its test results.
- The instruction to avoid commits, pushes, and pull requests until the human reviews the work.

## Human review boundary

Planning starts through Slack with `/plan`. Plan Mode must be enabled so
mutating tools are blocked while Hermes reads the GitHub task and repository
documentation. The human reviews the complete plan and approves it explicitly
before implementation begins.

Hermes may edit and test the working tree, but it must stop before commit or pull request creation. The human changes the project item to the custom review status only after reviewing the diff and reports.
