# Session handoff

Updated: 2026-09-01

## Project

- Root: `C:\Users\harsh\Desktop\API_Automation`
- Repository: `https://github.com/bharatakhanda/API-automation`
- Branch: `main`
- Built EXE: `C:\Users\harsh\Desktop\API_Automation\bin\api-automation.exe`
- Do not commit or push `DATA/`, `.local/`, captures, logs, result files, executables, or secrets.
- Do not push unless the user explicitly requests it.
- The Fiery secret key is injected into the EXE by linker flag from `.local/secrets.json`; it is not committed or printed.

## Git state at handoff

- Local `main` is synchronized with `origin/main` after the user-requested logical commit and push; generated executables, captures, logs, results, `.local/`, and secrets remain excluded.
- Latest implementation work: direct connection-gated navigation, compact Server Details, one-second documented `/status` polling with active-automation Busy override, staged connection replacement, documented-taxonomy-plus-metadata capability eligibility with schema-v3 UI audits, correct Fiery constraints, exact-wire `EFOutProfile`, direct validated `EFPageRange`, a 10-worker ceiling, presets, and guarded administration.
- Implementation commits:
  - `1d4044a feat: add categorized constraint-aware capability metadata`
  - `c6f2a3e feat: add secure local settings preset store`
  - `373c96b feat: validate job settings with Fiery constraints`
  - `2be430a feat: evaluate Fiery lifecycle outcomes`
  - `8469edb feat: add feature tabs search ranges presets and lifecycle results`
  - `e304744 docs: document categorized constraint-aware automation`
  - `ab656bd feat: tighten Fiery capability filtering and diagnostics`
  - `4b0c1fd feat: improve Fiery workspace automation and live status`
- The complete local history is pushed to `origin/main` as requested.

## Current implementation

### Workspace UX

- Connection, Overview, Test Settings, Job Properties, Automation, and Results use direct sidebar navigation; Activity Logs and Administration are separate. Guided Workflow text, navigation arrows/numbers, step counters, and previous/next footers are removed.
- No non-Connection page is accessible until the exact draft passes Test Connection and the user presses OK.
- Existing passwords and secret/API keys are never repopulated into editors. The UI shows only Configured and accepts an optional replacement.
- Server replacement is staged; the active connection remains authoritative until the tested draft is explicitly applied. A true server/credential change invalidates capabilities, server presets, Job Property widgets, job IDs, inventory, and cached health while preserving test files, automation choices, saved local presets, results, and logs.
- Overview removes prior generic/promotional/footer and disk/memory elements. Its two header buttons match. One session-reusing monitor polls documented `/live/api/v5/status` every second while visible and emits `OVERVIEW_STATUS_POLL`; active application automation displays Busy even if API health is stale `running/none`, while failures retain adaptive backoff.
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
- Every advertised `EFPageRange` value, including `Range1`, remains an exact independent checkbox option. When `EFPageRange` advertises range capability, custom input is normalized, checked against the imported file's original page count, sent directly as `EFPageRange`, and strictly read back from that field. `DPP_PAGE_RANGE` is never emitted or accepted as substitute verification.
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

The following passed after the workspace, constraint-intent, and press-applicability filtering implementation:

- `gofmt -w internal`
- `go test ./...`
- `go test -race ./...`
- `staticcheck ./...`
- `go vet -all ./...`
- `go mod verify`
- `go mod tidy -diff`
- `govulncheck ./...` — no called vulnerabilities
- Secret-injected Windows GUI build with `-trimpath -s -w -H=windowsgui`
- PE subsystem verification: Windows GUI (`Subsystem 2`)
- Embedded-secret byte verification without printing the key
- GUI startup smoke test, no `cmd`/`curl`/PowerShell child process, graceful `WM_CLOSE`, exit code 0

Built executable (Windows GUI subsystem 2, 17,195,520 bytes) SHA-256:

`795dac6af2e2fcf64c90b32d7a5059377e0a72fef627dd22805adc2bf7e3f5dc`

### Latest copied test-machine evidence

- `D:\api-automation.exe` matched the previous crashing-test build SHA-256 `58b4dadb4e4f0726dbda336f23d6caad1ff13fba4e3dda0d6c6af7c1a8c48571`; it does not contain the new page-range safety gate.
- The latest copied 5,000-page run passed API set/get and lifecycle checks with `EFOutProfile` sent using the exact leading U+FEFF, and the operator confirmed CWS now shows the profile as selected.
- The 19:34 schema-v3 live report proved the prior filter was active at 86 included/displayed / 179 excluded / zero removed / 25 constrained, but its exact UI audit exposed eight backend-only properties admitted by broad category fallback. The documented-taxonomy requirement now yields 78 / 187 / zero removed / 24 constrained; live UI confirmation remains required.
- The subsequent page-range run planned `Custom(1-5)` but sent five attributes, including unadvertised `DPP_PAGE_RANGE="1-5"` plus `EFPageRange="Range1"`. The update returned success, then the log stopped immediately after RIP began; its JSONL result file is empty, consistent with the reported server crash.
- Capability discovery advertises `EFPageRange` values `All`, `Odd`, `Even`, and `Range1`, with `Range1` proving range-capable metadata. It does not advertise `DPP_PAGE_RANGE` as writable.
- New CWS/Postman evidence for job `P00014754.6A9724A9.8301` shows `EFPageRange="5-10"`, `DPP_PAGE_RANGE=""`, `OrigPageCount=12`, and six generated/ripped pages. The implementation therefore sends normalized custom text directly as `EFPageRange`; it never maps custom text to `Range1` or emits the legacy companion.

## Recommended next action

Run a small live Fiery validation before a high-concurrency campaign:

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
