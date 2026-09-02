# Session handoff

Updated: 2026-09-02

## Project

- Root: `C:\Users\harsh\Desktop\API_Automation`
- Repository: `https://github.com/bharatakhanda/API-automation`
- Stable branch: `main`
- Active branch: `feature/wails3-ui`
- Stable baseline tags: `gio-stable-20260902` and `wails-core-extracted-20260902`
- Production Gio EXE: `C:\Users\harsh\Desktop\API_Automation\bin\api-automation.exe`
- Wails preview EXE: `C:\Users\harsh\Desktop\API_Automation\bin\api-automation-wails-preview.exe`
- Do not commit or push `DATA/`, `.local/`, captures, logs, result files, executables, or secrets.
- The user has explicitly authorized logical commits and pushes throughout the backend extraction and Wails 3 introduction. Report after each completed stage with pending work and the next task.
- The Fiery secret key is injected into the EXE by linker flag from `.local/secrets.json`; it is not committed or printed.

## Git state at handoff

- `main` and `origin/main` include the integrated platform-neutral core at merge `042ad84`; generated executables, captures, logs, results, `.local/`, and secrets remain excluded.
- `gio-stable-20260902` marks the rollback point before extraction; `wails-core-extracted-20260902` marks the validated Gio checkpoint before introducing Wails.
- Latest implementation work fixes custom-range planning so non-empty text cannot silently send bare `Range1`, and combines `/status` with captured plus bounded recent-job workload evidence so externally started jobs show Busy.
- Wails migration architecture, invariants, stage gates, beta risk, and rollback policy are recorded in `docs/wails3-migration-plan.md`.
- Stages 1 through 5 and Stage 6 implementation are complete: the platform-neutral core is integrated, and the separate Wails 3 preview now covers connection, monitoring, files, capabilities, presets, planning, automation, results/export, logs, manual job actions, and administration while Gio remains production. Stage 6 live Fiery parity is pending.
- Implementation commits:
  - `1d4044a feat: add categorized constraint-aware capability metadata`
  - `c6f2a3e feat: add secure local settings preset store`
  - `373c96b feat: validate job settings with Fiery constraints`
  - `2be430a feat: evaluate Fiery lifecycle outcomes`
  - `8469edb feat: add feature tabs search ranges presets and lifecycle results`
  - `e304744 docs: document categorized constraint-aware automation`
  - `ab656bd feat: tighten Fiery capability filtering and diagnostics`
  - `4b0c1fd feat: improve Fiery workspace automation and live status`
  - `23b50dc fix: honor custom ranges and external Fiery workload`
  - `e56aa97 refactor: extract platform-neutral planning core`
  - `49d6ddd refactor: extract automation execution runner`
  - `2c443cf refactor: extract application state and safeguards`
  - `ca3fc4e refactor: complete core extraction integration`
  - `e398b77 feat: add read-only Wails 3 preview`
- The complete stable history and baseline tag are pushed to `origin`.

## Current implementation

### Platform-neutral planning core

- `internal/application.PlanRequest` snapshots capability metadata and frontend selections without widget or OS dependencies; returned plans own their axes/maps and do not mutate request inputs.
- The shared planner preserves selected/default/advertised/baseline value-source behavior, preferred fallback axes, numeric/Copies expansion, bounded randomization, custom page-range replacement, published-constraint filtering, and Max-cases limits.
- Exact `EFPageRange`, legacy-companion exclusion, `EFOutProfile` wire/display comparison, omitted-default readback, selected readback materialization, expected constraint-rejection classification, run modes, lifecycle policies, RIP-readback requirements, and worker/case limits are shared backend semantics.
- `application.Runner` uses narrow authenticated-client and result-recorder ports, owns the bounded worker pool and each import/update/lifecycle/readback/result path, and closes the JSONL recorder before publishing terminal state.
- Runner operations have stable IDs, cancellation, immutable request snapshots, non-blocking event dispatch, coalesced progress, and lossless result/terminal events. Event payloads exclude credential/header data.
- Deterministic fake-client tests cover Hold/RIP success, terminal lifecycle failures, strict set/get, server presets, expected-constraint cleanup, cancellation, per-job panic recovery, result-storage errors, worker bounds, and typed event delivery.
- Exact-draft connection testing/application and blank credential replacement are authoritative in `application.ConnectionState`; snapshots expose only safe configured flags and active address.
- Overview polling cadence, bounded 64-job probes, capped backoff, external/application Busy composition, and stale-update generations are shared application policy/state.
- Safe local-preset capture and reconciliation now canonicalize current wire values, migrate legacy page-range data, clamp limits, reject stale options/server presets/modes, and report skipped values before Gio mutates controls.
- Administration interlocks, server-bound expiring inventory leases, exact confirmation, count revalidation, clear acceptance, empty verification, and recovery status polling are headlessly tested application safeguards.
- `internal/appgio` converts Gio widget state to application DTOs, binds Fiery HTTP/session adapters, consumes typed events, invokes native confirmations, and retains adapter tests. No business logic was introduced in TypeScript.

