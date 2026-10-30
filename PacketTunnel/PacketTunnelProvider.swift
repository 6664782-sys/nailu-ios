import Foundation
import NetworkExtension
import Darwin

final class PacketTunnelProvider: NEPacketTunnelProvider {
    private let state = DispatchQueue(label: "pro.nailu.tunnel-state")
    private var bridge: PacketBridge?
    private var epoch = 0
    private var starting: ((Error?) -> Void)?

    override func startTunnel(options: [String: NSObject]?, completionHandler: @escaping (Error?) -> Void) {
        state.async { [self] in
            guard starting == nil, bridge == nil else { completionHandler(ClientError.message("连接已启动")); return }
            epoch += 1
            let ticket = epoch
            starting = completionHandler
            do {
                guard let proto = protocolConfiguration as? NETunnelProviderProtocol,
                      proto.providerConfiguration?["credentialReference"] as? String == "active-tunnel",
                      let data = try KeychainStore.read("active-tunnel") else { throw ClientError.message("请回到奈鹿重新选择节点") }
                // Resolve before installing default routes. Only the encrypted peer
                // bypasses the tunnel; destination DNS stays inside the tunnel.
                DispatchQueue.global(qos: .userInitiated).async { [self] in
                    let result = Result { try PeerBootstrap.resolve(configuration: data) }
                    state.async { [self] in
                        guard ticket == epoch else { return }
                        do {
                            let peer = try result.get()
                            var failure: NSError?
                            guard NaiLuEngineStart(peer.configuration, &failure) else { throw failure ?? (ClientError.message("连接引擎启动失败") as NSError) }
                            let next = PacketBridge(flow: packetFlow) { [weak self] in self?.engineFailed() }
                            bridge = next
                            try next.start()
                            setTunnelNetworkSettings(peer.settings) { [self] error in
                                state.async { [self] in
                                    guard ticket == epoch else { return }
                                    if let error { fail(error) } else { let done = starting; starting = nil; done?(nil) }
                                }
                            }
                        } catch { fail(error) }
                    }
                }
            } catch { fail(error) }
        }
    }

    override func stopTunnel(with reason: NEProviderStopReason, completionHandler: @escaping () -> Void) {
        state.async { [self] in
            epoch += 1
            let pending = starting; starting = nil
            pending?(ClientError.message("连接已取消"))
            cleanup(completionHandler)
        }
    }

    private func fail(_ error: Error) {
        epoch += 1
        let done = starting; starting = nil
        cleanup { done?(error) }
    }
    private func engineFailed() {
        state.async { [self] in
            let error = ClientError.message("数据包引擎已停止，请重新连接")
            if starting != nil { fail(error) }
            else { cleanup { [weak self] in self?.cancelTunnelWithError(error) } }
        }
    }
    private func cleanup(_ completion: @escaping () -> Void) {
        let previous = bridge
        bridge = nil
        let finish = { NaiLuEngineStop(); completion() }
        if let previous { previous.stop { [self] in state.async(execute: finish) } }
        else { finish() }
    }
}

private struct PeerBootstrap {
    let configuration: String
    let settings: NEPacketTunnelNetworkSettings

    static func resolve(configuration data: Data) throws -> PeerBootstrap {
        guard var config = try JSONSerialization.jsonObject(with: data) as? [String: Any],
              var outbounds = config["outbounds"] as? [[String: Any]], outbounds.count == 1,
              var settings = outbounds[0]["settings"] as? [String: Any],
              var servers = settings["servers"] as? [[String: Any]], servers.count == 1,
              let host = servers[0]["address"] as? String else { throw ClientError.message("节点配置无效") }
        var hints = addrinfo(); hints.ai_family = AF_UNSPEC; hints.ai_socktype = SOCK_STREAM; hints.ai_flags = AI_DEFAULT
        var result: UnsafeMutablePointer<addrinfo>?
        guard getaddrinfo(host, nil, &hints, &result) == 0, let first = result else { throw ClientError.message("无法解析节点地址，请检查网络") }
        defer { freeaddrinfo(first) }
        var addresses: [(String, Int32)] = []
        var cursor: UnsafeMutablePointer<addrinfo>? = first
        while let item = cursor {
            let info = item.pointee
            if info.ai_family == AF_INET || info.ai_family == AF_INET6 {
                var text = [CChar](repeating: 0, count: Int(NI_MAXHOST))
                if getnameinfo(info.ai_addr, info.ai_addrlen, &text, socklen_t(text.count), nil, 0, NI_NUMERICHOST) == 0 { addresses.append((String(cString: text), info.ai_family)) }
            }
            cursor = info.ai_next
        }
        // Keep the system resolver order, including DNS64 synthesis on IPv6-only
        // networks. No unconditional IPv4 preference is imposed by the client.
        guard let peer = addresses.first else { throw ClientError.message("节点没有可用地址") }
        servers[0]["address"] = peer.0; settings["servers"] = servers; outbounds[0]["settings"] = settings; config["outbounds"] = outbounds
        let encoded = try JSONSerialization.data(withJSONObject: config)
        let network = NEPacketTunnelNetworkSettings(tunnelRemoteAddress: peer.0)
        network.mtu = 1500
        let v4 = NEIPv4Settings(addresses: ["198.18.0.1"], subnetMasks: ["255.255.255.0"])
        v4.includedRoutes = [.default()]
        if peer.1 == AF_INET { v4.excludedRoutes = [NEIPv4Route(destinationAddress: peer.0, subnetMask: "255.255.255.255")] }
        network.ipv4Settings = v4
        let v6 = NEIPv6Settings(addresses: ["fd00:6e61:696c::1"], networkPrefixLengths: [64])
        v6.includedRoutes = [.default()]
        if peer.1 == AF_INET6 { v6.excludedRoutes = [NEIPv6Route(destinationAddress: peer.0, networkPrefixLength: 128)] }
        network.ipv6Settings = v6
        let dns = NEDNSSettings(servers: ["1.1.1.1", "9.9.9.9"])
        dns.matchDomains = [""]; network.dnsSettings = dns
        return PeerBootstrap(configuration: String(decoding: encoded, as: UTF8.self), settings: network)
    }
}
