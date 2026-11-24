import Foundation
import Darwin
import XCTest
@testable import LoomLocalAppCore

final class LocalProductPathsTests: XCTestCase {
    func testProductPathsMatchSharedDaemonFixture() throws {
        var repo = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { repo.deleteLastPathComponent() }
        let body = try Data(contentsOf: repo.appendingPathComponent("testdata/product-identity.json"))
        let fixture = try JSONDecoder().decode([String: String].self, from: body)
        let paths = try LocalProductPaths(home: try XCTUnwrap(fixture["home"]))
        let actual = [
            "product_id": LocalProductPaths.productID,
            "bundle_id": LocalProductPaths.bundleID,
            "service_label": LocalProductPaths.serviceLabel,
            "build_id": LocalProductPaths.buildID,
            "root": paths.root.path, "socket": paths.socket.path,
            "state": paths.state.path, "vault": paths.vault.path,
            "key": paths.key.path, "diagnostics": paths.diagnostics.path,
        ]
        for (key, value) in actual { XCTAssertEqual(value, fixture[key], key) }
    }

    func testProductPathsRejectUnsafeAndOverlongHomes() throws {
        for home in ["", "/", ".", "/Users/x/../y", "/Users/x/", "/Users/\0x",
                     "/" + String(repeating: "界", count: 40)] {
            XCTAssertThrowsError(try LocalProductPaths(home: home), home)
        }
        for home in ["/Users/fixture", "/Users/space user", "/Users/用户", "/private/tmp/fixture"] {
            XCTAssertNoThrow(try LocalProductPaths(home: home))
        }
    }

    func testProductPingRejectsOtherProductsAndUnavailableService() {
        XCTAssertTrue(LocalProductPaths.acceptsPing(protocolVersion: 1, available: true, buildID: "zoridesk-local-v1"))
        for build in ["", "loom-phase2a-w1", "zoridesk-local-v10"] {
            XCTAssertFalse(LocalProductPaths.acceptsPing(protocolVersion: 1, available: true, buildID: build))
        }
        XCTAssertFalse(LocalProductPaths.acceptsPing(protocolVersion: 2, available: true, buildID: "zoridesk-local-v1"))
        XCTAssertFalse(LocalProductPaths.acceptsPing(protocolVersion: 1, available: false, buildID: "zoridesk-local-v1"))
    }