### Wails preview

- Wails runtime and CLI are exactly pinned to `v3.0.0-beta.16`; the previous machine-wide alpha CLI was replaced before binding generation.
- `cmd/api-automation-wails` embeds a dependency-free, npm-lockfile-controlled frontend and uses the distinct `API Automation Preview` identity and `api-automation-wails-preview.exe` output.
- `internal/appwails.Service` exposes credential-safe DTOs for exact-draft connection state, bounded external-workload monitoring, native file selection, normalized capabilities, safe local presets, backend planning, automation events/cancellation, disk-backed results/Excel export, manual job actions, and guarded administration. Credentials and session cookies remain private Go state and are absent from response DTOs/assets.
- Generated name-based bindings use the bundled Wails runtime, are byte-for-byte reproducible, and include no source maps. `tools/build-wails-preview.ps1` verifies the CLI pin, lockfile install, frontend secret absence, Windows subsystem, and embedded backend secret without printing it.
- The frontend receives run modes, worker/case limits, and monitor cadence from Go. It snapshots selections into DTOs and does not reimplement planning, wire serialization, constraint, lifecycle, or administration semantics.
- Wails intentionally uses a simpler modern desktop visual system rather than copying Gio: restrained surfaces, compact 36px content-width actions, consistent 40px fields, fixed-height Browse buttons, responsive wrapping, visible focus, and unambiguous hidden/disabled/destructive states.
- Preview presets, JSONL results, and diagnostics use the distinct `API Automation Wails Preview` configuration directory; Wails does not mutate Gio's existing settings.
- Automation drains typed shared-runner events, retains at most 500 live rows/1,000 live logs, preserves all result records on disk, and waits up to five seconds for cancellation/finalization during shutdown.

### Workspace UX

- Connection, Overview, Test Settings, Job Properties, Automation, and Results use direct sidebar navigation; Activity Logs and Administration are separate. Guided Workflow text, navigation arrows/numbers, step counters, and previous/next footers are removed.
- No non-Connection page is accessible until the exact draft passes Test Connection and the user presses OK.
- Existing passwords and secret/API keys are never repopulated into editors. The UI shows only Configured and accepts an optional replacement.
- Server replacement is staged; the active connection remains authoritative until the tested draft is explicitly applied. A true server/credential change invalidates capabilities, server presets, Job Property widgets, job IDs, inventory, and cached health while preserving test files, automation choices, saved local presets, results, and logs.
- Overview removes prior generic/promotional/footer and disk/memory elements. Its two header buttons match. One session-reusing monitor polls documented `/live/api/v5/status` every second while visible. After capability discovery it initializes from captured job workload and probes a bounded 64-job tail every two seconds. `OVERVIEW_STATUS_POLL`, `CAPABILITY_JOB_WORKLOAD`, and `OVERVIEW_JOB_POLL` retain the evidence; external jobs and active application automation display Busy even if API health is stale `running/none`, while failures or ignored pagination back off.
- Job Property categories use a compact fixed-width single horizontal row with count badges and narrow-window overflow. Local and Fiery preset controls appear once above the category row.
- Automation independently selects Positive Validation or Expected Constraint Rejection; Server Baseline, Advertised Defaults, User-Selected Values, or All Advertised Values; and Single Configuration, All Combinations (Cartesian), Pairwise, or Bounded Random Sample.
- Validation Only is the default constraint mode. Expected conflict is PASS, no conflict is FAIL, and timeout, unavailable endpoint, HTTP 500, unrelated rejection, server failure, or disposable-job cleanup failure is ERROR. Controlled Apply is explicitly advanced and accepts only evidenced client-side constraint rejections.

