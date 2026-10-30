import Foundation
import Combine
import NetworkExtension

@MainActor
final class TunnelController: ObservableObject {
    @Published private(set) var status: NEVPNStatus = .invalid
    @Published private(set) var busy = false
    @Published var error: String?
    private var manager: NETunnelProviderManager?
    private var observer: NSObjectProtocol?
    private var intent = 0

    var connected: Bool { status == .connected }
    var running: Bool { status == .connected || status == .connecting || status == .reasserting }
    var requiresStop: Bool { busy || running || status == .disconnecting }
    var title: String {
        switch status { case .connected: return "已连接"; case .connecting, .reasserting: return "连接中"; case .disconnecting: return "断开中"; default: return "未连接" }
    }
    func load() async {
        do {
            let managers = try await NETunnelProviderManager.loadAllFromPreferences()
            manager = managers.first { ($0.protocolConfiguration as? NETunnelProviderProtocol)?.providerBundleIdentifier == Bundle.main.object(forInfoDictionaryKey: "PacketTunnelBundleID") as? String }
            observe()
        } catch { self.error = error.localizedDescription }
    }
    func connect(node: Node, subscription: Subscription) async {
        #if targetEnvironment(simulator)
        error = "模拟器可调试界面和账户，VPN 连接请在签名后的 iPhone 真机测试"
        return
        #else
        guard !requiresStop else { return }
        guard subscription.usable() else { error = "套餐已到期或流量不足"; return }
        busy = true; error = nil; defer { busy = false }
        intent += 1; let ticket = intent
        do {
            let json = String(decoding: try JSONEncoder().encode(node), as: UTF8.self)
            var failure: NSError?
            guard let configuration = NaiLuEngineConfiguration(json, &failure) else { throw failure ?? NSError(domain: "NaiLu", code: 1, userInfo: [NSLocalizedDescriptionKey: "节点配置无效"]) }
            try KeychainStore.write("active-tunnel", data: Data(configuration.utf8))
            let m = manager ?? NETunnelProviderManager()
            let proto = NETunnelProviderProtocol()
            guard let id = Bundle.main.object(forInfoDictionaryKey: "PacketTunnelBundleID") as? String else { throw ClientError.message("VPN 扩展配置缺失") }
            proto.providerBundleIdentifier = id; proto.serverAddress = node.name
            proto.providerConfiguration = ["credentialReference": "active-tunnel"]
            proto.disconnectOnSleep = false
            m.localizedDescription = "奈鹿 NaiLu"; m.protocolConfiguration = proto; m.isEnabled = true
            try await m.saveToPreferences(); try await m.loadFromPreferences()
            guard ticket == intent else { return }
            manager = m; observe()
            try m.connection.startVPNTunnel()
        } catch { self.error = error.localizedDescription }
        #endif
    }
    func disconnect() { intent += 1; manager?.connection.stopVPNTunnel() }
    private func observe() {
        if let observer { NotificationCenter.default.removeObserver(observer) }
        status = manager?.connection.status ?? .invalid
        guard let connection = manager?.connection else { return }
        observer = NotificationCenter.default.addObserver(forName: .NEVPNStatusDidChange, object: connection, queue: .main) { [weak self] _ in
            Task { @MainActor in self?.status = connection.status }
        }
    }
    deinit { if let observer { NotificationCenter.default.removeObserver(observer) } }
}
