#!/usr/bin/env bash
# Bounded, account-free Q05 baseline. No application startup or installation.
set -euo pipefail

race_args=()
case "${1:-}" in
  "") ;;
  --race) race_args=(-race); shift ;;
  *) echo "usage: bash scripts/verify-sandbox.sh [--race]" >&2; exit 2 ;;
esac
if [[ $# -ne 0 ]]; then
  echo "usage: bash scripts/verify-sandbox.sh [--race]" >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repo_root"

# Do not inherit flags that silently filter tests, use a workspace or fetch a toolchain.
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-mod=readonly
export GOPROXY=off
go version

unformatted=$(gofmt -l internal/sandbox/*.go internal/testreport/*.go)
if [[ -n "$unformatted" ]]; then
  printf 'Unformatted files (not modified):\n%s\n' "$unformatted" >&2
  exit 1
fi

go mod verify
go build ./internal/sandbox
go vet ./internal/sandbox ./internal/testreport

# The temporary result contains only this account-free suite, not application logs.
report=$(mktemp "${TMPDIR:-/tmp}/zoridesk-tests.XXXXXX")
trap 'rm -f -- "$report"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM

go test ${race_args[@]+"${race_args[@]}"} -json -count=1 -timeout=90s \
  ./internal/sandbox ./internal/testreport | tee "$report"

module=$(go list -m)
go run ./internal/testreport \
  "$module/internal/sandbox" "$module/internal/testreport" < "$report"

echo "PASS: bounded Go sandbox baseline only; not app build, Docker or model E2E."
