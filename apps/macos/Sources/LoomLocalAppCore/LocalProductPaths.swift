import Foundation
import Darwin

// Protocol and encrypted-record names stay compatible; only product-owned roots differ.
public struct LocalProductPaths: Equatable, Sendable {
    public static let productID = "zoridesk"
    public static let bundleID = "io.github.ggggggtm.zoridesk.local"
    public static let serviceLabel = bundleID + ".daemon"
    public static let buildID = "zoridesk-local-v1"

    public let root: URL
    public var state: URL { root.appendingPathComponent("state/loom.db") }
    public var socket: URL { root.appendingPathComponent("run/loomd.sock") }
    public var vault: URL { root.appendingPathComponent("state/credential-vault.db") }
    public var key: URL { root.appendingPathComponent("private/vault.key") }
    public var diagnostics: URL { root.appendingPathComponent("diagnostics") }

    public init(home: String) throws {
        guard home.hasPrefix("/"), home != "/", !home.contains("\0"),
              !home.hasSuffix("/"), !home.contains("//"),
              !home.split(separator: "/").contains(where: { $0 == "." || $0 == ".." })
        else { throw LocalProductClientError.invalidSocket }
        root = URL(fileURLWithPath: home, isDirectory: true)
            .appendingPathComponent("Library/Application Support/ZoriDesk", isDirectory: true)
        guard socket.path.utf8.count <= 96 else {
            throw LocalProductClientError.invalidSocket
        }
    }

    public static func installed() throws -> LocalProductPaths {
        guard let home = canonicalPath(FileManager.default.homeDirectoryForCurrentUser.path) else {
            throw LocalProductClientError.invalidSocket
        }
        return try LocalProductPaths(home: home)
    }

    public static var installedRoot: URL {
        URL(fileURLWithPath: canonicalPath(FileManager.default.homeDirectoryForCurrentUser.path)
            ?? FileManager.default.homeDirectoryForCurrentUser.path, isDirectory: true)
            .appendingPathComponent("Library/Application Support/ZoriDesk", isDirectory: true)
    }

    // Reject aliases before creating a child; never chmod an existing foreign root.
    public func prepareRoot() throws {
        let home = root.deletingLastPathComponent().deletingLastPathComponent().deletingLastPathComponent()
        for directory in [home, home.appendingPathComponent("Library"),
                          home.appendingPathComponent("Library/Application Support"), root] {
            var info = stat()
            if lstat(directory.path, &info) != 0 {
                guard errno == ENOENT, directory != home,
                      mkdir(directory.path, 0o700) == 0 else {
                    throw LocalProductClientError.unavailable
                }
            }
            guard lstat(directory.path, &info) == 0,
                  info.st_mode & S_IFMT == S_IFDIR, info.st_uid == geteuid(),
                  info.st_mode & 0o022 == 0,
                  Self.canonicalPath(directory.path) == directory.path,
                  directory != root || info.st_mode & 0o777 == 0o700 else {
                throw LocalProductClientError.unavailable
            }
        }
    }

    private static func canonicalPath(_ path: String) -> String? {
        guard let resolved = realpath(path, nil) else { return nil }
        defer { free(resolved) }
        return String(cString: resolved)
    }

    public static func acceptsPing(protocolVersion: Int, available: Bool, buildID: String) -> Bool {
        protocolVersion == 1 && available && buildID == Self.buildID
    }
}
