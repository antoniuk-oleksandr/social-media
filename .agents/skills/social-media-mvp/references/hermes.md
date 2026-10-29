# Hermes Local Agent Workflow

Hermes itself runs in the Compose `hermes` container. Its terminal, file, and delegated-agent work run inside that container. The repository is mounted at `/workspace`, and the Hermes container joins the `social-media-local` Docker network so it can reach DynamoDB Local.

## Start local infrastructure

From the repository root:

```bash
just local-up
```

This starts the complete local infrastructure, including SAM Local API Gateway
at `http://127.0.0.1:8080`; for example,
`curl http://127.0.0.1:8080/api/v1/feed`.

DynamoDB endpoints:

```text
Host process:    the published local DynamoDB endpoint
Hermes sandbox:  http://dynamodb-local:8000
```

## Local Lambda invocation

Local Lambda build artifacts are written under the repository root's `build/`
directory. The auth bootstrap is generated at `build/auth/bootstrap`; no
generated binaries are written under `infra/local`.

Use the direct Lambda helpers for focused function tests:

```bash
just auth-request POST /api/v1/auth/register \
  '{"username":"alice","email":"alice@example.com","password":"secure-password","displayName":"Alice"}'
```

For an arbitrary API Gateway request, provide the method, path, body, and
headers:

```bash
just auth-request POST /api/v1/auth/register \
  '{"username":"alice","email":"alice@example.com","password":"secure-password","displayName":"Alice"}' \
  '{"content-type":"application/json"}'
```

Create an authenticated local post with the Posts Lambda:

```bash
just posts-request POST /api/v1/posts \
  '{"content":"Hello from local SAM"}' \
  '{"content-type":"application/json","authorization":"Bearer $ACCESS_TOKEN"}'
```

The SAM template is [`infra/local/template.yaml`](../../../../infra/local/template.yaml),
and the non-secret base environment is [`infra/local/env.json`](../../../../infra/local/env.json).
It defines Auth, Users, and Posts Lambda functions; `local-init` creates their
local DynamoDB tables before each request.
Local SAM invocations overlay values from the ignored `infra/local/.env` file.
Create it from `infra/local/.env.example` when setting up a new checkout. Keep
local signing keys in `.env`; never commit that file.


## Configure and run Hermes

Create the persistent Hermes data directory and run the setup wizard inside Docker:

```bash
just hermes-setup
```

Compose mounts [`.agents/hermes/config.yaml`](../../.agents/hermes/config.yaml) directly as `/opt/data/config.yaml`, so no host Hermes installation or manual config copy is required. Provider credentials and session data remain in the mounted `$HOME/.hermes` directory.

The configuration:

- uses the local terminal backend inside the Hermes container;
- mounts this repository at `/workspace`;
- joins `social-media-local`;
- keeps provider credentials in the persistent `/opt/data` mount; and
- allows up to three delegated children;
- enables `agent.coding_context: on` for Slack and CLI sessions; and
- injects the Dev → Tester → Reviewer workflow as persistent
  `agent.coding_instructions`, so it does not need to be repeated in every
  request;
- requires agent replies to be in English unless the user explicitly requests
  another language. Hermes `display.language` controls UI text only, so the
  reply-language rule is kept in `agent.coding_instructions`.
- requires persistence tasks to include and run Testcontainers integration
  tests; unit tests alone cannot receive approval. Missing test dependencies or
  files must be added. Docker is required only for executing the
  container-backed tests: when unavailable, Hermes must still add and
  compile-check the tests, run non-container checks, and report the integration
  execution as blocked.

Every Hermes startup automatically trusts `/workspace`, so the repository's
`.agents/skills/` are available to new sessions. You can also refresh trust
manually with `just hermes-trust`.

The workflow instructions are loaded into new sessions from
`.agents/hermes/config.yaml`. Restart the gateway after changing this file;
existing sessions retain their cached system prompt.

## Slack and GitHub Project planning

Hermes uses Slack Socket Mode for messaging and GitHub MCP for reading GitHub
Projects and repository content. Socket Mode does not require a public Hermes
endpoint. The GitHub MCP server is configured in the tracked project config,
but its token is resolved from the private `/opt/data/.env` file.

Create the private credentials file on the host:

Set `HERMES_DATA_DIR` to your private Hermes data directory before running the
commands below.

```bash
mkdir -p "$HERMES_DATA_DIR"
touch "$HERMES_DATA_DIR/.env"
chmod 600 "$HERMES_DATA_DIR/.env"
${EDITOR:-vi} "$HERMES_DATA_DIR/.env"
```

Add values like these, replacing placeholders locally:

