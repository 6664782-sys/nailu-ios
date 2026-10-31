# NaiLu native iOS client

SwiftUI iPhone/iPad development client for NaiLu, supporting AnyTLS v2 over standard TLS, a Network Extension packet tunnel, email accounts and account/traffic screens.

[中文说明、功能范围与真机验收项目](README.zh-CN.md)

## Independent AnyTLS candidate

This branch replaces Xray with an independently implemented, bounded loopback SOCKS5 bridge and the project's AnyTLS client. Runtime traffic passes through AnyTLS; the bridge does not directly resolve or connect to destination hosts. The app accepts only AnyTLS nodes.

Local Go tests, vet, SOCKS TCP/UDP lifecycle and request validation, and encrypted interoperability with the pinned independent AnyTLS reference server have passed. **Cloud race tests, iOS compilation and simulator launch of this replacement are still pending.**

The [previous engine's build job](https://github.com/6664782-sys/nailu-ios/actions/runs/41119432628/job/124282059082) passed using Xcode 26.6 / iOS SDK 26.5. Those existing artifacts contain the previous engine and do not validate this branch.

## Development and release

Run `bash scripts/build-macos.sh` on a Mac with full Xcode, Go 1.27.1 and XcodeGen. The workflow uses a standard macOS 26 runner and produces an unsigned development IPA, simulator app, screenshot, checksums and logs. An unsigned IPA cannot be installed directly on an iPhone without appropriate Apple signing and Network Extension provisioning.

Signed physical-device VPN and account testing, account deletion, privacy review and final binary/license review remain required before distribution. No TestFlight or App Store submission has occurred. Public source and a successful development build do not establish distribution readiness.

See [third-party notices](THIRD_PARTY_NOTICES.md). No production credentials, customer data or signing materials are included.
