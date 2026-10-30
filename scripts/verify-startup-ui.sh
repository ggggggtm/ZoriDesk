#!/usr/bin/env bash
# Focused snapshot-state regression, not an installed application or GUI E2E.
set -euo pipefail
if [[ $# -ne 0 ]]; then
  echo "usage: bash scripts/verify-startup-ui.sh" >&2
  exit 2
fi
repo_root=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")/.." && pwd -P)
cd "$repo_root"
tests=(
  testQ11PiFailureRemainsDegradedWithPagination
  testRunHistoryPageOverflowNormalizesToOnline
  testEvidenceHistoryPageOverflowNormalizesToOnline
  testNavigationPaginationKeepsHealthyConnectionState
  testObserverModelsTimeoutKeepsDegradedPartialState
  testObserverVersionTimeoutKeepsDegradedPartialState
)
filter=$(IFS='|'; echo "${tests[*]}")
report=$(mktemp "${TMPDIR:-/tmp}/zoridesk-startup-ui.XXXXXX")
trap 'rm -f -- "$report"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
swift --version
# Package.swift has no remote dependencies. Do not install tools or resolve updates.
swift test --package-path apps/macos --disable-automatic-resolution --jobs 2 \
  --filter "LocalProductStoreTests/($filter)$" 2>&1 | tee "$report"
if grep -Ei 'Test Case.*(skipped|failed)' "$report"; then
  echo "FAIL: skipped or failed XCTest case" >&2
  exit 1
fi
for test_name in "${tests[@]}"; do
  if ! grep -F "$test_name" "$report" | grep -F 'passed (' >/dev/null; then
    echo "FAIL: missing passing result for $test_name" >&2
    exit 1
  fi
done
echo "Verified all 6 selected XCTest cases passed; no skips."
