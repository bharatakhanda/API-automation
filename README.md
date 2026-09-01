# API Automation

Windows desktop automation for Fiery job import, lifecycle execution, attribute updates, and readback verification. The GUI is written in Go with Gio; native Windows dialogs use Windigo.

## Capabilities

- Authenticates with Fiery v5 (`apikey`) and retains v4 compatibility fallback (`accessrights`).
- Discovers server, queue, and job-property capabilities.
- Imports supported test files and runs Hold, RIP, Ready-to-Print, Press-Print, and Print lifecycles.
- Generates selected, bounded Cartesian, or memory-safe pairwise test combinations.
- Uses configurable worker concurrency from 1 through 1000, cancellation, condition-based polling, and final set/get verification.
- Saves diagnostics and API/capability snapshots beside the executable under `logs/` and `captures/`.
- Provides visible vertical scrolling for long workspace pages and exports completed runs to formatted `.xlsx` workbooks with Summary and Results sheets.
- Organizes discovered features into Job info, Layout, Substrate, Color and Image, Finishing, VDP, Installable, and Advanced tabs instead of rendering one full capability catalog at once.
- Provides feature search plus header Reset and category/capability-level Select all controls for discovered checkbox values.
- Renders Fiery `efirange` properties as validated numeric inputs using server min/max/increment/precision metadata, provides optional Scale input, and accepts single values, comma-separated values, or inclusive ranges.
- Accepts Copies as individual values or inclusive ranges from 1 through 9999 and feeds them into selected, permutation, and pairwise generation while respecting Max cases.
- Renders Fiery `EFPageRange=Range1` as a custom page-range text field instead of a checkbox. Inputs such as `1,3,5-8` are normalized and validated against each imported file's original page count before the job update.
- Saves reusable local settings presets without credentials or file paths and validates restored values against the currently connected Fiery.
- Filters explicit local conflicts using discovered constraint metadata and performs a cached, job-specific Fiery constraint check before applying constrained settings when the endpoint is supported.
- Provides confirmed manual job actions plus separate Cancel-while-Processing/Ripping, Cancel-while-Waiting-to-Print, Cancel-while-Printing, and Delete automation modes; each selected mode imports its own job.
- Evaluates Process-and-Hold/RIP success from final Fiery status, state, error, and raster/page evidence in addition to strict set/get matching; lifecycle failures remain failures even when ticket values match.
- Coordinates GUI shutdown with background cancellation and bounded waiting, and finalizes long-run result files without a blocking full-file sync.
- Keeps all normal GUI HTTP work in-process; the standalone readback probe alone may invoke curl for diagnostics.

## Build and verification

```powershell
go test ./...
go test -race ./...
go vet -all ./...
staticcheck ./...
go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/api-automation.exe ./cmd/api-automation
```

`internal/fiery.DefaultSecretKey` is empty in source. If a field build needs a default key, inject it with `-ldflags -X` from ignored local secret storage. Never commit credentials, `.local/`, `DATA/`, generated captures, logs, or executables.

## Project layout

```text
cmd/api-automation             GUI entrypoint
cmd/fiery-readback-probe       Standalone diagnostic probe
internal/appgio                Gio desktop UI and Fiery workflow orchestration
internal/fiery                 Fiery HTTP client and capability discovery
internal/capabilities          Capability normalization and taxonomy
internal/combinations          Bounded Cartesian, pairwise, and random generation
internal/files                 Supported test-file selection
internal/preflight             Environment checks and snapshots
tools                          Diagnostic launch scripts
```
