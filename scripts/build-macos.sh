#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
[[ "$(uname -s)" == Darwin ]] || { echo '需要 macOS 和完整 Xcode。'; exit 1; }
for tool in go git make xcrun xcodebuild xcodegen; do command -v "$tool" >/dev/null || { echo "缺少 $tool"; exit 1; }; done
xcrun --sdk iphoneos --show-sdk-path >/dev/null
mkdir -p "$ROOT/.build/bin" "$ROOT/Frameworks/Hev" "$ROOT/Resources"
export GOBIN="$ROOT/.build/bin"
export PATH="$GOBIN:$PATH"
cd "$ROOT/Engine"
go mod download
go test ./...
go vet ./...
go install golang.org/x/mobile/cmd/gobind golang.org/x/mobile/cmd/gomobile
gomobile init
gomobile bind -target=ios/arm64,iossimulator/arm64,iossimulator/amd64 -iosversion=16.0 -trimpath -ldflags='-s -w' -o "$ROOT/Frameworks/NailuEngine.xcframework" .

# Every architecture has a separate checkout, avoiding stale cross-platform objects.
HEV_COMMIT=0166c4765e9e42eee7d18923026f88fda91bd9fa
for tuple in 'iphoneos arm64' 'iphonesimulator arm64' 'iphonesimulator x86_64'; do
    read -r platform arch <<< "$tuple"
    tree="$ROOT/.build/hev-$platform-$arch"
    if [[ ! -d "$tree/.git" ]]; then git clone --no-checkout https://github.com/heiher/hev-socks5-tunnel.git "$tree"; fi
    git -C "$tree" checkout --detach "$HEV_COMMIT"
    git -C "$tree" submodule update --init --recursive
    sdk="$(xcrun --sdk "$platform" --show-sdk-path)"
    cc="$(xcrun --sdk "$platform" --find clang)"
    ar="$(xcrun --sdk "$platform" --find ar)"
    target="$arch-apple-ios16.0"; [[ "$platform" != iphonesimulator ]] || target="$target-simulator"
    make -C "$tree" -j2 static "CC=$cc" "PP=$cc -E" "AR=$ar" "CFLAGS=-target $target -isysroot $sdk"
    destination="$ROOT/Frameworks/Hev/$platform/$arch"
    mkdir -p "$destination"
    cp "$tree/bin/libhev-socks5-tunnel.a" "$destination/"
    cp "$tree/third-part/yaml/bin/libyaml.a" "$destination/"
    cp "$tree/third-part/lwip/bin/liblwip.a" "$destination/"
    cp "$tree/third-part/hev-task-system/bin/libhev-task-system.a" "$destination/"
done
for library in hev-socks5-tunnel yaml lwip hev-task-system; do
    cp "$ROOT/Frameworks/Hev/iphoneos/arm64/lib$library.a" "$ROOT/Frameworks/Hev/iphoneos/"
    xcrun lipo -create "$ROOT/Frameworks/Hev/iphonesimulator/arm64/lib$library.a" "$ROOT/Frameworks/Hev/iphonesimulator/x86_64/lib$library.a" -output "$ROOT/Frameworks/Hev/iphonesimulator/lib$library.a"
done
go run ../scripts/licenses/main.go "$ROOT"
cd "$ROOT/Shared"
swift test
cd "$ROOT"
xcodegen generate
xcodebuild -project NaiLu.xcodeproj -scheme NaiLu -configuration Debug -sdk iphonesimulator -destination 'generic/platform=iOS Simulator' -derivedDataPath .build/DerivedData CODE_SIGNING_ALLOWED=NO build
echo '模拟器构建完成。VPN 连接还需要签名后在 iPhone 真机测试。'
