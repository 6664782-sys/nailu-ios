import SwiftUI
import NetworkExtension

@main
struct NaiLuApp: App {
    @StateObject private var session = SessionStore()
    @StateObject private var tunnel = TunnelController()
    var body: some Scene {
        WindowGroup {
            RootView().environmentObject(session).environmentObject(tunnel)
                .tint(Palette.teal)
                .task { await tunnel.load(); await session.restore() }
        }
    }
}

enum Palette {
    static let teal = Color(red: 0.02, green: 0.47, blue: 0.44)
    static let ink = Color(red: 0.08, green: 0.22, blue: 0.22)
    static let mint = Color(red: 0.88, green: 0.95, blue: 0.91)
    static let cream = Color(red: 1, green: 0.97, blue: 0.91)
}

struct RootView: View {
    @EnvironmentObject private var session: SessionStore
    @EnvironmentObject private var tunnel: TunnelController
    var body: some View {
        Group {
            if session.restoring { ProgressView("正在恢复登录…") }
            else if session.account == nil { NavigationStack { LoginView() } }
            else {
                TabView {
                    NavigationStack { HomeView() }.tabItem { Label("首页", systemImage: "square.grid.2x2.fill") }
                    NavigationStack { NodesView() }.tabItem { Label("节点", systemImage: "globe.asia.australia.fill") }
                    NavigationStack { AccountView() }.tabItem { Label("我的", systemImage: "person.crop.circle") }
                }
            }
        }
        .alert("提示", isPresented: Binding(get: { session.error != nil || tunnel.error != nil }, set: { if !$0 { session.error = nil; tunnel.error = nil } })) {
            Button("知道了") { session.error = nil; tunnel.error = nil }
        } message: { Text(session.error ?? tunnel.error ?? "") }
    }
}

struct BrandHeader: View {
    var body: some View {
        HStack { Text("奈鹿 NaiLu").font(.headline).foregroundStyle(Palette.teal); Spacer(); Image(systemName: "leaf.fill").foregroundStyle(Palette.teal) }
    }
}

struct LoginView: View {
    @EnvironmentObject private var session: SessionStore
    @State private var email = ""
    @State private var password = ""
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 22) {
                BrandHeader()
                Image("mascot").resizable().scaledToFit().frame(height: 190).frame(maxWidth: .infinity).accessibilityHidden(true)
                Text("抬头，是更大的世界。").font(.largeTitle.bold()).foregroundStyle(Palette.ink)
                Text("欢迎回来，登录奈鹿继续你的每一程。").foregroundStyle(.secondary)
                TextField("邮箱地址", text: $email).textContentType(.username).keyboardType(.emailAddress).textInputAutocapitalization(.never).autocorrectionDisabled().padding().background(.white, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
                SecureField("密码", text: $password).textContentType(.password).padding().background(.white, in: RoundedRectangle(cornerRadius: 18, style: .continuous))
                Button { let value = password; password = ""; Task { await session.login(email: email, password: value) } } label: {
                    Group { if session.busy { ProgressView().tint(.white) } else { Text("登录奈鹿").bold() } }.frame(maxWidth: .infinity).padding()
                }.buttonStyle(.borderedProminent).disabled(session.busy || email.isEmpty || password.isEmpty)
                HStack {
                    NavigationLink("创建账号") { RegisterView() }
                    Spacer()
                    Link("忘记密码", destination: URL(string: "https://nailu.pro/#/forget")!)
                }
                HStack { Link("隐私说明", destination: NaiLuOrigin.privacy); Text("·"); Link("服务条款", destination: NaiLuOrigin.terms) }.font(.footnote).frame(maxWidth: .infinity)
            }.padding(24).frame(maxWidth: 520)
        }.frame(maxWidth: .infinity).background(LinearGradient(colors: [Palette.mint, .white, Palette.cream], startPoint: .topLeading, endPoint: .bottomTrailing))
    }
}

