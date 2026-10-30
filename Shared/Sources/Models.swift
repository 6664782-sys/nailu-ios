import Foundation

public struct Node: Codable, Identifiable, Equatable, Sendable {
    public let id: String
    public let name: String
    public let `protocol`: String
    public let host: String
    public let port: Int
    public let password: String
    public let method: String?
    public let serverName: String?
    public let network: String?
    public let path: String?
    public let webSocketHost: String?
}

public struct ParsedSubscription: Decodable, Sendable {
    public let nodes: [Node]
    public let unsupported: Int
}

public struct Account: Decodable, Sendable {
    public let email: String
    public let uuid: String?
}

public struct Subscription: Decodable, Sendable {
    public let transfer_enable: Int64
    public let u: Int64
    public let d: Int64
    public let expired_at: Double?
    public let subscribe_url: String
    public let plan: Plan?
    public var remaining: Int64 {
        let allowance = max(0, transfer_enable)
        let upload = min(allowance, max(0, u))
        return max(0, allowance - upload - min(allowance - upload, max(0, d)))
    }
    public func usable(at date: Date = Date()) -> Bool {
        remaining > 0 && (expired_at == nil || expired_at == 0 || expired_at! > date.timeIntervalSince1970)
    }
    public struct Plan: Decodable, Sendable { public let name: String }
}

public enum NaiLuOrigin {
    public static let api = URL(string: "https://nailu.pro/api/v1/")!
    public static let privacy = URL(string: "https://nailu.pro/privacy.html")!
    public static let terms = URL(string: "https://nailu.pro/terms.html")!
    public static let support = URL(string: "https://nailu.pro/#/tickets")!
    public static let community = URL(string: "https://t.me/nailujiasu")!
    public static func subscription(_ source: String) throws -> URL {
        let hosts: Set<String> = ["nailu.pro", "hkdinyue-diny.nailujiedian.xyz", "nailu.cc", "api1.nailu.club", "api.nailu.club", "api.nailu.sbs", "api1.nailu.sbs", "cn1.nailu.club", "cn2.nailu.club", "cn3.nailu.club", "cn4.nailu.pro", "cn5.nailu.pro", "cn6.nailu.pro"]
        guard var c = URLComponents(string: source), c.scheme == "https", let host = c.host, hosts.contains(host.lowercased()), c.user == nil, c.password == nil, c.port == nil || c.port == 443 else { throw ClientError.message("订阅地址不属于奈鹿，已拒绝加载") }
        var query = c.queryItems?.filter { $0.name != "flag" } ?? []
        query.append(URLQueryItem(name: "flag", value: "v2ray")); c.queryItems = query
        guard let url = c.url else { throw ClientError.message("订阅地址无效") }; return url
    }
}

public enum ClientError: LocalizedError {
    case message(String)
    case unauthorized
    public var errorDescription: String? {
        switch self { case .message(let value): return value; case .unauthorized: return "登录已失效，请重新登录" }
    }
}

public enum Traffic {
    public static func gigabytes(_ bytes: Int64) -> String { String(format: "%.2f GB", Double(max(0, bytes)) / 1_073_741_824) }
}
