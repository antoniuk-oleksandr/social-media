set shell := ["zsh", "-cu"]

compose := "docker compose -f infra/local/docker-compose.yaml"
hermes_compose := compose + " --profile hermes"
go_local := "GOTOOLCHAIN=auto"

default:
    @just --list

local-up:
    {{compose}} up -d dynamodb-local otel-collector tempo loki alloy grafana
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 DYNAMODB_ENDPOINT=http://127.0.0.1:8000 {{go_local}} go run ./cmd/local-init
    GOARCH=amd64 bash scripts/package_lambdas.sh
    if test -f infra/local/.sam-api.pid && kill -0 "$(sed -n '1p' infra/local/.sam-api.pid)" 2>/dev/null; then echo "SAM Local API already running on http://127.0.0.1:8080"; else nohup setsid sam local start-api --template-file infra/local/template.yaml --env-vars infra/local/env.json --docker-network social-media-local --host 127.0.0.1 --port 8080 </dev/null > /tmp/social-media-local-api.log 2>&1 & echo $! > infra/local/.sam-api.pid; fi
    for attempt in {1..30}; do if curl -s --max-time 1 http://127.0.0.1:8080/ >/dev/null; then echo "Local API available at http://127.0.0.1:8080"; break; fi; sleep 1; done

local-observability:
    {{compose}} up -d otel-collector tempo loki alloy grafana

local-init:
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 DYNAMODB_ENDPOINT=http://127.0.0.1:8000 {{go_local}} go run ./cmd/local-init

auth-build:
    mkdir -p build/auth
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 {{go_local}} go build -trimpath -ldflags='-s -w' -o build/auth/bootstrap ./cmd/auth

users-build:
    mkdir -p build/users
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 {{go_local}} go build -trimpath -ldflags='-s -w' -o build/users/bootstrap ./cmd/users

posts-build:
    mkdir -p build/posts
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 {{go_local}} go build -trimpath -ldflags='-s -w' -o build/posts/bootstrap ./cmd/posts

ads-build:
    mkdir -p build/ads
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 {{go_local}} go build -trimpath -ldflags='-s -w' -o build/ads/bootstrap ./cmd/ads

feed-build:
    mkdir -p build/feed
    AWS_ACCESS_KEY_ID=dummy AWS_SECRET_ACCESS_KEY=dummy AWS_REGION=us-east-1 CGO_ENABLED=0 GOOS=linux GOARCH=amd64 {{go_local}} go build -trimpath -ldflags='-s -w' -o build/feed/bootstrap ./cmd/feed

lambda-package:
    bash scripts/package_lambdas.sh

build: lambda-package

auth-request method path body headers='{"content-type":"application/json"}': local-up local-init auth-build
    python3 infra/local/invoke.py '{{method}}' '{{path}}' '{{body}}' '{{headers}}'

users-request method path body='' headers='{"content-type":"application/json"}': local-up local-init users-build
    SAM_FUNCTION=UsersFunction python3 infra/local/invoke.py '{{method}}' '{{path}}' '{{body}}' '{{headers}}'

posts-request method path body='' headers='{"content-type":"application/json"}': local-up local-init posts-build
    SAM_FUNCTION=PostsFunction python3 infra/local/invoke.py '{{method}}' '{{path}}' '{{body}}' '{{headers}}'

ads-request method path body='' headers='{"content-type":"application/json"}': local-up local-init ads-build
    SAM_FUNCTION=AdsFunction python3 infra/local/invoke.py '{{method}}' '{{path}}' '{{body}}' '{{headers}}'

feed-request method path body='' headers='{"content-type":"application/json"}': local-up local-init feed-build
    SAM_FUNCTION=FeedFunction python3 infra/local/invoke.py '{{method}}' '{{path}}' '{{body}}' '{{headers}}'

terraform-bootstrap-dev:
    set -a; source infra/dev/.env; set +a; terraform -chdir=infra/dev/bootstrap init -input=false
    set -a; source infra/dev/.env; set +a; terraform -chdir=infra/dev/bootstrap apply -auto-approve -input=false

terraform-init-dev: terraform-bootstrap-dev
    test -f infra/dev/backend.hcl
    set -a; source infra/dev/.env; set +a; terraform -chdir=infra/dev init -migrate-state -input=false -backend-config=backend.hcl

terraform-deploy-dev: build terraform-init-dev
    test -f infra/dev/.env
    set -a; source infra/dev/.env; set +a; terraform -chdir=infra/dev apply

terraform-destroy-dev: terraform-init-dev
    set -a; source infra/dev/.env; set +a; terraform -chdir=infra/dev destroy -auto-approve -input=false
    set -a; source infra/dev/.env; set +a; terraform -chdir=infra/dev/bootstrap destroy -auto-approve -input=false

local-down:
    if test -f infra/local/.sam-api.pid; then kill "$(sed -n '1p' infra/local/.sam-api.pid)" 2>/dev/null || true; rm -f infra/local/.sam-api.pid; fi
    {{compose}} down

coverage:
    go test ./... -coverprofile=coverage.out
    bash scripts/check_coverage.sh coverage.out

integration:
    GOTOOLCHAIN=auto go test -v -count=1 -race -p 1 -tags=integration ./tests/integration/...

e2e: auth-build users-build posts-build ads-build feed-build
    test -x "$(pwd)/build/auth/bootstrap"
    test -x "$(pwd)/build/users/bootstrap"
    test -x "$(pwd)/build/posts/bootstrap"
    test -x "$(pwd)/build/ads/bootstrap"
    test -x "$(pwd)/build/feed/bootstrap"
    GOTOOLCHAIN=auto go test -v -count=1 -tags=e2e ./tests/e2e/...

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
    {{hermes_compose}} run --rm --build hermes /opt/hermes/bin/hermes skills trust /workspace

hermes-up:
    {{hermes_compose}} up -d --build hermes

hermes-down:
    {{hermes_compose}} stop hermes

hermes-chat:
    {{hermes_compose}} run --rm hermes chat

hermes-logs:
    {{hermes_compose}} logs -f hermes
