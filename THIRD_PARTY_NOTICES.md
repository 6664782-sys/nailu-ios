# 第三方组件说明

本工程的 SwiftUI 应用、Objective-C 数据包接口、Go SOCKS5 桥接及 AnyTLS 客户端为独立实现。未复制已有 FlClash / Flutter 客户端代码。品牌图片来自已有奈鹿资源。

本分支已移除 Xray 及其 sing / sing-shadowsocks 依赖。AnyTLS v2 客户端、padding、UOT 编解码与 SOCKS5 桥接按公开协议实现，没有链接 anytls-go / sing-anytls / mihomo / dae 的实现代码。

协议参考：
- AnyTLS 协议：<https://github.com/anytls/anytls-go/blob/fd6167acd6d73b9fa3e607659951847fbc9e6c50/docs/protocol.md>
- UOT：<https://sing-box.sagernet.org/configuration/shared/udp-over-tcp/>
- SOCKS5：<https://www.rfc-editor.org/rfc/rfc1928>

独立互通测试下载固定提交的 AnyTLS 参考服务端，仅作为测试程序运行，不链接或分发到 App。

分发组件：
- hev-socks5-tunnel，固定 `0166c4765e9e42eee7d18923026f88fda91bd9fa`，MIT：<https://github.com/heiher/hev-socks5-tunnel/tree/0166c4765e9e42eee7d18923026f88fda91bd9fa>。子模块由该提交锁定，保留各自许可。
- golang.org/x/mobile，固定 `v0.0.0-20260908204917-8b95e45f8d3e`，Go BSD 风格许可。其他构建工具依赖由 Engine/go.mod 和 go.sum 锁定。

构建时 `scripts/licenses/main.go` 收集模块根目录及 HEV 子模块许可原文到 `Resources/ThirdPartyLicenses.txt`，供应用内查看。当前重新生成的清单未检出 GPL / Xray / sagernet 条目。清单覆盖模块图，可能多于最终链接的包，不能替代最终二进制与许可证审查；仍须检查清单尾部的缺失许可项及真实发行产物。

本分支尚待云端 iOS 编译和真机验收。没有包含证书、RAM 密钥、客户订阅、用户令牌或签名材料。
