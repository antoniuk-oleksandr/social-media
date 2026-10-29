#!/usr/bin/env bash

set -euo pipefail

mkdir -p build

lambda_architecture="${GOARCH:-arm64}"

for command_dir in cmd/*; do
  [[ -d "$command_dir" ]] || continue

  service="${command_dir##*/}"
  [[ "$service" == "local-init" ]] && continue

  mkdir -p "build/$service"
  CGO_ENABLED=0 GOOS=linux GOARCH="$lambda_architecture" \
    go build -trimpath -tags=lambda.norpc -ldflags='-s -w' \
    -o "build/$service/bootstrap" "./$command_dir"

  (cd "build/$service" && zip -q -j "../$service-lambda.zip" bootstrap)
done

test -s build/auth-lambda.zip
test -s build/users-lambda.zip
