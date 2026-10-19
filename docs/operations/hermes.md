# Hermes Local Agent Workflow

Hermes itself runs in the Compose `hermes` container. Its terminal, file, and delegated-agent work run inside that container. The repository is mounted at `/workspace`, and the Hermes container joins the `social-media-local` Docker network so it can reach DynamoDB Local.

## Start local infrastructure

From the repository root:

```bash
just local-up
```

DynamoDB endpoints:

```text
Host process:    http://127.0.0.1:8000
Hermes sandbox:  http://dynamodb-local:8000
```

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
- allows up to three delegated children.

## Slack and GitHub Project planning

Hermes uses Slack Socket Mode for messaging and GitHub MCP for reading GitHub
Projects and repository content. Socket Mode does not require a public Hermes
endpoint. The GitHub MCP server is configured in the tracked project config,
but its token is resolved from the private `/opt/data/.env` file.

Create the private credentials file on the host:

```bash
mkdir -p "$HOME/.hermes"
touch "$HOME/.hermes/.env"
chmod 600 "$HOME/.hermes/.env"
${EDITOR:-vi} "$HOME/.hermes/.env"
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

## Agent roles

- `hermes-dev`: implements one endpoint task and does not commit or open PRs.
- `hermes-tester`: runs validation, unit tests, Testcontainers integration tests, and coverage without silently changing product code.
- `hermes-reviewer`: performs a read-only contract and architecture review.

Run them sequentially against the same workspace:

```text
developer → tester → reviewer → human review
```

Do not run multiple editing agents concurrently. Delegated children share the persistent Docker workspace by default, so simultaneous edits can conflict.

## Test isolation

Compose DynamoDB Local is for manual development. Integration tests must create and own isolated Testcontainers and tables; they must not rely on a developer-managed Compose service.

## Security boundaries

Do not mount the Docker socket, host home directory, SSH keys, AWS credentials, browser profiles, or password stores. Only the repository and `$HOME/.hermes` data directory are mounted. Provider credentials are entered into the Hermes setup flow and remain under `/opt/data`.
