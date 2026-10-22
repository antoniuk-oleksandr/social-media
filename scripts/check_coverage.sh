#!/usr/bin/env bash
set -euo pipefail
profile=${1:-coverage.out}
threshold=${2:-90.0}
[[ -s "$profile" ]] || { echo "coverage profile missing: $profile" >&2; exit 2; }
filtered=$(mktemp)
trap 'rm -f "$filtered"' EXIT
# The auth Lambda bootstrap is the only documented executable excluded from the
# statement threshold; local-init helpers are application code and must count.
awk 'NR == 1 || $1 !~ /^github.com\/antoniuk-oleksandr\/social-media\/cmd\/auth\/main.go:/' "$profile" > "$filtered"
total=$(go tool cover -func="$filtered" | awk '/^total:/ { gsub(/%/, "", $3); print $3 }')
[[ -n "$total" ]] || { echo "coverage total missing" >&2; exit 2; }
awk -v actual="$total" -v minimum="$threshold" 'BEGIN { if (actual+0 < minimum+0) { printf "coverage %.1f%% is below required %.1f%%\n", actual, minimum > "/dev/stderr"; exit 1 }; printf "coverage %.1f%% meets required %.1f%%\n", actual, minimum }'
