#!/usr/bin/env bash
# Focused state, diagnostic-export and isolated-brand regression, not GUI E2E.
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
  testRoundtableUnknownErrorUsesSafeRecoveryMessage
  testRoundtableLocalizedErrorDescriptionIsNotEvaluated
  testRoundtableClientErrorsUseSafeRecoveryMessage
  testRoundtableRemoteErrorKeepsSafeMessageAndStage
)
filter=$(IFS='|'; echo "${tests[*]}")
diagnostic_tests=(
  testRecoveryStagesSurviveAppPreviewAndExport
  testRecoveryStagesSurviveDaemonPreviewAndExport
  testRecoveryStageExpansionKeepsInvalidRecordsExcluded
  testCredentialStagesSurviveAppPreviewAndExport
  testCredentialStagesSurviveDaemonPreviewAndExport
  testCredentialStageExpansionKeepsInvalidRecordsExcluded
  testPreviewAndExportContainOnlyAllowlistedSupportData
  testGatewayV3RoundTripPreservesPrivacySafeAuthority
  testDiagnosticExportRetainsBoundedGatewayWindowForInstalledAcceptance
  testGatewayV3RejectsInvalidAuthorityAndForbiddenFields
  testGatewayV3RejectsUnknownFieldsBeforeDecoding
  testGatewayLegacyRecordsAllowUnknownFieldsButRejectNestedContent
  testGatewayV3PreservesNativeAuthorityExactly
  testGatewayV3RejectsMixedNativeAndBrokeredAuthority
  testGatewayV3PreservesBrokeredAbsentPolicyAuthority
  testGatewayV1RecordRemainsCompatible
  testGatewayV2RecordRemainsCompatibleWithoutInstanceIdentity
  testUnsafeDiagnosticFileIsExcludedInsteadOfRead
)
diagnostic_filter=$(IFS='|'; echo "${diagnostic_tests[*]}")
brand_tests=(
  testWorkspaceBrandCopyIdentifiesZoriDesk
  testWorkspaceBrandWiringPreservesCompatibilityIdentifiers
  testWorkspaceBrandHeaderRendersWideAndCompact
)
brand_filter=$(IFS='|'; echo "${brand_tests[*]}")
chat_state_tests=(
  testChatBrandCopyUsesProductName
  testChatMessagePresentationPreservesRolesAndContent
  testChatTranscriptPreservesUnicodeAndWireRoles
  testConversationTranscriptTextBuildsReadableCopyableThread
  testConversationComposerActionBecomesAccessibleStopWhileResponding
)
chat_state_filter=$(IFS='|'; echo "${chat_state_tests[*]}")
chat_view_tests=(
  testChatBrandWiringPreservesComposerAndCopyInteraction
  testWorkspaceShellConsumesAgentTeamBuilderSession
)
chat_view_filter=$(IFS='|'; echo "${chat_view_tests[*]}")
connection_state_tests=(
  testConnectionBrandCopyUsesProductName
  testConnectionBrandingPreservesStatesRefreshAndSnapshots
  testConnectionCopyDescribesLocalServiceWithoutTransportReasons
)
connection_state_filter=$(IFS='|'; echo "${connection_state_tests[*]}")
connection_view_tests=(
  testMissionConnectionBrandWiringPreservesRefreshAndRecovery
  testContentViewStartsSetupRecoveryAlongsideInitialRefreshes
)
connection_view_filter=$(IFS='|'; echo "${connection_view_tests[*]}")
report=$(mktemp "${TMPDIR:-/tmp}/zoridesk-startup-ui.XXXXXX")
trap 'rm -f -- "$report"' EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
swift --version
# Package.swift has no remote dependencies. Do not install tools or resolve updates.
swift test --package-path apps/macos --disable-automatic-resolution --jobs 2 \
  --filter "LocalProductStoreTests/($filter)$|LocalDiagnosticBundleTests/($diagnostic_filter)$|LoomGraphiteViewTests/($brand_filter|$chat_view_filter|$connection_view_filter)$|LoomWorkspaceShellStateTests/($chat_state_filter)$|LocalProductExperienceTests/($connection_state_filter)$" 2>&1 | tee "$report"
if grep -Ei 'Test Case.*(skipped|failed)' "$report"; then
  echo "FAIL: skipped or failed XCTest case" >&2
  exit 1
fi
for test_name in "${tests[@]}" "${diagnostic_tests[@]}" "${brand_tests[@]}" "${chat_state_tests[@]}" "${chat_view_tests[@]}" "${connection_state_tests[@]}" "${connection_view_tests[@]}"; do
  if ! grep -F "$test_name" "$report" | grep -F 'passed (' >/dev/null; then
    echo "FAIL: missing passing result for $test_name" >&2
    exit 1
  fi
done
echo "Verified all $((${#tests[@]} + ${#diagnostic_tests[@]} + ${#brand_tests[@]} + ${#chat_state_tests[@]} + ${#chat_view_tests[@]} + ${#connection_state_tests[@]} + ${#connection_view_tests[@]})) selected XCTest cases passed; no skips."
