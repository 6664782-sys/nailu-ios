import Foundation
import Security

enum KeychainStore {
    private static func query(_ account: String) throws -> [String: Any] {
        #if targetEnvironment(simulator)
        // Allow unsigned simulator UI development before a developer team exists.
        // Real devices always require the shared, signed access group below.
        return [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "pro.nailu.ios.simulator", kSecAttrAccount as String: account]
        #else
        guard let group = Bundle.main.object(forInfoDictionaryKey: "KeychainAccessGroup") as? String, !group.contains("$("), group.contains(".pro.nailu.ios.shared") else { throw ClientError.message("请先配置 Apple 团队签名和共享钥匙串") }
        return [kSecClass as String: kSecClassGenericPassword, kSecAttrService as String: "pro.nailu.ios", kSecAttrAccount as String: account, kSecAttrAccessGroup as String: group]
        #endif
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
}
