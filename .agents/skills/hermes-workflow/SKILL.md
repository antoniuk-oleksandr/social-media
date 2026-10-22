---
name: hermes-workflow
description: Orchestrate the Social Media MVP Dev → Tester → Reviewer workflow in one shared Docker project workspace, including delegated review fixes.
---

# Hermes Three-Agent Workflow

## Workspace

- Run Hermes using the Compose `hermes` service; no host Hermes installation is required.
- The Hermes container mounts the repository at `/workspace`.
- DynamoDB Local is reachable at `http://dynamodb-local:8000` through the `social-media-local` network.
- Trust the repository so project-local skills are discoverable.

## Mandatory orchestration

For every implementation task, the parent Hermes session is an orchestrator. It
must not implement the task itself when delegation is available. It must call
delegated child agents in this order:

1. `hermes-dev` implements the task and the production code.
2. `hermes-tester` inspects the implementation and adds or updates the
   required tests, then runs the validation suite.
3. `hermes-reviewer` reviews both the generated production code and the
   generated tests against the task, `docs/`, and every applicable `.agents`
   rule.

The stages are sequential and operate on the same `/workspace`. Do not run
editing children concurrently because delegated agents share the workspace.

The parent must record and report each child delegation, including the role,
child/session identifier, result, and tests run. A parent-only implementation,
or a report that merely claims the three roles without delegation evidence, is
non-compliant.

## Review-fix loop

The reviewer is the quality gate. It reviews the complete diff, including tests;
it does not silently patch the files itself. When it finds a problem, it must
delegate the fix to the correct role:

- production-code, architecture, contract, or implementation defects go to
  `hermes-dev`;
- missing, incorrect, flaky, or insufficient tests go to `hermes-tester`.

If both production code and tests need changes, delegate `hermes-dev` first,
then `hermes-tester` after the code fix is complete. After every fix child
finishes, delegate `hermes-reviewer` again against the new diff. The loop ends
only when the reviewer reports `APPROVED`, or when it reports a blocker that
requires the human. Never report completion while blocker or high-severity
findings remain.

Use at most three review-fix cycles for one task. If the task still fails after
three cycles, stop and ask the human for direction; do not repeatedly mutate
the workspace.

The reviewer must explicitly classify every finding as one of:
`production_fix`, `test_fix`, `both`, or `human_blocker`. This classification
determines the next delegation and prevents test failures from being “fixed” by
weakening or deleting tests.

## Delegation context

Every child receives explicit context because delegated agents start with a fresh conversation. Include:

- GitHub issue number and exact endpoint.
- Expected scope and acceptance criteria.
- Workspace path `/workspace`.
- Which previous stage completed and its test results.
- The current workflow cycle and the review findings assigned to that child.
- The exact instruction that the child may modify only its role's files: Dev may
  change production code and focused tests when necessary for implementation;
  Tester may add or correct tests but must not alter production behavior to make
  tests pass; Reviewer is read-only and may only issue findings.
- The instruction to avoid commits, pushes, and pull requests until the human reviews the work.

## Human review boundary

Planning starts through Slack with `/plan`. Plan Mode must be enabled so
mutating tools are blocked while Hermes reads the GitHub task and repository
documentation. The human reviews the complete plan and approves it explicitly
before implementation begins.

Hermes may edit and test the working tree through delegated children, but it
must stop before commit or pull request creation. The human changes the project
item to the custom review status only after reviewing the final diff and the
reviewer report.

## Slack completion notifications

When reporting that a task, test run, review, or delegated stage is complete,
mention the requesting Slack user using Slack's native mention syntax
(``<@USER_ID>``) when the sender's member ID is available in the conversation.
Use the same mention when reporting a blocker that requires the human's input.
Do not invent a member ID or write a raw `@name` mention when the ID is
unknown.