### Capability UX

- Fiery `/properties` is broad schema. The normalizer now requires both documented CWS Job Properties mapping and affirmative direct server metadata plus a meaningful choice; backend-only/unmapped controls cannot leak through category fallbacks. All prior configuration, visibility, context, one-value, family, and alias checks remain.
- The tracked NZ-1000 regression yields 78 usable/displayed controls (including two synthetic), 187 retained exclusions, zero removed values, and 24 constrained properties. Eight latest-capture leaks are now excluded: `EFHTGraphics`, `EFHTImages`, `EFHTText`, `EFMarginZero`, `EFPDFPreflightProfile`, `EFUseAPPE`, `EFUseSPDMediaMapping`, and `EFRaster`. Schema-v3 reports/logs retain all exact decisions.
- Applicable features are grouped into Job Info, Substrate/Media, Layout, Color, Image, Finishing, and VDP tabs. Quick Access is excluded.
- Controls are vertically stacked in Antares/Capella/Vela Job Properties order under nested headings such as Job notes, Reporting, Die printing, Color input/settings, Edge enhancement, Advanced, Barcode, and Delivery option.
- Search matches canonical labels, exact API keys, categories, current/default values, and advertised values across categories.
- Common aliases are deduplicated while preserving exact server IDs for updates and readback.
- `efirange` metadata retains min, max, increment, and precision and renders as a validated numeric field rather than checkboxes.
- Numeric fields accept one value, comma-separated values, or inclusive ranges and remain bounded by 10,000 expanded values and Max cases.
- Every advertised `EFPageRange` value, including `Range1`, remains available as an exact option when custom text is blank. A non-empty custom input is the sole page-range value for that plan, is normalized, checked against the imported file's original page count, sent directly as `EFPageRange`, and strictly read back from that field. `DPP_PAGE_RANGE` is never emitted or accepted as substitute verification.
- `EFOutProfile` preserves the server's leading U+FEFF on the update wire so CWS receives the exact advertised menu identity. The invisible code point is removed only from labels and readback/preset comparison.
- Capability capture now writes a normalized decision report beside the raw snapshot, including full retained metadata for excluded options and their reasons.
- Copies remains independently validated from 1 through 9999. Parallel Jobs is independently capped at 10 in the UI, preset restore, planning, and runtime worker pool.
- Scale is optional and blank by default. A discovered `Scaling`/`EFScale` property is authoritative; otherwise a standard optional `Scaling` input is available and Fiery's update response remains authoritative.

### Presets

- Local settings presets save selected values, numeric inputs, combination strategy, Max cases, worker count, file mode, run modes, and a safe Fiery server-preset ID.
- Presets explicitly exclude password, secret/API key, cookies, server address, and file paths.
- Storage is versioned and atomic under the user's configuration directory.
- Loading reconciles values, numeric limits, and server-preset IDs against the currently discovered Fiery and reports skipped stale values.
- Capability capture includes read-only `GET /live/api/v5/presets`. The UI can select one advertised preset and applies it to each stable spooled job before explicit capability overrides; there are no create/edit/delete controls.

### Server administration

- A separate Administration workspace provides Fiery-process restart, full-server reboot, job inventory, and clear-all-jobs.
- Restart/reboot are blocked during other operations, require native confirmation, and monitor recovery through re-login plus documented `/status`.
- Clear-all-jobs requests only `method=clear&services=jobs`. It requires a fresh inventory for the exact server, exact typed `CLEAR ALL JOBS`, a second confirmation showing server/count, immediate count revalidation, and verified empty inventory.
- Malformed or partial/paginated HTTP-200 job inventory responses are errors and can never be interpreted as zero or a misleadingly low job count.

### Constraints

- Published property constraints are retained from capability discovery. Fiery's arrays list incompatible dependency values; they are not allowlists.
- Combination generation filters only when an explicitly selected dependency equals one of those incompatible values. It does not assume missing/default dependency values are invalid.
- A bounded reserve of candidates prevents early conflicts from consuming the entire Max cases allowance.
- Before updating constrained settings, the app probes the imported job's Fiery constraint-check endpoint using POST then PUT compatibility fallback.
- Definite endpoint unavailability is cached; supported checks remain concurrent after the one-time probe; transient 5xx failures are not cached.
- The actual Fiery attribute-update response remains authoritative on older servers.