```dotenv
SLACK_BOT_TOKEN=xoxb-replace-me
SLACK_APP_TOKEN=xapp-replace-me
SLACK_ALLOWED_USERS=U_REPLACE_WITH_YOUR_SLACK_MEMBER_ID
GITHUB_PERSONAL_ACCESS_TOKEN=ghp_replace-me
```

Do not send these values in chat, commit them, or put them in a repository
file. The GitHub token should have only the access required to read the
repository and GitHub Project. Project mutations should remain disabled until
you explicitly decide to enable them.

Install the pinned Plan Mode plugin so `/plan` is enforced as read-only until
you approve the generated plan:

```bash
just hermes-install-plan-mode
```

Restart the gateway after installing or updating the plugin.

Generate the Slack app manifest from inside the container:

```bash
just hermes-slack-manifest
```

Paste the generated manifest into the Slack app configuration, enable Socket
Mode, install the app, and invite the bot to the chosen channel. Hermes will
then connect to Slack through the gateway.

Use the planning workflow:

```text
@Hermes /plan Read GitHub Project task #123 and the repository documentation.
Create a detailed implementation plan only. Do not edit files, commit, open a
pull request, or modify GitHub Project items.
```

Review the plan in Slack. Ask Hermes to revise it with ordinary text. Approve
only the final unchanged plan with `/planmode approve`. Approval is the point
at which implementation may begin; the Dev → Tester → Reviewer workflow then
runs against the mounted `/workspace`.

The first planning phase is intentionally read-only. Do not grant write-capable
GitHub Project permissions or ask Hermes to create/update project items until
this review workflow has been proven.

Trust the repository from inside the container:

```bash
just hermes-trust
```

Run interactive Hermes:

```bash
just hermes-chat
```

Run the persistent gateway instead:

```bash
just hermes-up
just hermes-logs
```

## Agent roles and delegated review loop

- `hermes-dev`: implements one endpoint task and does not commit or open PRs.
- `hermes-tester`: adds or corrects tests, then runs validation, unit tests,
  Testcontainers integration tests, and coverage; it does not change production
  behavior.
- `hermes-reviewer`: performs a read-only contract and architecture review of
  both production code and tests, classifies findings, and routes fixes to Dev
  or Tester.

For every public endpoint, the workflow must verify the complete deployment
path: feature/Chi route, Lambda entry point, and Terraform
`aws_apigatewayv2_route` in `infra/dev/api_gateway.tf`. A direct Lambda or SAM
route is not enough; the endpoint is incomplete until API Gateway targets the
correct Lambda integration and Terraform formatting, validation, and plan
checks pass.

For every implementation task, the parent Hermes session must delegate these
stages sequentially against the same workspace:

```text
developer → tester → reviewer
                         ↓ findings
                 dev and/or tester → reviewer
                         ↓ APPROVED
                    human review
```

The reviewer never edits the workspace itself. Production defects are delegated
to `hermes-dev`; missing or incorrect tests are delegated to `hermes-tester`.
When both are needed, Dev runs first and Tester runs after the code fix. The
reviewer is delegated again after every fix. Use at most three review-fix
cycles, then stop for human direction. The parent must report each child/session
ID and result; claiming a three-agent workflow without delegation evidence is
not completion.

Do not run multiple editing agents concurrently. Delegated children share the
persistent Docker workspace by default, so simultaneous edits can conflict.

## Test isolation

Compose DynamoDB Local is for manual development. Integration tests must create
and own isolated Testcontainers and tables; they must not rely on a
developer-managed Compose service. The repository uses one `tests/integration`
Go package, with exactly one DynamoDB Local Testcontainer shared by all specs.
Start it in `BeforeSuite`, terminate it in `AfterSuite`, and isolate specs with
table names or cleanup. Never create one container per test or split the
integration suite into multiple packages.

Keep deterministic data in `tests/fixtures/`. Keep Testcontainers lifecycle,
DynamoDB clients, table setup/reset, and shared assertions in
`tests/integration/support/`. Keep Resty client helpers in
`tests/e2e/support/`.

Run local public-route E2E tests with `just e2e`. The suite uses SAM Local and
one disposable DynamoDB Local Testcontainer for the whole run. E2E tests must
cover multi-request user journeys rather than individual endpoint checks.
Extend the relevant authentication, profile, post/feed, administrator ad, or
mixed-feed flow when a feature changes that journey.

## Security boundaries

Do not mount the Docker socket, host home directory, SSH keys, AWS credentials, browser profiles, or password stores. Only the repository and `$HOME/.hermes` data directory are mounted. Provider credentials are entered into the Hermes setup flow and remain under `/opt/data`.
