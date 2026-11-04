import Foundation
import XCTest
@testable import LoomLocalAppCore

final class LocalProductExperienceTests: XCTestCase {
    func testConnectionBrandCopyUsesProductName() {
        let cases: [(LocalProductConnectionState, String)] = [
            (.loading, "ZoriDesk is checking the service on this Mac."),
            (.online, "This view comes from the resident ZoriDesk service."),
            (.partial(reason: "private-partial"), "ZoriDesk is showing the information it could safely load."),
            (.offline(reason: "private-offline"), "ZoriDesk reconnects automatically. Try again if this continues."),
        ]
        for (state, detail) in cases {
            XCTAssertEqual(
                LocalProductExperience(snapshot: nil, connectionState: state).connection.detail,
                detail
            )
        }
        XCTAssertEqual(LocalProductCopy.connectionCheckingDetail, "ZoriDesk is checking the service on this Mac.")
        XCTAssertEqual(LocalProductCopy.connectionOnlineDetail, "This view comes from the resident ZoriDesk service.")
        XCTAssertEqual(LocalProductCopy.connectionPartialDetail, "ZoriDesk is showing the information it could safely load.")
        XCTAssertEqual(LocalProductCopy.connectionReconnectingDetail, "ZoriDesk reconnects automatically. Try again if this continues.")
        XCTAssertEqual(LocalProductCopy.missionConnectingTitle, "Connecting to ZoriDesk")
        XCTAssertEqual(LocalProductCopy.missionStartingServiceBanner, "Starting the ZoriDesk service on this Mac")
    }

    func testConnectionBrandingPreservesStatesRefreshAndSnapshots() throws {
        let empty = LocalProductSnapshot.empty(viewVersion: "view-preserved")
        let populated = try ExperienceFixtures.populatedSnapshot()
        let cases: [(LocalProductSnapshot?, LocalProductConnectionState, LocalProductExperienceState, String, String, Bool)] = [
            (nil, .loading, .loading, "Starting local service", "arrow.triangle.2.circlepath", false),
            (populated, .loading, .loading, "Starting local service", "arrow.triangle.2.circlepath", false),
            (empty, .online, .connectedEmpty, "Local service connected", "checkmark.circle.fill", false),
            (populated, .online, .connectedPopulated, "Local service connected", "checkmark.circle.fill", false),
            (populated, .partial(reason: "private-partial"), .partial, "Some information is unavailable", "exclamationmark.triangle.fill", true),
            (nil, .stale(reason: "private-stale"), .stalePreserved, "Showing last loaded state", "clock.badge.exclamationmark", true),
            (nil, .offline(reason: "private-offline"), .offline, "Local service unavailable", "bolt.slash.fill", true),
            (empty, .offline(reason: "private-offline"), .offlinePreserved, "Showing last loaded state", "bolt.slash.fill", true),
            (populated, .fatal(reason: "private-fatal"), .fatal, "Local service rejected the connection", "exclamationmark.octagon.fill", true),
        ]
        for (snapshot, state, expectedState, title, icon, refresh) in cases {
            let experience = LocalProductExperience(snapshot: snapshot, connectionState: state)
            XCTAssertEqual(experience.state, expectedState)
            XCTAssertEqual(experience.connection.title, title)
            XCTAssertEqual(experience.connection.systemImage, icon)
            XCTAssertEqual(experience.connection.offersRefresh, refresh)
            XCTAssertFalse(experience.connection.detail.contains("private-"))
            XCTAssertEqual(experience.workActivity.map(\.runID), snapshot?.runs.map(\.runID) ?? [])
            XCTAssertEqual(experience.acceptedEvidence.map(\.evidenceID), snapshot?.evidence.map(\.evidenceID) ?? [])
            XCTAssertEqual(experience.attention.count, snapshot?.attention.count ?? 0)
            XCTAssertEqual(experience.teams.count, snapshot?.teams.count ?? 0)
            XCTAssertEqual(experience.runtimes.count, snapshot?.runtimes.count ?? 0)
            XCTAssertEqual(experience.homeGroups, [.attention, .workActivity, .teams, .systemReadiness])
        }
        XCTAssertEqual(
            LocalProductExperience(snapshot: nil, connectionState: .stale(reason: "private-stale")).connection.detail,
            "Reconnect to refresh this workspace."
        )
        XCTAssertEqual(
            LocalProductExperience(snapshot: empty, connectionState: .offline(reason: "private-offline")).connection.detail,
            "The local service is unavailable. Your workspace is preserved."
        )
        XCTAssertEqual(
            LocalProductExperience(snapshot: populated, connectionState: .fatal(reason: "private-fatal")).connection.detail,
            "Check the local service configuration, then try again."
        )
    }

