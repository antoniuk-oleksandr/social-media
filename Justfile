set shell := ["zsh", "-cu"]

compose := "docker compose -f infra/local/docker-compose.yaml"
hermes_compose := compose + " --profile hermes"

default:
    @just --list

local-up:
    {{compose}} up -d dynamodb-local

local-down:
    {{compose}} down

hermes-setup:
    mkdir -p "$HOME/.hermes"
    {{hermes_compose}} run --rm hermes setup

hermes-install-plan-mode:
    {{hermes_compose}} run --rm hermes plugins install 100yenadmin/hermes-plan-mode --ref 118a92a --enable

hermes-mcp-test:
    {{hermes_compose}} run --rm hermes mcp test github

hermes-slack-manifest:
    {{hermes_compose}} run --rm hermes slack manifest --agent-view --write

hermes-trust:
    {{hermes_compose}} run --rm hermes skills trust /workspace

hermes-up:
    {{hermes_compose}} up -d hermes

hermes-down:
    {{hermes_compose}} stop hermes

hermes-chat:
    {{hermes_compose}} run --rm hermes chat

hermes-logs:
    {{hermes_compose}} logs -f hermes
