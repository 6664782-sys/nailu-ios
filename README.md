# NaiLu native iOS client

SwiftUI iPhone/iPad development client for NaiLu, with AnyTLS v2 over standard TLS, a Network Extension packet tunnel, email account flows and account/traffic screens.

[中文说明、功能范围与真机验收项目](README.zh-CN.md)

## Verified cloud build

[Run 41119432628 succeeded](https://github.com/6664782-sys/nailu-ios/actions/runs/41119432628) on 2026-10-31, using a standard macOS 26 runner, Xcode 26.6 and iOS SDK 26.5.

- Go race tests and independent AnyTLS reference-server TCP/UDP interoperability passed.
- Four shared Swift tests passed.
- The simulator app and unsigned iPhone archive built successfully.
- The app was installed and launched in the simulator; the build includes a real login-screen screenshot.

Artifacts contain an **unsigned development IPA**, a simulator app, a screenshot, SHA-256 checksums and build logs. The IPA cannot be installed directly on an iPhone without appropriate Apple signing and provisioning. Simulator launch does not verify real account registration, production node connectivity or device VPN operation. No TestFlight or App Store submission has been performed.

## Release hold

The project still requires signed physical-device testing, account deletion, final privacy and binary/license review. In particular, Xray's MPL-2.0 top-level license does not resolve the licensing of its GPL-3.0-or-later `sing` and `sing-shadowsocks` dependencies. Those dependencies are used by the engine, not just by the separate reference test server. Applicable permissions and compatibility must be resolved, or the dependencies replaced, before selecting a distribution plan. Public source and a successful development build do not establish App Store distribution readiness.

See [third-party notices](THIRD_PARTY_NOTICES.md) and the bundled license inventory. No production credentials, customer data or signing materials are included.

