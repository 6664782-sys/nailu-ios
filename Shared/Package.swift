// swift-tools-version: 5.9
import PackageDescription
let package = Package(name: "NaiLuShared", platforms: [.iOS(.v16), .macOS(.v13)], products: [.library(name: "NaiLuShared", targets: ["NaiLuShared"])], targets: [.target(name: "NaiLuShared", path: "Sources"), .testTarget(name: "NaiLuSharedTests", dependencies: ["NaiLuShared"])])
