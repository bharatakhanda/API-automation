#!/usr/bin/env bash
set -euo pipefail
set +x

root="$(cd "$(dirname "$0")/.." && pwd)"
expected_wails="v3.0.0-beta.16"
arch="$(go env GOARCH)"
output="$root/dist/api-automation-$arch"
secret="${API_AUTOMATION_SECRET_KEY:-}"

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "Wails macOS builds must run natively on macOS" >&2
    exit 1
fi
if [[ "$arch" != "arm64" && "$arch" != "amd64" ]]; then
    echo "unsupported macOS architecture: $arch" >&2
    exit 1
fi
if [[ -z "$secret" ]]; then
    echo "API_AUTOMATION_SECRET_KEY is required for a release-capable build" >&2
    exit 1
fi
if [[ "$(wails3 version 2>&1)" != "$expected_wails" ]]; then
    echo "wails3 $expected_wails is required; install it with: go install github.com/wailsapp/wails/v3/cmd/wails3@$expected_wails" >&2
    exit 1
fi

cd "$root/cmd/api-automation/frontend"
npm ci --ignore-scripts
npm audit --package-lock-only
node --check src/app.js
node --check build.mjs
wails3 generate bindings -b -names -d src/bindings "$root/cmd/api-automation"
npm run build

SECRET_TO_SCAN="$secret" python3 - <<'PY'
import os
from pathlib import Path
secret = os.environ["SECRET_TO_SCAN"].encode()
for root in (Path("src"), Path("bundle")):
    for path in root.rglob("*"):
        if path.is_file() and secret in path.read_bytes():
            raise SystemExit(f"configured secret found in frontend asset: {path}")
PY

mkdir -p "$root/dist"
ldflags="-s -w -X api-automation/internal/fiery.DefaultSecretKey=$secret"
cd "$root"
go build -tags production -trimpath -ldflags "$ldflags" -o "$output" ./cmd/api-automation

BUILT_BINARY="$output" SECRET_TO_SCAN="$secret" python3 - <<'PY'
import os
from pathlib import Path
binary = Path(os.environ["BUILT_BINARY"]).read_bytes()
secret = os.environ["SECRET_TO_SCAN"].encode()
if secret not in binary:
    raise SystemExit("embedded backend secret verification failed")
PY

file "$output"
shasum -a 256 "$output" > "$output.sha256"
secret=""
ldflags=""
echo "Wails macOS application build: PASS; architecture=$arch; output=$output"