    func testPrimaryDestinationsAreTaskFirstAndBounded() {
        XCTAssertEqual(
            LocalProductWorkspaceState().tasks.map(\.title),
            ["New task"]
        )
        XCTAssertEqual(
            LocalProductInspectorTab.allCases.map(\.rawValue),
            ["Team", "Context", "Changes", "Evidence"]
        )
        XCTAssertFalse(
            LocalProductInteractionCopy.primaryFlow.contains("Home")
        )
        XCTAssertFalse(
            LocalProductInteractionCopy.primaryFlow.contains("System")
        )
    }

    func testHomeAndWorkPreserveFrozenSourceOrder() throws {
        let snapshot = try ExperienceFixtures.populatedSnapshot()
        let experience = LocalProductExperience(
            snapshot: snapshot,
            connectionState: .online
        )

        XCTAssertEqual(
            experience.homeGroups,
            [.attention, .workActivity, .teams, .systemReadiness]
        )
        XCTAssertEqual(
            experience.workActivity.map(\.runID),
            ["run-first", "run-second"]
        )
        XCTAssertEqual(
            experience.acceptedEvidence.map(\.evidenceID),
            ["evidence-first", "evidence-second"]
        )
        XCTAssertEqual(
            experience.attentionActions,
            ["Review blocked work"]
        )
        XCTAssertFalse(
            LocalProductInteractionCopy.primaryFlow.contains("Compare")
        )
    }

    func testPresentationStatesDistinguishAvailabilityAndPreservedViews() {
        let empty = LocalProductSnapshot.empty(viewVersion: "view-empty")

        XCTAssertEqual(
            LocalProductExperience(
                snapshot: nil,
                connectionState: .loading
            ).state,
            .loading
        )
        XCTAssertEqual(
            LocalProductExperience(
                snapshot: empty,
                connectionState: .online
            ).state,
            .connectedEmpty
        )
        XCTAssertEqual(
            LocalProductExperience(
                snapshot: nil,
                connectionState: .offline(reason: "unavailable")
            ).state,
            .offline
        )
        XCTAssertEqual(
            LocalProductExperience(
                snapshot: empty,
                connectionState: .offline(reason: "unavailable")
            ).state,
            .offlinePreserved
        )
        XCTAssertEqual(
            LocalProductExperience(
                snapshot: empty,
                connectionState: .stale(reason: "stale_view")
            ).state,
            .stalePreserved
        )
        XCTAssertEqual(
            LocalProductExperience(
                snapshot: empty,
                connectionState: .partial(reason: "partial_view")
            ).state,
            .partial
        )
        XCTAssertEqual(
            LocalProductExperience(
                snapshot: nil,
                connectionState: .fatal(reason: "unauthorized_peer")
            ).state,
            .fatal
        )
    }

    func testConnectionCopyDescribesLocalServiceWithoutTransportReasons() {
        let loading = LocalProductExperience(
            snapshot: nil,
            connectionState: .loading
        ).connection
        let offline = LocalProductExperience(
            snapshot: nil,
            connectionState: .offline(reason: "invalid_socket")
        ).connection
        let preserved = LocalProductExperience(
            snapshot: .empty(viewVersion: "last-view"),
            connectionState: .offline(reason: "invalid_socket")
        ).connection
        let fatal = LocalProductExperience(
            snapshot: nil,
            connectionState: .fatal(reason: "unauthorized_peer")
        ).connection

        XCTAssertEqual(loading.title, "Starting local service")
        XCTAssertEqual(offline.title, "Local service unavailable")
        XCTAssertEqual(
            offline.detail,
            "ZoriDesk reconnects automatically. Try again if this continues."
        )
        XCTAssertEqual(preserved.title, "Showing last loaded state")
        XCTAssertEqual(fatal.title, "Local service rejected the connection")

        for presentation in [loading, offline, preserved, fatal] {
            XCTAssertFalse(presentation.title.contains("invalid_socket"))
            XCTAssertFalse(presentation.detail.contains("invalid_socket"))
            XCTAssertFalse(presentation.title.localizedCaseInsensitiveContains("offline"))
        }
    }

