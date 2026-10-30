import Foundation
import Security

enum KeychainStore {
    #if targetEnvironment(simulator)
    // Unsigned simulator binaries have no keychain entitlement. Keep debug
    // credentials in memory only; never fall back to plaintext disk storage.
    // This branch is excluded from every iPhone build.
    private final class TransientStore: @unchecked Sendable {
        let lock = NSLock()
        var values: [String: Data] = [:]
        func read(_ key: String) -> Data? {
            lock.lock(); defer { lock.unlock() }
            return values[key]
        }
        func write(_ key: String, data: Data) {
            lock.lock(); defer { lock.unlock() }
            values[key] = data
        }
        func delete(_ key: String) {
            lock.lock(); defer { lock.unlock() }
            values.removeValue(forKey: key)
        }
    }
    private static let transient = TransientStore()
    static func read(_ account: String) throws -> Data? { transient.read(account) }
    static func write(_ account: String, data: Data) throws { transient.write(account, data: data) }
    static func delete(_ account: String) throws { transient.delete(account) }
    #else
    private static func query(_ account: String) throws -> [String: Any] {
        guard let group = Bundle.main.object(forInfoDictionaryKey: "KeychainAccessGroup") as? String, !group.contains("$("), group.contains(".pro.nailu.ios.shared") else { throw ClientError.message("请先配置 Apple 团队签名和共享钥匙串") }
        return [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "pro.nailu.ios", kSecAttrAccount as String: account, kSecAttrAccessGroup as String: group]
    }
    static func read(_ account: String) throws -> Data? {
        var q = try query(account); q[kSecReturnData as String] = true; q[kSecMatchLimit as String] = kSecMatchLimitOne
        var item: CFTypeRef?; let status = SecItemCopyMatching(q as CFDictionary, &item)
        if status == errSecItemNotFound { return nil }
        guard status == errSecSuccess else { throw ClientError.message("无法读取安全存储") }; return item as? Data
    }
    static func write(_ account: String, data: Data) throws {
        let q = try query(account)
        let update = [kSecValueData as String: data]
        let status = SecItemUpdate(q as CFDictionary, update as CFDictionary)
        if status == errSecItemNotFound {
            var add = q; add[kSecValueData as String] = data; add[kSecAttrAccessible as String] = kSecAttrAccessibleAfterFirstUnlockThisDeviceOnly
            guard SecItemAdd(add as CFDictionary, nil) == errSecSuccess else { throw ClientError.message("无法保存安全存储") }
        } else if status != errSecSuccess { throw ClientError.message("无法更新安全存储") }
    }
    static func delete(_ account: String) throws {
        let status = SecItemDelete(try query(account) as CFDictionary)
        guard status == errSecSuccess || status == errSecItemNotFound else { throw ClientError.message("无法清除安全存储") }
    }
    #endif
}
