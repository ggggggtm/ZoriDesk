#!/usr/bin/env bash
# Q11 account-free startup/assembly regressions, not installed-app or model E2E.
set -euo pipefail

race_args=()
case "${1:-}" in
  "") ;;
  --race) race_args=(-race); shift ;;
  *) echo "usage: bash scripts/verify-startup.sh [--race]" >&2; exit 2 ;;
esac
if [[ $# -ne 0 ]]; then
  echo "usage: bash scripts/verify-startup.sh [--race]" >&2
  exit 2
fi

repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repo_root"
export GOTOOLCHAIN=local GOWORK=off GOFLAGS=-mod=readonly GOPROXY=off
go version

packages=(./cmd/loomd ./internal/app ./internal/api ./internal/runtime/piadapter ./internal/runtime/discoveryscan)
# Keep formatting scoped to this slice; inherited unrelated files are not rewritten.
unformatted=$(gofmt -l \
  cmd/loomd/local_app_service.go cmd/loomd/run.go cmd/loomd/run_test.go \
  cmd/loomd/product_daemon.go cmd/loomd/product_daemon_test.go \
  cmd/loomd/product_composition_agent_runtime.go cmd/loomd/optional_runtime_test.go \
  internal/app/runtime_daemon.go internal/app/optional_runtime_test.go \
  internal/api/local_product_read.go internal/api/optional_runtime_health_test.go \
  internal/runtime/piadapter/process_runner_test.go)
if [[ -n "$unformatted" ]]; then
  printf 'Unformatted files (not modified):\n%s\n' "$unformatted" >&2
  exit 1
fi
go mod verify
go build "${packages[@]}"
go vet "${packages[@]}"

# Fixed, reviewed fixture-only selection: no user accounts, installed tools, model
# servers or opt-in live gates. The checker rejects skipped or empty packages.
tests='^(TestQ11.*|TestRun.*|TestDaemonBuildFailureNestedAmbiguityFailsClosed|TestExpandLocalAppServiceArgs.*|TestOptionalCredentialImportSourceFailsClosedOnUnsafePath|TestPrepareLocalAppInvocation.*|TestParseLocalAppServiceInvocation.*|TestMissionExecutionConfigFromDaemonBuild.*|TestLocalRuntimeObservationDaemon.*|TestNextRuntimeObservationSequence|TestPiLocalRuntimeProbeFactory.*|TestPiMetadataProcessRunnerSuccessIsolationAndCleanup|TestPiMetadataProcessRunnerMaterializesBoundLocalCatalog|TestDiscoverConfiguredRuntimes.*|TestProductSetupRemainsAvailableWhenNoRuntimeCanBuildATeam|TestProductDaemonColdStartAdmitsAllNativeProviderRuntimes|TestContainableProductObserverTimeoutRejectsMixedFailureChains|TestProductDaemonContainsExactPiMetadataTimeoutWithoutStoppingIPC|TestObserverFailureReasonIsClosedTypedAndNonDisclosing)$'
report=$(mktemp "${TMPDIR:-/tmp}/zoridesk-startup-tests.XXXXXX")
trap 'rm -f -- "$report"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
go test ${race_args[@]+"${race_args[@]}"} -json -run "$tests" -count=1 -timeout=180s "${packages[@]}" | tee "$report"
module=$(go list -m)
go run ./internal/testreport \
  "$module/cmd/loomd" "$module/internal/app" "$module/internal/api" \
  "$module/internal/runtime/piadapter" "$module/internal/runtime/discoveryscan" < "$report"
echo "PASS: bounded startup/assembly regressions only; not installed app or model E2E."
