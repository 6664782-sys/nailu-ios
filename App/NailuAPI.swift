import Foundation

actor NailuAPI {
    struct Envelope<T: Decodable>: Decodable { let data: T }
    struct Auth: Decodable { let auth_data: String }
    struct Config: Decodable {
        let app_flow: String
        let registration_open: Bool?
        let is_email_verify: Int?
    }
    private let session: URLSession
    private let policy = HTTPSRedirectPolicy()
    init() {
        let config = URLSessionConfiguration.ephemeral
        config.timeoutIntervalForRequest = 15
        config.timeoutIntervalForResource = 30
        config.urlCache = nil
        session = URLSession(configuration: config, delegate: policy, delegateQueue: nil)
    }
    func call<T: Decodable>(_ path: String, body: [String: String]? = nil, token: String? = nil) async throws -> T {
        let data = try await perform(path, body: body, token: token)
        return try JSONDecoder().decode(Envelope<T>.self, from: data).data
    }
    func acknowledge(_ path: String, body: [String: String]) async throws {
        _ = try await perform(path, body: body)
    }
    private func perform(_ path: String, body: [String: String]? = nil, token: String? = nil) async throws -> Data {
        guard !path.contains("://"), !path.contains(".."), !path.hasPrefix("/") else { throw ClientError.message("请求地址无效") }
        var request = URLRequest(url: NaiLuOrigin.api.appendingPathComponent(path))
        request.setValue("application/json", forHTTPHeaderField: "Accept")
        request.setValue("no-store", forHTTPHeaderField: "Cache-Control")
        request.setValue("NaiLu/0.2.0 (ios)", forHTTPHeaderField: "User-Agent")
        if let token { request.setValue(token, forHTTPHeaderField: "Authorization") }
        if let body {
            request.httpMethod = "POST"
            request.setValue("application/json", forHTTPHeaderField: "Content-Type")
            request.httpBody = try JSONEncoder().encode(body)
        }
        let (data, response) = try await session.data(for: request)
        guard let http = response as? HTTPURLResponse, data.count <= 4_194_304 else { throw ClientError.message("服务响应无效") }
        if http.statusCode == 401 || http.statusCode == 403 { throw ClientError.unauthorized }
        guard (200..<300).contains(http.statusCode) else {
            let object = try? JSONSerialization.jsonObject(with: data) as? [String: Any]
            throw ClientError.message((object?["message"] as? String).map { String($0.prefix(300)) } ?? "暂时无法完成请求，请稍后重试")
        }
        return data
    }
    func subscription(_ source: String) async throws -> String {
        let url = try NaiLuOrigin.subscription(source)
        var request = URLRequest(url: url)
        request.setValue("NaiLu/0.2.0 (ios)", forHTTPHeaderField: "User-Agent")
        request.setValue("no-store", forHTTPHeaderField: "Cache-Control")
        let (data, response) = try await session.data(for: request)
        guard let response = response as? HTTPURLResponse, response.statusCode == 200, data.count <= 4_194_304, let text = String(data: data, encoding: .utf8) else { throw ClientError.message("无法更新节点，请检查套餐状态") }
        return text
    }
}

private final class HTTPSRedirectPolicy: NSObject, URLSessionTaskDelegate {
    func urlSession(_ session: URLSession, task: URLSessionTask, willPerformHTTPRedirection response: HTTPURLResponse, newRequest request: URLRequest, completionHandler: @escaping (URLRequest?) -> Void) {
        // Subscription URLs contain credentials: never forward them across redirects.
        completionHandler(nil)
    }
}
