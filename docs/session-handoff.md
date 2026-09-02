# Session handoff

Updated: 2026-09-02

## Repository state

- Repository: `C:\Users\harsh\Desktop\API_Automation`
- Remote: `https://github.com/bharatakhanda/API-automation`
- Wails runtime and CLI: exactly `v3.0.0-beta.16`
- Canonical desktop command: `cmd/api-automation`
- Canonical Windows executable: `bin/api-automation.exe`
- Active desktop adapter: `internal/appwails`
- Authoritative platform-neutral use-case layer: `internal/application`

## Adoption decision

The operator confirmed that the Wails project works correctly and explicitly requested removal of Gio source. Wails is now the sole active desktop implementation. The legacy command, adapter, direct dependencies, fallback UI text, and side-by-side build identity were removed in a separate adoption change.

Rollback history remains available through:

- `gio-stable-20260902`
- `wails-core-extracted-20260902`
- `wails-preview-accepted-20260902`

See `docs/wails3-adoption-record.md` for the decision record. Historical migration details remain in `docs/wails3-migration-plan.md`.

## Current desktop behavior

- Connection uses an exact tested draft plus explicit Apply before workspace unlock.
- Credentials and authenticated clients remain private in Go and are never returned in DTOs.
- Overview combines Fiery status, bounded external-job probes, and active automation state.
- Capability discovery, normalization, taxonomy, constraints, exact values, numeric values, Copies, custom ranges, local presets, and read-only server presets feed backend-owned planning.
- Custom ranges are sent directly in `EFPageRange`; `DPP_PAGE_RANGE` is omitted.
- Non-default `EFOutProfile` preserves its leading U+FEFF on the wire.
- Runner events, cancellation, lifecycle outcomes, strict set/get evidence, complete JSONL records, and Excel export are backend-owned.
- Manual cancel/delete and restart/reboot/clear operations preserve shared interlocks and native confirmations.
- The exact expected-constraint baseline error remains: `constraint testing requires explicit incompatible Job Property values; Server Baseline sends no property updates`.
- The frontend consumes generated JSON names exactly, including `testOk` and `activeIpAddress`.

## Storage and diagnostics

On Windows, diagnostics are portable beside the executable:

- `bin/logs/api-automation-wails-*.log`
- `bin/captures/...`

Log names use timestamp, process ID, and a unique suffix so simultaneous instances cannot append to one file. Normal close records `Application exiting`.

Presets and disk-backed JSONL results intentionally retain the accepted application-data identity:

- `%APPDATA%\API Automation Wails Preview`

This internal compatibility path avoids destructive migration or lost operator history. It does not expose credentials.

## Build and verification

```powershell
# Install once or verify exact output: v3.0.0-beta.16
go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16

# Full secret-safe canonical Windows build
.\tools\build-windows.ps1

# Startup, no-shell-child, graceful-close smoke
.\.local\gui-smoke-test.ps1 -ExePath '.\bin\api-automation.exe'
```

Validated canonical Windows artifact:

- Size: `14,702,080` bytes
- UTC build timestamp: `2026-09-02T11:14:39.4439666Z`
- SHA-256: `CBD648A997607A0099DD7159C2DA61F29076CC9477213CB5531B87EE029069A6`
- Startup/no-child/WM_CLOSE smoke: **PASS**

The Windows build script:

- verifies the exact Wails CLI pin;
- generates name-based bindings;
- installs/builds the locked dependency-free frontend without source maps;
- scans frontend assets for configured-secret bytes;
- injects the default Fiery key only into the Go binary from ignored local storage;
- verifies PE GUI subsystem 2 and embedded-secret presence without printing the secret.

Never run `go version -m` on secret-injected executables.

Generic repeat-run result comparison:

```bash
go run ./cmd/compare-automation-results \
  -baseline /path/to/baseline.jsonl \
  -candidate /path/to/candidate.jsonl
```

## Remaining external release gates

- Controlled live Fiery checks still require the operator server, credentials, approved files, and isolated destructive-test environment. Use `docs/wails3-regression-checklist.md`.
- macOS builds must run natively. `.github/workflows/wails-macos.yml` builds arm64/x86_64 artifacts and a universal package; post-merge `main` run `33624425597` passed.
- Signed/notarized physical-Mac acceptance still requires Apple Developer credentials and supported Macs. Use `docs/wails3-macos-release.md`.

These are release-evidence gates; they do not restore or require duplicate desktop source.

## Repository hygiene

Never commit executables, credentials, `.local/`, `DATA/`, logs, captures, result stores, or exports. In Bash use `2>/dev/null`, never `2>NUL`.
