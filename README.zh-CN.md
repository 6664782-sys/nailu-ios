# 奈鹿 NaiLu — iOS 原生客户端（AnyTLS）

这是新的 SwiftUI iPhone / iPad 客户端工程，与现有 Windows、Android、Mac 客户端分别维护。默认连接奈鹿现有 API，不修改线上网站、节点、DNS 或其他站点。

## 已写入工程

- 奈鹿薄荷绿界面、品牌插画、首页 / 节点 / 我的三个页面。
- 原生邮箱密码登录、邮箱验证码注册、邀请码、条款同意。
- 账户、套餐、有效期、剩余流量（统一 GB）、订阅更新。
- **以奈鹿现有 AnyTLS 节点为主要连接协议**：AnyTLS v2 标准 TLS TCP、SHA-256 认证、会话复用、padding 更新、TCP 和 UOT v2 的 UDP/DNS 转发。对外仅接收 AnyTLS 节点；其他协议会报告为不支持。
- iOS Network Extension 扩展、真实系统连接状态、钥匙串共享凭据。
- 独立实现的本地 SOCKS5 桥接 + HEV 数据包桥接；只使用公开 packetFlow API，不读取系统私有文件描述符。
- IPv4 / IPv6 默认路由、隧道内 DNS；连接前解析加密服务器，固定该次连接 IP 并保留 TLS SNI。
- 连接前的数据使用说明、隐私清单初稿、开源许可展示。

## 当前验证状态

本分支替换了旧版 Xray 核心，采用独立 AnyTLS 客户端与有连接数量、握手和空闲超时限制的 SOCKS5 桥接。TCP 与 UDP 均经 AnyTLS 转发，不通过桥接直连目标域名。