struct RegisterView: View {
    @EnvironmentObject private var session: SessionStore
    @State private var email = ""
    @State private var password = ""
    @State private var confirmation = ""
    @State private var emailCode = ""
    @State private var invitation = ""
    @State private var agreed = false
    @State private var sending = false
    @State private var codeSent = false
    var body: some View {
        Form {
            Section("创建奈鹿账号") {
                TextField("邮箱地址", text: $email).keyboardType(.emailAddress).textContentType(.emailAddress).textInputAutocapitalization(.never).autocorrectionDisabled()
                SecureField("密码，至少 8 位", text: $password).textContentType(.newPassword)
                SecureField("再次输入密码", text: $confirmation).textContentType(.newPassword)
                if session.registrationConfig?.is_email_verify == 1 {
                    TextField("邮箱验证码", text: $emailCode).keyboardType(.numberPad).textContentType(.oneTimeCode)
                    Button(codeSent ? "重新发送验证码" : "获取邮箱验证码") {
                        sending = true
                        Task { defer { sending = false }; do { try await session.sendEmail(email); codeSent = true } catch { session.error = error.localizedDescription } }
                    }.disabled(sending || email.isEmpty)
                }
                TextField("邀请码（选填）", text: $invitation).textInputAutocapitalization(.never).autocorrectionDisabled()
            }
            Section {
                Toggle("我已阅读并同意服务条款与隐私说明", isOn: $agreed)
                Link("阅读服务条款", destination: NaiLuOrigin.terms)
                Link("阅读隐私说明", destination: NaiLuOrigin.privacy)
            }
            Section {
                Button("注册并登录") {
                    let value = password; password = ""; confirmation = ""
                    Task { await session.register(email: email, password: value, code: emailCode, invitation: invitation) }
                }.disabled(!agreed || session.busy || session.registrationConfig == nil || password.count < 8 || password != confirmation || email.isEmpty)
            }
        }.navigationTitle("注册账号").task { await session.prepareRegistration() }
    }
}

struct HomeView: View {
    @EnvironmentObject private var session: SessionStore
    @EnvironmentObject private var tunnel: TunnelController
    @AppStorage("nailu.vpn-data-consent.v1") private var dataConsent = false
    @State private var showDisclosure = false
    var body: some View {
        ScrollView {
            VStack(spacing: 20) {
                BrandHeader()
                Link(destination: NaiLuOrigin.community) { Label("官方社区 · 和奈鹿一起出发", systemImage: "paperplane").frame(maxWidth: .infinity, alignment: .leading) }.padding(14).background(.white.opacity(0.85), in: Capsule())
                ZStack {
                    Image("hero").resizable().scaledToFill().frame(height: 345).clipped()
                    VStack(spacing: 14) {
                        Text("抬头，\n是更大的世界。").font(.title.bold()).frame(maxWidth: .infinity, alignment: .leading).foregroundStyle(Palette.ink)
                        Spacer()
                        Button {
                            if tunnel.running { tunnel.disconnect() }
                            else if !dataConsent { showDisclosure = true }
                            else { connect() }
                        } label: {
                            VStack(spacing: 9) { Image(systemName: "power").font(.system(size: 36, weight: .medium)); Text(tunnel.running ? "断开" : "连接").font(.headline) }.foregroundStyle(.white).frame(width: 130, height: 130).background(Palette.teal, in: Circle()).padding(10).background(Palette.teal.opacity(0.12), in: Circle())
                        }.disabled(tunnel.busy || tunnel.status == .disconnecting).accessibilityLabel(tunnel.running ? "断开奈鹿 VPN" : "连接奈鹿 VPN")
                        Text(tunnel.title).font(.title3.bold()).foregroundStyle(Palette.ink)
                    }.padding(22)
                }.clipShape(RoundedRectangle(cornerRadius: 28, style: .continuous))
                NavigationLink { NodesView() } label: {
                    HStack { Image(systemName: "globe"); VStack(alignment: .leading) { Text("节点选择").font(.caption).foregroundStyle(.secondary); Text(session.selected?.name ?? "请选择可用节点").font(.headline) }; Spacer(); Image(systemName: "chevron.right") }.padding().background(.white, in: RoundedRectangle(cornerRadius: 22, style: .continuous))
                }.foregroundStyle(Palette.ink)
                PlanCard()
                if session.unsupportedNodes > 0 { Text("有 \(session.unsupportedNodes) 条节点使用了首版尚未支持的协议，已隐藏。可选节点支持 AnyTLS 标准 TLS。").font(.footnote).foregroundStyle(.secondary) }
            }.padding(20).frame(maxWidth: 620)
        }.frame(maxWidth: .infinity).background(LinearGradient(colors: [Palette.mint, .white, Palette.cream], startPoint: .topLeading, endPoint: .bottomTrailing)).refreshable { await session.refreshFromUI() }
        .navigationBarHidden(true)
        .sheet(isPresented: $showDisclosure) {
            NavigationStack {
                VStack(alignment: .leading, spacing: 20) {
                    Label("连接前请了解", systemImage: "hand.raised.fill").font(.title2.bold())
                    Text("开启后，设备网络流量将经过你选择的奈鹿节点。客户端会向奈鹿服务器提交登录信息，并获取账号、套餐、用量和节点配置。")
                    Text("本版本不接入广告或第三方行为分析 SDK，不把连接目的地址写入客户端日志。服务器对数据的处理以奈鹿隐私说明为准。")
                    Link("阅读完整隐私说明", destination: NaiLuOrigin.privacy)
                    Spacer()
                    Button("我已了解，继续连接") { dataConsent = true; showDisclosure = false; connect() }.buttonStyle(.borderedProminent).frame(maxWidth: .infinity)
                    Button("暂不连接") { showDisclosure = false }.frame(maxWidth: .infinity)
                }.padding(26).navigationTitle("使用说明").navigationBarTitleDisplayMode(.inline)
            }.presentationDetents([.medium, .large])
        }
    }
    private func connect() {
        guard let node = session.selected, let plan = session.subscription else { tunnel.error = "请先刷新套餐并选择节点"; return }
        Task { await tunnel.connect(node: node, subscription: plan) }
    }
}

