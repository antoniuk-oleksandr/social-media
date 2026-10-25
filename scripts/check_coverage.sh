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
read -r covered statements < <(awk 'NR > 1 { statements += $2; if ($3 > 0) covered += $2 } END { print covered+0, statements+0 }' "$filtered")
[[ "$statements" -gt 0 ]] || { echo "coverage total missing" >&2; exit 2; }
awk -v covered="$covered" -v statements="$statements" -v minimum="$threshold" 'BEGIN {
  actual = covered / statements * 100
  if (actual+0 < minimum+0) {
    printf "coverage %d/%d = %.6f%% is below required %.6f%%\n", covered, statements, actual, minimum > "/dev/stderr"
    exit 1
  }
  printf "coverage %d/%d = %.6f%% meets required %.6f%%\n", covered, statements, actual, minimum
}'
