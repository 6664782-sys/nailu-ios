import SwiftUI

@MainActor
final class SessionStore: ObservableObject {
    @Published private(set) var account: Account?
    @Published private(set) var subscription: Subscription?
    @Published private(set) var nodes: [Node] = []
    @Published private(set) var unsupportedNodes = 0
    @Published private(set) var busy = false
    @Published private(set) var restoring = true
    @Published var error: String?
    @Published var selectedID = ""
    @Published var registrationConfig: NailuAPI.Config?
    private let api = NailuAPI()
    private var token: String?
    private var revision = 0

    var selected: Node? { nodes.first { $0.id == selectedID } ?? nodes.first }
    func restore() async {
        defer { restoring = false }
        do {
            if let data = try KeychainStore.read("session"), let value = String(data: data, encoding: .utf8) {
                token = value
                try await refresh()
            }
        } catch {
            self.error = error.localizedDescription
            if let failure = error as? ClientError, case .unauthorized = failure { clearLocalSession() }
        }
    }
    func login(email: String, password: String) async {
        let ticket = revision
        await operation {
            let auth: NailuAPI.Auth = try await self.api.call("guest/nailu/app/login", body: ["email": email.trimmingCharacters(in: .whitespacesAndNewlines), "password": password])
            guard ticket == self.revision else { return }
            try await self.accept(auth.auth_data)
        }
    }
    func prepareRegistration() async {
        await operation {
            self.registrationConfig = try await self.api.call("guest/nailu/app/config")
        }
    }
    func sendEmail(_ email: String) async throws {
        guard let flow = registrationConfig?.app_flow else { throw ClientError.message("请重新打开注册页面") }
        try await api.acknowledge("guest/nailu/app/email", body: ["email": email.trimmingCharacters(in: .whitespacesAndNewlines), "app_flow": flow])
    }
    func register(email: String, password: String, code: String, invitation: String) async {
        let ticket = revision
        guard let flow = registrationConfig?.app_flow, registrationConfig?.registration_open != false else { error = "暂未开放注册，请稍后重试"; return }
        await operation {
            let auth: NailuAPI.Auth = try await self.api.call("guest/nailu/app/register", body: ["email": email.trimmingCharacters(in: .whitespacesAndNewlines), "password": password, "email_code": code, "invite_code": invitation, "app_flow": flow])
            guard ticket == self.revision else { return }
            self.registrationConfig = nil
            try await self.accept(auth.auth_data)
        }
    }
    func refresh() async throws {
        guard let token else { throw ClientError.unauthorized }
        let epoch = revision
        async let nextAccount: Account = api.call("user/info", token: token)
        async let nextSubscription: Subscription = api.call("user/getSubscribe", token: token)
        let (profile, plan) = try await (nextAccount, nextSubscription)
        guard epoch == revision else { return }
        account = profile; subscription = plan
        if !plan.usable() { nodes = []; return }
        let raw = try await api.subscription(plan.subscribe_url)
        let parsed = try await Task.detached(priority: .userInitiated) {
            var failure: NSError?
            guard let json = NaiLuEngineParseSubscription(raw, &failure) else { throw failure ?? NSError(domain: "NaiLu", code: 1, userInfo: [NSLocalizedDescriptionKey: "无法解析节点"]) }
            return try JSONDecoder().decode(ParsedSubscription.self, from: Data(json.utf8))
        }.value
        guard epoch == revision else { return }
        nodes = parsed.nodes; unsupportedNodes = parsed.unsupported
        if !nodes.contains(where: { $0.id == selectedID }) { selectedID = nodes.first?.id ?? "" }
    }
    func refreshFromUI() async { await operation { try await self.refresh() } }
    func logout() { revision += 1; clearLocalSession() }
    private func clearLocalSession() {
        token = nil; account = nil; subscription = nil; nodes = []; selectedID = ""
        do { try KeychainStore.delete("session"); try KeychainStore.delete("active-tunnel") } catch { self.error = error.localizedDescription }
    }
    private func accept(_ value: String) async throws {
        guard !value.isEmpty else { throw ClientError.message("登录响应无效") }
        try KeychainStore.write("session", data: Data(value.utf8)); token = value
        try await refresh()
    }
    private func operation(_ body: () async throws -> Void) async {
        guard !busy else { return }; busy = true; error = nil
        defer { busy = false }
        do { try await body() } catch { self.error = error.localizedDescription }
    }
}
