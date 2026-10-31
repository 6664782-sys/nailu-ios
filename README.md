# NaiLu native iOS client

SwiftUI iPhone/iPad development client for NaiLu, supporting AnyTLS v2 over standard TLS, a Network Extension packet tunnel, email accounts and account/traffic screens.

[中文说明、功能范围与真机验收项目](README.zh-CN.md)

## Independent AnyTLS engine

This branch replaces Xray with an independently implemented, bounded loopback SOCKS5 bridge and the project's AnyTLS client. Runtime traffic passes through AnyTLS; the bridge does not directly resolve or connect to destination hosts. The app accepts only AnyTLS nodes.

[Cloud build #6 passed](https://github.com/6664782-sys/nailu-ios/actions/runs/41163645570) on 2026-10-31 with Xcode 26.6 / iOS SDK 26.5. Go race tests, independent reference-server TCP/UDP interoperability, four shared Swift tests, simulator build, unsigned iPhone archive and actual simulator launch all passed. Local vet and 20 repeated SOCKS lifecycle test runs also passed.

Downloaded artifact checksums and both arm64 iPhone executables were verified. Go build metadata in the app and extension contains only `golang.org/x/mobile` and `nailu/ios/engine`; Xray, sing and sing-shadowsocks markers are absent. This verifies removal of the old Go runtime dependencies, not completion of the final distribution license review.

The [previous engine's build job](https://github.com/6664782-sys/nailu-ios/actions/runs/41119432628/job/124282059082) passed using Xcode 26.6 / iOS SDK 26.5. Those existing artifacts contain the previous engine and do not validate this branch.

## Development and release

Run `bash scripts/build-macos.sh` on a Mac with full Xcode, Go 1.27.1 and XcodeGen. The workflow uses a standard macOS 26 runner and produces an unsigned development IPA, simulator app, screenshot, checksums and logs. An unsigned IPA cannot be installed directly on an iPhone without appropriate Apple signing and Network Extension provisioning.

Signed physical-device VPN and account testing, account deletion, privacy review and final binary/license review remain required before distribution. No TestFlight or App Store submission has occurred. Public source and a successful development build do not establish distribution readiness.

See [third-party notices](THIRD_PARTY_NOTICES.md). No production credentials, customer data or signing materials are included.

