import Foundation
import Security

/// Minimal Keychain wrapper for the API token. The item is readable after
/// the first unlock so queued uploads can resume in the background, and it
/// never leaves this device.
nonisolated enum Keychain {
    private static let service = "app.pidex.ios"
    /// Where tokens were kept before the app was renamed. Read once and
    /// moved; remove once every install has been opened since.
    private static let legacyService = "app.casebook.ios"

    static func read(_ account: String) -> String? {
        if let value = read(account, service: service) { return value }
        guard let old = read(account, service: legacyService) else { return nil }
        write(old, account: account)
        delete(account, service: legacyService)
        return old
    }

    private static func read(_ account: String, service: String) -> String? {
        let query: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
            kSecReturnData as String: true,
            kSecMatchLimit as String: kSecMatchLimitOne,
        ]
        var out: CFTypeRef?
        guard SecItemCopyMatching(query as CFDictionary, &out) == errSecSuccess, let data = out as? Data else {
            return nil
        }
        return String(data: data, encoding: .utf8)
    }

    private static func delete(_ account: String, service: String) {
        SecItemDelete([
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ] as CFDictionary)
    }

    static func write(_ value: String, account: String) {
        let base: [String: Any] = [
            kSecClass as String: kSecClassGenericPassword,
            kSecAttrService as String: service,
            kSecAttrAccount as String: account,
        ]
        SecItemDelete(base as CFDictionary)
        guard !value.isEmpty else { return }
        var add = base
        add[kSecValueData as String] = Data(value.utf8)
        add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
        SecItemAdd(add as CFDictionary, nil)
    }
}
