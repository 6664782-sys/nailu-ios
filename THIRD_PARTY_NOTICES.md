# 第三方组件说明

本目录的 SwiftUI 应用、Objective-C 接口和 Go 包装层为独立新工程，没有复制既有 FlClash / Flutter 客户端的 GPL 实现。品牌图片来自本工作区已有奈鹿资源。

AnyTLS v2 客户端、padding、UOT 编解码和 Xray 出站接口由本工程按公开协议文档实现，没有链接 anytls-go / sing-anytls / mihomo / dae 的实现代码。协议参考：<https://github.com/anytls/anytls-go/blob/fd6167acd6d73b9fa3e607659951847fbc9e6c50/docs/protocol.md>、<https://sing-box.sagernet.org/configuration/shared/udp-over-tcp/>。参考服务端只由独立互通测试脚本下载运行，不随 App 分发。

- Xray-core，固定 `v1.260327.0`，MPL-2.0：<https://github.com/XTLS/Xray-core/tree/v1.260327.0>。本工程不修改其上游源文件。发布时保留 MPL 声明与该版本源代码获取方式。
- **发行许可待解决**：实际 Xray 依赖中的 `github.com/sagernet/sing v0.5.1` 和 `github.com/sagernet/sing-shadowsocks v0.2.7` 根许可是 GPL-3.0-or-later；它们不是仅用于独立参考测试的组件。源码和许可原文已收录，但尚未确认适用的额外授权及整个 App 的发行许可兼容性。不能依据 Xray 顶层的 MPL-2.0 声明判断整个 App 可闭源或可直接上架。发行前需解决适用授权，或替换这些依赖。源码公开及开发构建成功不等于许可审核完成。
- hev-socks5-tunnel，固定 `0166c4765e9e42eee7d18923026f88fda91bd9fa`，MIT：<https://github.com/heiher/hev-socks5-tunnel/tree/0166c4765e9e42eee7d18923026f88fda91bd9fa>。子模块由该提交锁定，分别保留许可。
- golang.org/x/mobile，固定 `v0.0.0-20260908204917-8b95e45f8d3e`，Go BSD 风格许可。其他 Go 模块由 Engine/go.mod 与 go.sum 锁定。

构建时 `scripts/licenses/main.go` 收集模块根目录及 HEV 子模块的许可原文到 `Resources/ThirdPartyLicenses.txt`，供应用内查看。自动清单覆盖模块图，可能多于最终链接的包；它不能代替最终二进制依赖与许可证审查。清单尾部列出未发现根许可的模块，必须在公开发布前逐个核实。

没有将任何证书、RAM 密钥、用户令牌、客户订阅或签名材料写入此工程。