    func testProductRootRejectsLegacyAliasWithoutChangingFiles() throws {
        let home = URL(fileURLWithPath: "/private/tmp")
            .appendingPathComponent("zd-\(UUID().uuidString.prefix(8))")
        try FileManager.default.createDirectory(at: home, withIntermediateDirectories: false,
                                               attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: home) }
        let paths = try LocalProductPaths(home: home.path)
        let legacy = home.appendingPathComponent("Library/Application Support/Loom")
        try FileManager.default.createDirectory(at: legacy, withIntermediateDirectories: true,
                                               attributes: [.posixPermissions: 0o700])
        let sentinel = legacy.appendingPathComponent("keep")
        try Data("legacy".utf8).write(to: sentinel)
        XCTAssertFalse(FileManager.default.fileExists(atPath: paths.root.path))
        try FileManager.default.createSymbolicLink(at: paths.root, withDestinationURL: legacy)
        XCTAssertThrowsError(try paths.prepareRoot())
        XCTAssertEqual(try Data(contentsOf: sentinel), Data("legacy".utf8))
        try FileManager.default.removeItem(at: paths.root)
        try paths.prepareRoot()
        XCTAssertTrue(FileManager.default.fileExists(atPath: paths.root.path))
        XCTAssertEqual(try Data(contentsOf: sentinel), Data("legacy".utf8))
        let attributes = try FileManager.default.attributesOfItem(atPath: paths.root.path)
        XCTAssertEqual((attributes[.posixPermissions] as? NSNumber)?.intValue, 0o700)
        let registry = paths.root.appendingPathComponent("chat-sessions.json")
        try LocalPrivateRegistryStorage.write(Data("new-product".utf8), to: registry)
        XCTAssertEqual(try LocalPrivateRegistryStorage.read(from: registry), Data("new-product".utf8))
        XCTAssertEqual(try Data(contentsOf: sentinel), Data("legacy".utf8))
    }

    func testProductIdentityWiringMatchesBundleAndBothBootstrapPaths() throws {
        var repo = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { repo.deleteLastPathComponent() }
        let package = repo.appendingPathComponent("apps/macos")
        func plist(_ relative: String) throws -> [String: Any] {
            let value = try PropertyListSerialization.propertyList(
                from: Data(contentsOf: package.appendingPathComponent(relative)), format: nil)
            return try XCTUnwrap(value as? [String: Any])
        }
        func text(_ relative: String) throws -> String {
            try String(contentsOf: repo.appendingPathComponent(relative), encoding: .utf8)
        }
        let info = try plist("Resources/Info.plist")
        XCTAssertEqual(info["CFBundleIdentifier"] as? String, LocalProductPaths.bundleID)
        XCTAssertEqual(info["CFBundleExecutable"] as? String, "LoomLocalApp")
        let agent = try plist("Resources/\(LocalProductPaths.serviceLabel).plist")
        XCTAssertEqual(agent["Label"] as? String, LocalProductPaths.serviceLabel)
        XCTAssertEqual(agent["BundleProgram"] as? String, "Contents/Library/Helpers/loomd")
        XCTAssertEqual(agent["ProgramArguments"] as? [String], ["loomd", "--zoridesk-app-service"])
        let host = try text("apps/macos/Sources/LoomLocalApp/LocalServiceProcessHost.swift")
        XCTAssertTrue(host.contains(#""--zoridesk-app-service""#))
        XCTAssertTrue(host.contains("expectedBuildID: LocalProductPaths.buildID"))
        for source in ["LocalProductStore", "LocalOperationalDiagnostics", "LocalDiagnosticBundle", "LocalIPCClient"] {
            let body = try text("apps/macos/Sources/LoomLocalAppCore/\(source).swift")
            XCTAssertTrue(body.contains("LocalProductPaths"), source)
            XCTAssertFalse(body.contains("Library/Application Support/Loom"), source)
        }
        for script in ["build-loom-local-app", "install-loom-local-app"] {
            let body = try text("scripts/\(script).sh")
            XCTAssertTrue(body.contains("/*/ZoriDesk.app)"))
            XCTAssertFalse(body.contains("/*/Loom.app)"))
            XCTAssertTrue(body.contains(LocalProductPaths.bundleID))
        }
    }

    func testInstallerDryRunRefusesUnknownExistingBundleWithoutMutation() throws {
        var repo = URL(fileURLWithPath: #filePath)
        for _ in 0..<5 { repo.deleteLastPathComponent() }
        let root = URL(fileURLWithPath: "/private/tmp")
            .appendingPathComponent("zd-install-\(UUID().uuidString.prefix(8))")
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: false,
                                               attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: root) }
        let target = root.appendingPathComponent("ZoriDesk.app")
        try FileManager.default.createDirectory(at: target, withIntermediateDirectories: false,
                                               attributes: [.posixPermissions: 0o700])
        let sentinel = target.appendingPathComponent("keep")
        try Data("unknown-bundle".utf8).write(to: sentinel)
        let process = Process()
        process.executableURL = URL(fileURLWithPath: "/bin/sh")
        process.arguments = [repo.appendingPathComponent("scripts/install-loom-local-app.sh").path,
            "--dry-run", "--app", root.appendingPathComponent("source/ZoriDesk.app").path,
            "--destination", target.path]
        let output = Pipe()
        process.standardOutput = output
        process.standardError = output
        try process.run()
        process.waitUntilExit()
        let message = String(decoding: output.fileHandleForReading.readDataToEndOfFile(), as: UTF8.self)
        XCTAssertEqual(process.terminationStatus, 3)
        XCTAssertTrue(message.contains("foreign or invalid app target"))
        XCTAssertEqual(try Data(contentsOf: sentinel), Data("unknown-bundle".utf8))
        XCTAssertEqual(try FileManager.default.contentsOfDirectory(atPath: root.path), ["ZoriDesk.app"])
    }

    func testProductRegistryRejectsAliasedParentBeforeCreatingFiles() throws {
        let root = URL(fileURLWithPath: "/private/tmp")
            .appendingPathComponent("zd-registry-\(UUID().uuidString.prefix(8))")
        try FileManager.default.createDirectory(at: root, withIntermediateDirectories: false,
                                               attributes: [.posixPermissions: 0o700])
        defer { try? FileManager.default.removeItem(at: root) }
        let legacy = root.appendingPathComponent("legacy")
        try FileManager.default.createDirectory(at: legacy, withIntermediateDirectories: false,
                                               attributes: [.posixPermissions: 0o700])
        let alias = root.appendingPathComponent("alias")
        try FileManager.default.createSymbolicLink(at: alias, withDestinationURL: legacy)
        XCTAssertThrowsError(try LocalPrivateRegistryStorage.write(
            Data("not-to-be-written".utf8), to: alias.appendingPathComponent("cache/chat-sessions.json")))
        XCTAssertTrue(try FileManager.default.contentsOfDirectory(atPath: legacy.path).isEmpty)
    }
}
