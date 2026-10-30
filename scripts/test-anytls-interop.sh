#!/bin/bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
mkdir -p "$ROOT/.build"
TREE="$(mktemp -d "$ROOT/.build/anytls-interop.XXXXXX")"
# The reference implementation is used only as an external test server. It is
# never copied into the application or linked into NailuEngine.xcframework.
git clone --no-checkout https://github.com/anytls/anytls-go.git "$TREE/reference"
git -C "$TREE/reference" checkout --detach fd6167acd6d73b9fa3e607659951847fbc9e6c50
cp "$ROOT/scripts/interop/reference_test.go" "$TREE/reference/cmd/server/nailu_interop_test.go"
cd "$TREE/reference"
go mod edit -go=1.27.1 -require=nailu/ios/engine@v0.0.0 "-replace=nailu/ios/engine=$ROOT/Engine"
go mod tidy
go test -run TestNailuReferenceInterop -v -timeout 40s ./cmd/server