### Lifecycle verdicts

- PASS is now two-layered: the selected lifecycle policy must pass and all selected attributes must pass set/get verification.
- Process and Hold/RIP requires final `status=done ripping`, `state=processed`, no processing/PDL error, and raster/page evidence.
- Processing errors, unsupported PDL, invalid media/paper, cancellation, and abort evidence fail promptly instead of waiting for a timeout.
- Ready-to-Print, Press Print, intentional cancel, and delete modes have mode-specific policies.
- Result UI rows, diagnostics, API traces, JSONL records, and Excel exports include mode, lifecycle summary, final status, state, and processing error evidence.

## Validation

The following passed again after Stage 6 Wails Windows-parity implementation:

- `gofmt -w internal`
- `go test ./...`
- `go test -race ./...`
- `staticcheck ./...`
- `go vet -all ./...`
- `go mod verify`
- `go mod tidy -diff`
- `govulncheck ./...` — no called vulnerabilities
- Darwin ARM64 build of all platform-neutral `internal` packages
- No `os/exec`, `exec.Command`, or `curl` execution references in `internal/appgio` or `internal/fiery`
- Secret-injected Windows GUI build with `-trimpath -s -w -H=windowsgui`
- PE subsystem verification: Windows GUI (`Subsystem 2`)
- Embedded-secret byte verification without printing the key
- Gio GUI startup smoke test, no `cmd`/`curl`/PowerShell child process, graceful `WM_CLOSE`, exit code 0
- Exact Wails CLI/runtime `v3.0.0-beta.16` pin, deterministic bundled-runtime binding regeneration, npm lock/audit, JavaScript syntax, and no-source-map checks
- Secret-safe production-tag Wails preview build, visual inspection, no-shell-child startup, and graceful `WM_CLOSE`

Latest Wails preview checkpoint with the modern compact UI (Windows GUI subsystem 2, 14,699,520 bytes; UTC build timestamp `2026-09-02T07:42:45.5094850Z`) SHA-256:

`91B9F9A36F713C9205962B619513D369C9BA7080CF25DE3BFE0546F8BD858417`

Stage 6 Gio fallback checkpoint (Windows GUI subsystem 2, 17,332,224 bytes; UTC build timestamp `2026-09-02T06:57:58.8838041Z`) SHA-256:

`EC031951802FCA82946C3BF1D960C89856224B18799313072C5A5243519AF4A7`

Both Windows executables passed simultaneous side-by-side startup, no-shell-child inspection, and graceful close. The Wails preview also passed visual inspection, frontend lock/audit/syntax checks, byte-reproducible binding generation, and absence of frontend source maps/configured-secret bytes. Windows `CGO_ENABLED=0` cross-compilation is not used as a Wails release gate. GitHub Actions run `33603768527` passed native macOS arm64/x86_64 builds and universal unsigned packaging; the ZIP SHA-256 is `367dcd15cbc0237e92a514c66cd9e6e5b64dab4d317d075ca8186b75e0926c03`. Platform-neutral Darwin ARM64 package compilation also passes.

Stage 4 secret-injected GUI checkpoint (Windows GUI subsystem 2, 17,332,224 bytes; UTC build timestamp `2026-09-02T06:03:44.4838064Z`) SHA-256:

`D6A827158F0D4CC42CC336544CE625C211550D5AD3C19167EDE658E4249BB2BC`

Stage 3 checkpoint SHA-256: `8914D0C83F7AF6A88616773ACC2C9FA643F76FE46E2663B50D3A864970ACB019`

Stage 2 checkpoint SHA-256: `52585D12DCC4D948EFD295994FF48DE4FF99291AD6A76006777AE53531E09A87`

Stage 1 checkpoint SHA-256: `2D57547E850F4CA5F62C0FF3DAECB3D3F5CBCF74CD7D874C7CD189C6A9A0607E`

Stable rollback executable SHA-256 remains:

`C4A16DDC2B74DC1C3F88CB702590B20EE05F45ED505E28BDADB07BC27A78A1D4`

### Latest copied test-machine evidence