struct PlanCard: View {
    @EnvironmentObject private var session: SessionStore
    var body: some View {
        VStack(alignment: .leading, spacing: 12) {
            Text(session.subscription?.plan?.name ?? "我的套餐").font(.title3.bold())
            if let plan = session.subscription {
                Text("剩余流量：\(Traffic.gigabytes(plan.remaining)) / \(Traffic.gigabytes(plan.transfer_enable))").font(.subheadline)
                ProgressView(value: Double(plan.remaining), total: Double(max(1, plan.transfer_enable))).tint(Palette.teal)
                if let expiry = plan.expired_at, expiry > 0 { Text("有效期至 \(Date(timeIntervalSince1970: expiry).formatted(date: .numeric, time: .omitted))").font(.caption).foregroundStyle(.secondary) }
                else { Text("到期时间以账户套餐为准").font(.caption).foregroundStyle(.secondary) }
            } else { Text("暂未获取套餐信息，下拉刷新重试。").foregroundStyle(.secondary) }
        }.frame(maxWidth: .infinity, alignment: .leading).padding(20).background(Palette.cream, in: RoundedRectangle(cornerRadius: 24, style: .continuous)).foregroundStyle(Palette.ink)
    }
}

struct NodesView: View {
    @EnvironmentObject private var session: SessionStore
    @EnvironmentObject private var tunnel: TunnelController
    var body: some View {
        List {
            if tunnel.requiresStop { Section { Text("切换节点前请先断开连接。").foregroundStyle(.secondary) } }
            if session.nodes.isEmpty { Text("暂无可用节点，请检查套餐或刷新重试。").foregroundStyle(.secondary) }
            ForEach(session.nodes) { node in
                Button { session.selectedID = node.id } label: {
                    HStack { Image(systemName: "globe.asia.australia").foregroundStyle(Palette.teal); VStack(alignment: .leading) { Text(node.name).foregroundStyle(Palette.ink); Text(node.protocol.uppercased()).font(.caption).foregroundStyle(.secondary) }; Spacer(); if session.selected?.id == node.id { Image(systemName: "checkmark.circle.fill").foregroundStyle(Palette.teal) } }
                }.disabled(tunnel.requiresStop)
            }
        }.navigationTitle("选择节点").refreshable { await session.refreshFromUI() }.toolbar { Button { Task { await session.refreshFromUI() } } label: { Image(systemName: "arrow.clockwise") }.disabled(session.busy || tunnel.requiresStop) }
    }
}

struct AccountView: View {
    @EnvironmentObject private var session: SessionStore
    @EnvironmentObject private var tunnel: TunnelController
    @State private var showLogout = false
    var body: some View {
        List {
            Section { Label(session.account?.email ?? "奈鹿账户", systemImage: "person.crop.circle.fill") }
            Section("我的套餐") { PlanCard().listRowInsets(EdgeInsets()); Button("刷新账户与流量") { Task { await session.refreshFromUI() } }.disabled(session.busy) }
            Section("帮助与说明") {
                Link("在线客服与工单", destination: NaiLuOrigin.support)
                Link("奈鹿官方社区", destination: NaiLuOrigin.community)
                Link("隐私说明", destination: NaiLuOrigin.privacy)
                Link("服务条款", destination: NaiLuOrigin.terms)
                NavigationLink("开源组件与版本") { LicensesView() }
            }
            Section { Button("退出登录", role: .destructive) { showLogout = true }.disabled(session.busy || tunnel.requiresStop) } footer: { Text("退出前请先断开 VPN。") }
        }.navigationTitle("我的奈鹿").confirmationDialog("退出当前账号？", isPresented: $showLogout) { Button("退出登录", role: .destructive) { session.logout() } }
    }
}

struct LicensesView: View {
    @State private var notices = "正在读取许可…"
    var body: some View {
        ScrollView {
            VStack(alignment: .leading, spacing: 18) {
                Text("奈鹿 NaiLu · iOS 0.2.0 开发版").font(.headline)
                Text("连接引擎：Xray-core；数据包桥接：hev-socks5-tunnel。")
                Text(notices).font(.caption.monospaced()).textSelection(.enabled)
            }.padding()
        }.navigationTitle("开源许可").task {
            if let url = Bundle.main.url(forResource: "ThirdPartyLicenses", withExtension: "txt"), let text = try? String(contentsOf: url, encoding: .utf8) { notices = text }
            else { notices = "许可清单缺失，请重新运行 Mac 构建脚本。此版本不可对外发布。" }
        }
    }
}
