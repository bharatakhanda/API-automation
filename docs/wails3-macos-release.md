# Wails 3 macOS build and release gate

This document defines the native macOS path for the separate **API Automation Preview** application. Gio remains the production fallback until the controlled Fiery checklist and signed/notarized field cycles pass.

## Supported package and identity

- Minimum declared OS: macOS 13.0.
- Release package: one universal application containing `arm64` and `x86_64` slices.
- Bundle name: `API Automation Preview.app`.
- Bundle identifier: `com.fiery.api-automation.preview`.
- Executable: `api-automation-wails-preview`.
- Runtime/configuration identity: `API Automation Wails Preview`.
- Wails runtime and generator: exactly `v3.0.0-beta.16`.

The separate preview identity prevents macOS data from colliding with Gio or a future production Wails identity. Go's `os.UserConfigDir` places preview presets, captures, diagnostics, results, and exports below `~/Library/Application Support/API Automation Wails Preview`. No password, session cookie, or authenticated client is serialized there. Credentials remain in Go memory for the current process only, so this stage does not create a Keychain migration requirement. Any future credential persistence must use Keychain rather than files or frontend storage.

## Continuous native compile gate

`.github/workflows/wails-macos-preview.yml` runs on native Apple Silicon and Intel GitHub runners. It verifies:

1. The pinned Wails CLI.
2. Platform-neutral Go tests, race tests, vet, module verification, and tidy state.
3. The locked dependency-free frontend, audit, syntax, no-source-map policy, and generated bindings.
4. Native `arm64` and `x86_64` Mach-O builds.
5. A universal application bundle assembled with `lipo` and a validated `Info.plist`.

The workflow uploads unsigned short-retention artifacts only. Passing CI proves native compilation and packaging; it does not prove signing, notarization, UI behavior, Fiery connectivity, or destructive administration safety.

## Native release-capable builds

Run each architecture on matching macOS hardware with the configured secret supplied only through the process environment:

```bash
export API_AUTOMATION_SECRET_KEY='operator-provided-value'
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16
tools/build-wails-macos.sh
unset API_AUTOMATION_SECRET_KEY
```

The script disables shell tracing, validates the CLI pin, regenerates bindings, builds frontend assets, scans those assets for the configured secret, creates the native production-tag binary, and verifies that the backend secret is embedded without printing it. Do not run `go version -m` on this binary because linker settings can disclose the secret.

Move the two resulting binaries into one trusted macOS workspace as:

```text
dist/api-automation-wails-preview-arm64
dist/api-automation-wails-preview-amd64
```

## Universal package, signing, and notarization

Unsigned internal package:

```bash
tools/package-wails-macos.sh 0.1.0 1
```

Developer ID signing with hardened runtime:

```bash
export MACOS_SIGNING_IDENTITY='Developer ID Application: Example Company (TEAMID)'
tools/package-wails-macos.sh 0.1.0 1
unset MACOS_SIGNING_IDENTITY
```

Signing and notarization using an operator-created `notarytool` Keychain profile:

```bash
export MACOS_SIGNING_IDENTITY='Developer ID Application: Example Company (TEAMID)'
export MACOS_NOTARY_PROFILE='api-automation-notary'
tools/package-wails-macos.sh 0.1.0 1
unset MACOS_NOTARY_PROFILE MACOS_SIGNING_IDENTITY
```

The packaging script uses hardened runtime, verifies the signature, waits for notarization, staples the ticket, validates the staple, and recreates the final ZIP. Certificates, private keys, App Store Connect credentials, Keychain profiles, and configured secrets must never be committed or uploaded as ordinary artifacts.

## Interactive macOS acceptance gate

Run these checks on both a supported Apple Silicon Mac and the supported Intel baseline where available:

- Launch from Finder after downloading the signed/notarized ZIP; Gatekeeper produces no override prompt.
- Confirm the app name, bundle identity, window lifecycle, native open-folder/open-file/save dialogs, and keyboard navigation.
- Confirm Application Support paths are distinct and contain no plaintext credential or cookie.
- Connect to an isolated Fiery using its expected self-signed TLS behavior; test wrong credentials and cancellation without credential leakage.
- Discover capabilities, inspect saved evidence, and verify direct `EFPageRange` plus the leading U+FEFF output-profile wire identity.
- Exercise one approved lifecycle case, cancellation, external Busy-to-Idle monitoring, wake/sleep recovery, server presets, manual job action confirmation, and a large Excel export.
- Exercise restart/reboot/clear only on an isolated target with explicit approved jobs and exact native confirmations.
- Compare Wails result rows, complete JSONL records, Excel output, logs, and Fiery state against Gio.

Record OS, CPU architecture, Fiery model/version, build hash, test data, operation IDs, and outcomes in the regression checklist. Any unsafe difference keeps Gio as production and blocks Stage 8 adoption.