- `D:\api-automation.exe` matched the previous crashing-test build SHA-256 `58b4dadb4e4f0726dbda336f23d6caad1ff13fba4e3dda0d6c6af7c1a8c48571`; it does not contain the new page-range safety gate.
- The latest copied 5,000-page run passed API set/get and lifecycle checks with `EFOutProfile` sent using the exact leading U+FEFF, and the operator confirmed CWS now shows the profile as selected.
- The 19:34 schema-v3 live report proved the prior filter was active at 86 included/displayed / 179 excluded / zero removed / 25 constrained, but its exact UI audit exposed eight backend-only properties admitted by broad category fallback. The documented-taxonomy requirement now yields 78 / 187 / zero removed / 24 constrained; live UI confirmation remains required.
- The latest page-range failure planned `Custom(1-5)` but actually sent bare `EFPageRange="Range1"` (`custom=false`) because checked enum values and custom text shared one Single-Configuration axis. `23b50dc` fixes planning so non-empty custom text replaces enum values for that plan; live confirmation remains pending.
- Capability discovery advertises `EFPageRange` values `All`, `Odd`, `Even`, and `Range1`, with `Range1` proving range-capable metadata. It does not advertise `DPP_PAGE_RANGE` as writable.
- New CWS/Postman evidence for job `P00014754.6A9724A9.8301` shows `EFPageRange="5-10"`, `DPP_PAGE_RANGE=""`, `OrigPageCount=12`, and six generated/ripped pages. The implementation therefore sends normalized custom text directly as `EFPageRange`; it never maps custom text to `Range1` or emits the legacy companion.

## Wails 3 migration status

- Wails 3 is currently available as beta through `v3.0.0-beta.16`, not a GA release. It will be pinned exactly and introduced only after backend extraction.
- The platform-neutral core is integrated on `main`; Gio remains the production shell and validated fallback.
- `feature/wails3-ui` contains the full Windows-parity implementation and remains separate pending controlled live Fiery validation.
- Stages 1 through 5 are complete; Stage 6 implementation and automated/desktop gates pass, but its controlled live Fiery rows remain pending.
- Stage 7 native compile is covered on arm64 and x86_64 GitHub macOS runners, with universal packaging in `.github/workflows/wails-macos-preview.yml`. Follow `docs/wails3-macos-release.md`; signing/notarization and interactive Fiery/UI/wake-sleep/large-export gates still require Apple credentials, physical Macs, and the operator environment.
- Stage 8 preparation is complete: use `docs/wails3-field-adoption.md` for each controlled cycle and `cmd/compare-automation-results` for order-independent complete-record comparison. Promotion remains blocked until repeated accepted cycles and explicit release-owner approval; Gio retirement is a separate future decision.
- Full sequencing and verification requirements are in `docs/wails3-migration-plan.md`; reusable automated/live parity evidence is in `docs/wails3-regression-checklist.md`.

## Recommended live validation

Run a small live Fiery validation before a high-concurrency campaign and before declaring Gio parity after extraction:

1. Discover capabilities and confirm category/search/range rendering against the target server.
2. Save and reload one local preset, then reconnect to a different Fiery or discovery snapshot to confirm stale-value reconciliation.
3. Run one known-success TIFF/PDF in Process and Hold and confirm lifecycle PASS includes processed/raster evidence.
4. Run known-failure XPS and invalid-paper cases and confirm they FAIL with the Fiery processing error even if selected settings read back.
5. Run a disposable Hold-only custom range such as `5-10`; confirm `PAGE_RANGE_WIRE` reports `carrier=EFPageRange custom=true ... present=false`, API/CWS readback shows `EFPageRange="5-10"` with empty `DPP_PAGE_RANGE`, and a separately approved RIP produces six pages from a 12-page source.
6. Run one disposable Hold-only `EFOutProfile` API trace and confirm `ATTRIBUTE_WIRE` reports `leading=U+FEFF`, then verify the same profile is visibly selected in CWS. Return the API trace, normalized capability report, and diagnostic log if CWS still disagrees.
7. Run a constrained pair in a low Max-cases test and confirm local skipped-count reporting plus server constraint validation.
8. Confirm server presets populate on a target with known presets, apply one to a disposable imported job, and verify explicit selected capabilities override it afterward.
9. On an isolated test Fiery, inspect the Administration job count and validate restart/reboot recovery monitoring. Validate clear-all-jobs only with disposable jobs and explicit authorization; never test it against production jobs.
10. Inspect diagnostic JSON, API trace, JSONL, and Excel lifecycle columns for agreement.
11. Increase worker count only after the small run is stable, never above the enforced limit of 10.
