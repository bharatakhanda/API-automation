#!/usr/bin/env bash
set -euo pipefail
set +x

version="${1:-0.1.0}"
bundle_version="${2:-1}"
root="$(cd "$(dirname "$0")/.." && pwd)"
dist="$root/dist"
arm64_binary="$dist/api-automation-arm64"
amd64_binary="$dist/api-automation-amd64"
app="$dist/API Automation.app"
executable="$app/Contents/MacOS/api-automation"
archive="$dist/api-automation-macos-universal.zip"

if [[ "$(uname -s)" != "Darwin" ]]; then
    echo "macOS packaging must run natively on macOS" >&2
    exit 1
fi
for binary in "$arm64_binary" "$amd64_binary"; do
    if [[ ! -f "$binary" ]]; then
        echo "missing native input: $binary" >&2
        exit 1
    fi
done
if [[ ! "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]]; then
    echo "version must use numeric major.minor.patch form" >&2
    exit 1
fi
if [[ ! "$bundle_version" =~ ^[0-9]+$ ]]; then
    echo "bundle version must be a positive integer" >&2
    exit 1
fi

rm -rf "$app" "$archive"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"
lipo -create "$arm64_binary" "$amd64_binary" -output "$executable"
chmod 0755 "$executable"
cp "$root/build/wails-macos/Info.plist" "$app/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleShortVersionString $version" "$app/Contents/Info.plist"
/usr/libexec/PlistBuddy -c "Set :CFBundleVersion $bundle_version" "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist"

architectures="$(lipo -archs "$executable")"
[[ "$architectures" == *arm64* && "$architectures" == *x86_64* ]] || {
    echo "universal executable is missing an architecture: $architectures" >&2
    exit 1
}

if [[ -n "${MACOS_SIGNING_IDENTITY:-}" ]]; then
    codesign --force --timestamp --options runtime \
        --entitlements "$root/build/wails-macos/entitlements.plist" \
        --sign "$MACOS_SIGNING_IDENTITY" "$app"
    codesign --verify --deep --strict --verbose=2 "$app"
fi

ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"

if [[ -n "${MACOS_NOTARY_PROFILE:-}" ]]; then
    if [[ -z "${MACOS_SIGNING_IDENTITY:-}" ]]; then
        echo "notarization requires MACOS_SIGNING_IDENTITY" >&2
        exit 1
    fi
    xcrun notarytool submit "$archive" --keychain-profile "$MACOS_NOTARY_PROFILE" --wait
    xcrun stapler staple "$app"
    rm -f "$archive"
    ditto -c -k --sequesterRsrc --keepParent "$app" "$archive"
    xcrun stapler validate "$app"
fi

shasum -a 256 "$archive" > "$archive.sha256"
echo "macOS universal package: PASS; architectures=$architectures; archive=$archive"
