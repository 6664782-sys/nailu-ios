import Foundation
import NetworkExtension
import Darwin

// Only public packetFlow APIs are used. HEV expects a four-byte network-order
// address-family prefix on Apple platforms; the socketpair carries one IP packet
// per datagram. A full queue drops packets instead of retaining unbounded memory.
final class PacketBridge {
    private let queue = DispatchQueue(label: "pro.nailu.packet-bridge")
    private let worker = DispatchQueue(label: "pro.nailu.hev-worker", qos: .userInitiated)
    private let flow: NEPacketTunnelFlow
    private var source: DispatchSourceRead?
    private var appFD: Int32 = -1
    private var hevFD: Int32 = -1
    private var stopped = false
    private var stopWaiters: [() -> Void] = []
    private let failure: () -> Void

    init(flow: NEPacketTunnelFlow, failure: @escaping () -> Void) { self.flow = flow; self.failure = failure }

    func start() throws {
        var pair: [Int32] = [-1, -1]
        guard socketpair(AF_UNIX, SOCK_DGRAM, 0, &pair) == 0 else { throw ClientError.message("无法创建数据包通道") }
        appFD = pair[0]; hevFD = pair[1]
        _ = fcntl(appFD, F_SETFL, O_NONBLOCK)
        let descriptor = appFD
        let reader = DispatchSource.makeReadSource(fileDescriptor: descriptor, queue: queue)
        source = reader
        reader.setEventHandler { [weak self] in self?.receivePackets() }
        reader.setCancelHandler { close(descriptor) }
        reader.resume()
        // Each worker owns its HEV descriptor until the blocking function exits.
        let tunnelFD = hevFD
        worker.async { [self] in
            let config = """
            tunnel:
              mtu: 1500
              ipv4: 198.18.0.1
              ipv6: 'fd00:6e61:696c::1'
            socks5:
              address: 127.0.0.1
              port: 10808
              udp: 'udp'
            misc:
              task-stack-size: 32768
              tcp-buffer-size: 32768
              max-session-count: 128
              log-level: error
              log-file: /dev/null
            """
            _ = config.utf8CString.withUnsafeBufferPointer { bytes in
                hev_socks5_tunnel_main_from_str(UnsafeRawPointer(bytes.baseAddress!).assumingMemoryBound(to: UInt8.self), UInt32(bytes.count - 1), tunnelFD)
            }
            close(tunnelFD)
            queue.async { [self] in
                hevFD = -1
                let unexpected = !stopped
                stopped = true
                source?.cancel(); source = nil; appFD = -1
                let callbacks = stopWaiters; stopWaiters = []
                callbacks.forEach { $0() }
                if unexpected { failure() }
            }
        }
        queue.async { [self] in readPackets() }
    }

    func stop(completion: @escaping () -> Void) {
        queue.async { [self] in
            stopped = true
            source?.cancel(); source = nil; appFD = -1
            if hevFD < 0 { completion(); return }
            stopWaiters.append(completion)
            // HEV also handles quit arriving before its initialization finishes.
            hev_socks5_tunnel_quit()
        }
    }

    private func readPackets() {
        guard !stopped else { return }
        flow.readPackets { [weak self] packets, families in
            guard let self else { return }
            self.queue.async {
                guard !self.stopped, self.appFD >= 0 else { return }
                for (packet, family) in zip(packets, families) where packet.count <= 65535 {
                    guard family.int32Value == AF_INET || family.int32Value == AF_INET6 else { continue }
                    var value = family.uint32Value.bigEndian
                    var framed = withUnsafeBytes(of: &value) { Data($0) }
                    framed.append(packet)
                    _ = framed.withUnsafeBytes { send(self.appFD, $0.baseAddress, $0.count, 0) }
                }
                self.readPackets()
            }
        }
    }

    private func receivePackets() {
        guard !stopped, appFD >= 0 else { return }
        var buffer = [UInt8](repeating: 0, count: 65540)
        var packets: [Data] = []; var protocols: [NSNumber] = []
        // Bound each dispatch turn to keep stop requests responsive.
        for _ in 0..<32 {
            let count = recv(appFD, &buffer, buffer.count, 0)
            if count < 0 { break }
            if count <= 4 { continue }
            let family = buffer.prefix(4).reduce(UInt32(0)) { ($0 << 8) | UInt32($1) }
            guard family == UInt32(AF_INET) || family == UInt32(AF_INET6) else { continue }
            packets.append(Data(buffer[4..<count])); protocols.append(NSNumber(value: family))
        }
        if !packets.isEmpty { _ = flow.writePackets(packets, withProtocols: protocols) }
    }
}