    func testVisibleFrozenCopyIsFunctionalAndContainsNoEmDash() {
        XCTAssertEqual(LocalProductCopy.workActivity, "Work activity")
        XCTAssertEqual(
            LocalProductCopy.homeTitle,
            "Your local agent workspace"
        )
        XCTAssertTrue(
            LocalProductCopy.all.allSatisfy {
                !$0.contains("—") && !$0.contains("–")
            }
        )
    }

    func testVisibleNameFallsBackWhenDecoratedValueSanitizesToInternalID() {
        XCTAssertEqual(
            LocalProductExperience.visibleName(
                "\u{001B}[31mteam-internal\u{001B}[0m",
                internalID: "team-internal",
                fallback: "Team"
            ),
            "Team"
        )
        XCTAssertEqual(
            LocalProductExperience.visibleName(
                "Release Team",
                internalID: "team-internal",
                fallback: "Team"
            ),
            "Release Team"
        )
    }
}

enum ExperienceFixtures {
    static func populatedSnapshot() throws -> LocalProductSnapshot {
        var json = LocalProductModelsTests.snapshotJSON
        json = json.replacingOccurrences(
            of: "\"teams\":[]",
            with: """
            "teams":[{
              "team_instance_id":"team-first",
              "display_name":"Release Team",
              "source_kind":"saved",
              "state":"ready",
              "confirmed":true,
              "executable":true,
              "read_only":false,
              "agents":[{
                "role_kind":"main",
                "agent_definition_id":"agent-main",
                "runtime_profile_id":"profile-main",
                "binding_status":"configured",
                "harness_adapter":"opencode",
                "provider_id":"deepseek",
                "provider_account_id":"deepseek.primary",
                "model_id":"deepseek/deepseek-chat",
                "credential_revision":3
              },{
                "role_kind":"subagent",
                "agent_definition_id":"agent-reviewer",
                "runtime_profile_id":"profile-reviewer",
                "binding_status":"configured",
                "harness_adapter":"loom-native",
                "provider_id":"minimax",
                "provider_account_id":"minimax.primary",
                "model_id":"MiniMax-M3",
                "credential_revision":2
              }]
            }]
            """
        )
        json = json.replacingOccurrences(
            of: "\"runs\":[]",
            with: """
            "runs":[{
              "run_id":"run-first",
              "work_item_id":"work-first",
              "phase":"verification",
              "terminal_status":"",
              "terminal_reason":"",
              "runtime_instance_id":"runtime-first",
              "agent_instance_id":"agent-first",
              "claim_generation":1
            },{
              "run_id":"run-second",
              "work_item_id":"work-second",
              "phase":"complete",
              "terminal_status":"accepted",
              "terminal_reason":"",
              "runtime_instance_id":"runtime-second",
              "agent_instance_id":"agent-second",
              "claim_generation":2
            }]
            """
        )
        json = json.replacingOccurrences(
            of: "\"evidence\":[]",
            with: """
            "evidence":[{
              "evidence_id":"evidence-first",
              "work_item_id":"work-first",
              "digest":"digest-first"
            },{
              "evidence_id":"evidence-second",
              "work_item_id":"work-second",
              "digest":"digest-second"
            }]
            """
        )
        json = json.replacingOccurrences(
            of: "\"attention\":[]",
            with: """
            "attention":[{
              "schema_version":1,
              "attention_id":"attention-first",
              "kind":"blocked",
              "severity":"high",
              "team_instance_id":"team-first",
              "logical_node_id":"node-first",
              "work_item_id":"work-first",
              "approval_request_id":"",
              "runtime_instance_id":"",
              "status":"open",
              "occurred_at":"2026-07-29T00:00:00Z",
              "action_required":"Review blocked work"
            }]
            """
        )
        return try LocalProductWire.decodeSnapshot(Data(json.utf8))
    }

    static func snapshot(
        partial: Bool = false,
        stale: Bool = false
    ) throws -> LocalProductSnapshot {
        var json = LocalProductModelsTests.snapshotJSON
        json = json.replacingOccurrences(
            of: "\"partial\":false",
            with: "\"partial\":\(partial)"
        )
        json = json.replacingOccurrences(
            of: "\"stale\":false",
            with: "\"stale\":\(stale)"
        )
        let reason: String
        if stale {
            reason = "stale_view"
        } else if partial {
            reason = "partial_view"
        } else {
            reason = ""
        }
        json = json.replacingOccurrences(
            of: "\"reason\":\"\"",
            with: "\"reason\":\"\(reason)\""
        )
        return try LocalProductWire.decodeSnapshot(Data(json.utf8))
    }
}