新实现已通过本地 Go 单元测试、vet、SOCKS TCP/UDP 收发、关闭、非法请求与 UDP 来源限制测试，以及与固定版本独立 AnyTLS 参考服务端的加密互通。2026-10-31 [云端构建 #6 全部通过](https://github.com/6664782-sys/nailu-ios/actions/runs/41163645570)：Xcode 26.6 / iOS SDK 26.5，Go race、参考服务端 TCP/UDP 互通、4 项共享 Swift 测试、模拟器构建、未签名 iPhone 归档及实际模拟器启动截图均通过。公开分支源码与本地 45 个文件逐个核对一致，另完成 20 轮 SOCKS 桥接测试。

下载后已核对整个产物和内部 IPA / 模拟器包的 SHA-256，以及 App 和扩展的 arm64 iOS 可执行文件。两份可执行文件的 Go 模块信息仅包含 `golang.org/x/mobile` 与本工程引擎，未检出旧 Xray、sing、sing-shadowsocks 标记。未签名 IPA 为 6,804,592 字节。这些检查验证旧 Go 运行依赖已移除，不能代替完整发行许可审核或真机联网验收。

[前一版核心的构建任务](https://github.com/6664782-sys/nailu-ios/actions/runs/41119432628/job/124282059082) 已使用 Xcode 26.6 / iOS SDK 26.5 完成模拟器与未签名 iPhone 构建，但该产物仍包含旧核心，不能代表本分支已通过 iOS 验证。

**尚未进行 iPhone 真机 VPN 测试、Apple 签名、TestFlight 或 App Store 提交。未签名 IPA 不能直接安装到 iPhone。** 模拟器启动不等于真实账户登录、注册或 VPN 流量通过验收。

本分支已移除 Xray、sing 和 sing-shadowsocks 依赖，重新生成许可清单。已检查本次二进制内的 Go 模块信息；发行前仍须完成完整二进制及全部依赖的许可复核。


## 在 Mac 上构建


需要完整 Xcode、iOS SDK、Go 1.27.1 或更新兼容版本、Git、Make 和 XcodeGen。当前云端使用标准 `macos-26`，检查 Xcode 主版本至少为 26，以满足当前 App Store 最低 SDK 要求。先在 Xcode 完成首次启动和 SDK 下载。使用 Homebrew 时可安装 `go` 与 `xcodegen`，无需 Apple 账号即可先编译模拟器。

```sh
cd nailu-ios
bash scripts/build-macos.sh
open NaiLu.xcodeproj
```

脚本会：按 go.sum 下载依赖 → 测试 Go 引擎 → 生成 iOS / 模拟器 XCFramework → 从固定提交构建 HEV 静态库及子模块 → 收集许可 → 运行共享 Swift 测试 → 生成 Xcode 工程 → 编译模拟器。下载依赖需要网络；不会自动购买、签名、提交或上传应用。

原生 VPN 扩展需要在 iPhone 真机验证，模拟器编译成功不能证明实际 VPN 可用。在 Xcode 为 NaiLu、PacketTunnel 两个 target 选择同一开发团队；账号申请后按实际团队确认 App ID、Network Extension 和共享钥匙串授权。项目默认 Bundle ID 为 `pro.nailu.ios` / `pro.nailu.ios.PacketTunnel`；更改时须同步共享组、Info 字段与 KeychainStore 校验。

未签名模拟器仅在本次运行的内存中保存调试凭据，重启后需要重新登录，不写入普通文件或 UserDefaults；点击连接会明确提示需要真机。真实设备始终使用签名授权的共享钥匙串，不采用此模拟器分支。

## 首版的边界

- 仅邮箱登录。Google / Telegram / Apple 登录暂未加入；没有伪装成已完成的按钮。
- 不含购买、续费、付款入口；先处理已有套餐账户连接。后续收费方案需在目标地区按审核规则设计。
- AnyTLS 当前支持 **v2 + 标准 TLS TCP**；不支持 AnyTLS v1-only 服务端、Reality、WebSocket、ECH、证书 pin 参数及浏览器 TLS 指纹仿真。URI 的 `fp=chrome` 不代表本客户端会伪装 Chrome 指纹。
- AnyTLS 节点即使带 `insecure=1` 也会执行系统证书和 SNI 验证；有效证书可正常连接，自签名/过期/域名不符的证书会失败。没有关闭验证来强行连通，也没有修改线上节点证书。
- 不支持 Hysteria2 / TUIC / VLESS 等其他协议、Trojan / Shadowsocks。
- 暂无测速、按需连接、自动节点切换、规则分流、Widget。连接异常时由用户断开后重新连接。
- 当前为全局隧道，不包含局域网绕过规则。只排除所选加密服务器的单个地址。
- Go 24 MiB 是软内存目标，不代表扩展总内存已符合设备限制；HEV 会另外使用内存。
- 无所谓“iOS IPv6 已关闭”的承诺；本工程接管 IPv6 流量，必须真机测试不能泄漏。它不会修改服务器 IPv6 配置。

## 真机验收清单

1. 正常 / 错误密码登录、验证码注册、邀请码、验证码错误 / 超时 / 重发限制、注册流程过期后的重新打开。
2. 登录恢复、会话过期、离线重试、无套餐 / 用完 / 到期；确认服务器计费统计正确。
3. AnyTLS 的 TCP、UDP、域名访问、DNS、IPv4、IPv6、IPv6-only / NAT64 网络。
4. Wi-Fi / 移动网络切换、锁屏、后台、飞行模式、反复连接 / 取消、强制终止扩展。
5. 证书失效必须连接失败，DNS 不应走直连兜底；观察断线时系统路由与隐私表现。
6. 扩展持续运行峰值内存、CPU、耗电、低内存退出；目前没有测试数据可宣称稳定。
7. iPhone / iPad、深色模式、动态字体、VoiceOver、横竖屏与实际品牌图片裁切。

## AnyTLS 互通测试复现

```sh
bash scripts/test-anytls-interop.sh
```

脚本在 `.build` 创建独立测试目录，下载锁定提交的 AnyTLS 参考服务端，仅在本机使用临时测试证书、虚构密码和 TCP/UDP echo 服务。它不会访问生产节点或把参考服务端链接到 App。自研客户端按公开协议文档实现；v2 握手完成后才允许 VPN 扩展报告启动成功。仍需使用真实奈鹿节点在 iPhone 验证证书与实际网络适配。

## 上架前仍需完成

- 申请符合 VPN App 要求的组织类型 Apple Developer 账号，确定发行地区及当地许可要求；不能保证审核通过。
- 对照服务器真实的数据处理填写 App Store 隐私标签与政策；当前 PrivacyInfo 是客户端初稿，需复核 Go/HEV 依赖使用的 required reason API。
- 实现并验收账户删除功能（当前仅退出登录，不等于注销账号），确认订阅、余额、账单如何处理。
- 若加入 Google 等第三方主登录，再按 Apple 规则设计相应等效登录选项。
- 补齐审核专用可用账号、审核说明、支持信息、隐私链接、商店素材、出口合规声明及所需地区材料。
- 完成源码和最终二进制许可审查，保留全部依赖声明及许可原文。
- 当前沿用品牌 RGBA 图标。提交商店前需导出无 alpha 的 App Store 图标，并在真机检查系统图标裁切；未把它标记为已完成的上架素材。
- Mac 编译、iPhone 真机测试、签名归档、TestFlight 验收通过后，再提交 App Store。

官方参考：[审核指南 5.4 VPN Apps](https://developer.apple.com/app-store/review/guidelines/#vpn-apps)、[Packet Tunnel Provider](https://developer.apple.com/documentation/networkextension/nepackettunnelprovider)、[隐私清单](https://developer.apple.com/documentation/bundleresources/privacy-manifest-files)。





