# API Automation

Wails 3 desktop automation for Fiery job import, lifecycle execution, attribute updates, and readback verification. Connection, discovery, planning, execution, results, and guarded administration are backed by the authoritative Go application core; the frontend renders generated service DTOs and does not implement Fiery protocol semantics.

## Capabilities

- Authenticates with Fiery v5 (`apikey`) and retains v4 compatibility fallback (`accessrights`).
- Discovers server, queue, and job-property capabilities.
- Imports supported test files and runs Hold, RIP, Ready-to-Print, Press-Print, and Print lifecycles.
- Generates a single configuration, bounded Cartesian combinations, memory-safe pairwise coverage, or a bounded random sample from independently chosen value sources.
- Uses bounded worker concurrency from 1 through 10, cancellation, condition-based polling, and final set/get verification.
- Saves diagnostics, raw API/capability snapshots, and normalized schema-v3 filter audits beside the executable under `logs/` and `captures/`; each audit reconciles raw, normalized, and exact UI-displayed properties with inclusion/exclusion reasons.
- Uses direct sidebar navigation for Connection, Overview, Test Settings, Job Properties, Automation, and Results, with Activity Logs and Administration kept separate; wizard steps, numbering, arrows, and previous/next footers are intentionally absent.
- Gates the workspace behind a successful connection test plus explicit **OK**; changing servers is staged, so the current connection remains active until its replacement passes and is applied.
- Provides a compact Overview dashboard with matching blue header actions, Server Details without disk/memory rows, one-second `/status` polling, and—after capability discovery—a bounded recent-job workload probe. Jobs started from CWS or another client and active application automation both override stale API `running/none` responses to Busy; every poll is recorded diagnostically.
- Provides visible vertical scrolling for long workspace pages and exports completed runs to formatted `.xlsx` workbooks with Summary and Results sheets.
- Identifies the connected press from server property/queue metadata and filters the broad Fiery PPD schema before populating Job Properties. A control must have both documented CWS Job Properties taxonomy mapping and affirmative server metadata; backend-only, ungrouped, configuration, disabled/hidden, context-only, one-value, duplicate-alias, and installed-option-disabled entries remain diagnostic exclusions.
- Organizes applicable writable features into PDF-aligned Job Info, Substrate/Media, Layout, Color, Image, Finishing, and VDP tabs. Capabilities are vertically stacked under Fiery-style nested headings; Quick Access is intentionally excluded.
- Provides feature search, scoped resets, and category/capability-level Select all controls for eligible checkbox values.
- Renders eligible Fiery `efirange` properties as validated numeric inputs using server min/max/increment/precision metadata, provides optional application-standard Scale input, and accepts single values, comma-separated values, or inclusive ranges.
- Accepts Copies as individual values or inclusive ranges from 1 through 9999 and feeds them into single, Cartesian, pairwise, or bounded-random generation while respecting Max cases.
- Preserves every advertised `EFPageRange` value, including `Range1`, when custom text is blank. A non-empty validated custom range replaces those menu values for the plan and is sent directly as `EFPageRange`, preventing Single Configuration from silently sending bare `Range1`; `DPP_PAGE_RANGE` is never emitted.
- Preserves the leading `U+FEFF` in advertised non-default `EFOutProfile` values on the update wire so CWS can resolve the exact menu identity, while omitting that invisible character from labels and readback comparisons.
- Saves reusable local settings presets—including value source, test intent, generation strategy, selected properties, numeric values, run modes, and concurrency—without credentials or file paths, and validates restored values against the currently connected Fiery.
- Discovers Fiery server presets read-only through API v5, displays their IDs/advertised setting counts, and applies one selected preset to each imported job before explicit capability overrides. The application cannot create, edit, or delete server presets.
- Separates Positive Validation from Expected Constraint Rejection. Validation Only is the safe constraint default; an expected conflict is PASS, while timeout, endpoint failure, HTTP 500, unrelated rejection, or cleanup failure is ERROR. Controlled Apply is an explicit advanced option for disposable jobs.
- Treats Fiery property-constraint arrays as incompatible dependency values, filters only explicit matching conflicts from positive plans, and performs a cached job-specific Fiery constraint check before applying constrained settings when the endpoint is supported.
- Provides confirmed manual job actions plus separate Cancel-while-Processing/Ripping, Cancel-while-Waiting-to-Print, Cancel-while-Printing, and Delete automation modes; each selected mode imports its own job.
- Provides a separate Administration workspace for confirmed Fiery-process restart, full-server reboot, and guarded clear-all-jobs. Clearing requires a fresh job inventory, an exact typed phrase, a native confirmation containing server/count, immediate count revalidation, and verified empty readback; only the jobs service is requested.
- Evaluates Process-and-Hold/RIP success from final Fiery status, state, error, and raster/page evidence in addition to strict set/get matching; lifecycle failures remain failures even when ticket values match.
- Coordinates GUI shutdown with background cancellation and bounded waiting, and finalizes long-run result files without a blocking full-file sync.
- Keeps all normal GUI HTTP work in-process; the standalone readback probe alone may invoke curl for diagnostics.

## Build and verification

```powershell
go test ./...
go test -race ./...
go vet -all ./...
staticcheck ./...

# Requires the exactly pinned Wails CLI and ignored .local/secrets.json
.\tools\build-windows.ps1
.\.local\gui-smoke-test.ps1 -ExePath '.\bin\api-automation.exe'
```

The Wails runtime and CLI are pinned to `v3.0.0-beta.16`; verify/install the CLI with `go install github.com/wailsapp/wails/v3/cmd/wails3@v3.0.0-beta.16`. On Windows, `logs/` and `captures/` are created beside `api-automation.exe` for portable debugging. Presets and disk-backed results retain the accepted Wails application-data identity so promotion does not discard existing operator data. Native universal macOS build, signing, notarization, and acceptance are documented in `docs/wails3-macos-release.md`.

For repeat-run regression checks, compare complete baseline/candidate JSONL stores with `go run ./cmd/compare-automation-results -baseline <baseline.jsonl> -candidate <candidate.jsonl>`.

`internal/fiery.DefaultSecretKey` is empty in source. If a field build needs a default key, inject it with `-ldflags -X` from ignored local secret storage. Never commit credentials, `.local/`, `DATA/`, generated captures, logs, or executables.

## Project layout

```text
cmd/api-automation             Wails 3 desktop entrypoint and locked frontend
cmd/fiery-readback-probe       Standalone diagnostic probe
cmd/compare-automation-results Semantic baseline/candidate JSONL comparison tool
internal/appwails              Credential-safe Wails service/DTO adapter
internal/application           Platform-neutral planning, runner, state, lifecycle, safeguards, and events
internal/fiery                 Fiery HTTP client and capability discovery
internal/capabilities          Capability normalization and taxonomy
internal/combinations          Bounded Cartesian, pairwise, and random generation
internal/files                 Supported test-file selection
internal/preflight             Environment checks and snapshots
internal/resultcompare         Order-independent field-result comparison
tools                          Build, package, and diagnostic scripts
```
