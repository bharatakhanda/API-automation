# Wails 3 migration plan

Updated: 2026-09-02

## Decision

Migrate the desktop shell from Gio to Wails 3 without replacing or destabilizing the validated Gio application. The migration is an application-shell rewrite around shared Go services, not a rewrite of Fiery protocol, capability, planning, lifecycle, result, or safety semantics.

The validated rollback point is:

- Git commit: `23b50dc` (`fix: honor custom ranges and external Fiery workload`)
- Git tag: `gio-stable-20260902`
- Windows executable SHA-256: `C4A16DDC2B74DC1C3F88CB702590B20EE05F45ED505E28BDADB07BC27A78A1D4`
- Stable branch: `main`
- Core-extraction branch: `refactor/platform-neutral-core`
- Planned UI branch after extraction: `feature/wails3-ui`

The Gio executable remains the production fallback until Wails reaches verified Windows parity. Wails must initially build to a distinct executable and must not overwrite the Gio binary or mutate its configuration in an incompatible format.

## Framework status and toolchain

The Go module proxy currently reports Wails 3 through `v3.0.0-beta.16`; there is no Wails 3 general-availability tag. The local toolchain is Go 1.26.7, Node 26.7.0, and npm 11.19.0. Wails `v3.0.0-beta.16` declares Go 1.25, so the Go toolchain is sufficient.

Consequences:

1. Backend extraction does not depend on Wails and can proceed safely.
2. The Wails shell will pin an exact beta version rather than tracking latest.
3. Wails upgrades require a dedicated commit, changelog review, full parity suite, packaging checks, and a rollback test.
4. A Wails beta is never promoted over the Gio production fallback solely because it builds.
5. React with TypeScript is the initial frontend choice; dependencies beyond the Wails/Vite template must be justified by a concrete screen requirement and reviewed for license and maintenance risk.

## Non-negotiable behavior

The migration must preserve these rules in the shared Go backend:

- Exact connection draft testing and explicit application of the tested connection.
- No credential, cookie, secret key, local path, or embedded key in frontend state, events, logs, captures, presets, or result exports.
- Fiery v5 `apikey` login and deliberate v4 compatibility behavior.
- Capability normalization, taxonomy, exclusion diagnostics, exact wire values, and current NZ-1000 regression counts until an evidence-based filter change is approved.
- `EFOutProfile` leading U+FEFF on the wire.
- A non-empty custom page range is the only planned `EFPageRange` value; no `DPP_PAGE_RANGE` and no bare `Range1` substitution.
- Copies 1-9999, Parallel Jobs 1-10, Max Cases 1-10000, and bounded generation.
- Constraint arrays are incompatible values, not allowlists.
- Lifecycle verdicts remain independent from strict Set/Get verification.
- External and in-app Fiery workload evidence both drive Overview Busy.
- Cancellation, job cancel/delete, server restart/reboot, and clear-all-jobs remain distinct guarded operations.
- Clear-all-jobs retains fresh inventory, typed confirmation, native confirmation, immediate revalidation, jobs-only scope, and empty-inventory verification.
- Result JSONL/XLSX formats remain readable and secret-free; schema changes must be versioned and backward compatible.
- Shutdown cancels work, bounds waiting, closes resources, and does not leave shell or curl child processes.

## Current architecture inventory

### Already platform-neutral

These packages contain reusable backend/domain logic and should remain independent from either UI framework:

- `internal/fiery`: HTTP transport, authentication, discovery, jobs, attributes, constraints, presets, workload, and administration endpoints.
- `internal/capabilities`: parsing, eligibility, taxonomy, constraints, diagnostics, and normalization reports.
- `internal/combinations`: single, Cartesian, pairwise, and random generation.
- `internal/copyvalues`, `internal/pagevalues`, `internal/rangevalues`: validated value parsing.
- `internal/files`: supported-file selection.
- `internal/joboutcome`: lifecycle evidence and verdicts.
- `internal/preflight`: environment checks and snapshots.
- `internal/presets`: atomic versioned local preset persistence.
- `internal/reportxlsx`: disk-backed results and Excel export.
- `internal/model`: connection and file-selection data.

These packages compile without Gio/Windigo imports. They remain the authoritative implementation; equivalent logic must not be recreated in TypeScript.

### Logic currently coupled to Gio

The following behavior must be extracted before Wails binds to it:

- `automation_plan_windows.go`: widget-derived planning requests, value-source interpretation, constraint-intent filtering, and constraint execution classification.
- `window_windows.go`: run-mode definitions, run orchestration, worker scheduling, job execution, page-range validation, readback matching, lifecycle waits/actions, attribute serialization, API trace orchestration, result finalization, and several pure helpers.
- `workflow_windows.go`: connection approval state, external workload monitor orchestration, operation generations, and reset semantics.
- `administration_windows.go`: operation interlocks, inventory lease, recovery polling, and clear-jobs orchestration.
- `presets_windows.go`: conversion between UI state and safe preset DTOs.
- `diagnostic_windows.go`: executable-relative paths and diagnostic sink lifecycle.

Layout, Gio widgets, button handling, native dialogs, and visual state stay in `internal/appgio` and are not moved into the backend.

## Target boundaries

### `internal/application`

A platform-neutral application layer will own use cases and operation state. It may import existing domain packages but must not import Gio, Wails, Windigo, browser types, or OS-specific presentation code.

Planned responsibilities:

- Connection test/apply/invalidation state.
- Capability capture use case and safe summary DTOs.
- Planning request validation and immutable execution plans.
- Automation runner, worker bounds, cancellation, progress, and result events.
- Job action use cases.
- Workload monitoring and health snapshots.
- Preset validation/mapping.
- Guarded administration workflows.
- API trace use case.

### Ports and adapters

Use narrow interfaces at side-effect boundaries so services can be tested without a real Fiery or GUI:

- Fiery client/session factory.
- Clock and timers where expiry/backoff behavior matters.
- Diagnostic/event sink.
- Result store/export sink.
- Preset repository.
- File selection and native confirmation requests.

Do not wrap every pure package in an interface. Interfaces belong at I/O or time boundaries and should be defined by the consuming application service.

### DTO and event rules

- Frontends submit immutable request DTOs and receive immutable snapshot/result DTOs.
- Frontends never pass widget objects or mutate backend maps.
- Long-running operations return an operation ID and publish typed events.
- Events carry progress, safe status, counts, IDs needed for operation, and non-secret errors only.
- Event delivery must not block Fiery worker goroutines; bounded/coalesced progress is acceptable, but final results and terminal states cannot be dropped.
- The backend remains authoritative for validation and operation interlocks. Disabled buttons are not a security or safety boundary.

### Frontend adapters

- `internal/appgio` remains an adapter to the shared application layer until retirement.
- `internal/appwails` will expose a deliberately small binding facade and translate typed application events to Wails events.
- `frontend/` will contain React/TypeScript presentation and local ephemeral view state only.

## Branch and commit sequence

### Stage 0 — Stable baseline and migration contract

Branch: `refactor/platform-neutral-core`

Deliverables:

- Push `main` through `23b50dc`.
- Create and push `gio-stable-20260902`.
- Record migration architecture, invariants, parity gates, beta risk, and rollback point.
- Confirm existing quality/security suite and macOS compilation of platform-neutral packages.

Exit gate: clean branch, documentation committed and pushed, stable tag remotely available.

### Stage 1 — Extract pure planning and attribute semantics

Deliverables:

- Introduce UI-independent request/value DTOs.
- Move value-source planning, numeric/copies/page-range interpretation, constraint filtering, run-mode metadata, attribute serialization, output-profile comparison, and readback matching out of Gio.
- Adapt Gio to call the extracted functions with no visible behavior change.
- Move existing characterization tests to the platform-neutral package and retain Gio adapter tests.

Exit gate: all existing counts and wire semantics unchanged; Windows GUI audit and smoke test pass; Darwin compilation passes for the new package.

Status (2026-09-02): complete on `refactor/platform-neutral-core`. `internal/application` now owns immutable planning snapshots, value-source and range interpretation, bounded constraint-aware generation, Fiery wire serialization/readback semantics, run-mode metadata, and planning limits. Gio snapshots widgets into `application.PlanRequest` and delegates to the shared package. Platform-neutral characterization tests and retained Gio adapter tests pass, as do the full audit, Darwin ARM64 compilation, secret-safe GUI build, and GUI smoke test.

### Stage 2 — Extract execution runner and typed events

Deliverables:

- Move import/update/lifecycle/readback/result orchestration behind an application runner.
- Preserve worker bounds, cancellation, per-job recovery, constraint intent, server presets, lifecycle evidence, and JSONL result behavior.
- Introduce typed progress/result/terminal events and deterministic fake-client tests.
- Gio consumes runner snapshots/events rather than owning worker logic.

Exit gate: lifecycle and result parity tests pass; cancellation/race tests pass; GUI remains field-usable.

Status (2026-09-02): complete on `refactor/platform-neutral-core`. `application.Runner` now owns bounded worker scheduling, authenticated-client ports, import stabilization, server-preset application, constraint validation/disposable cleanup, attribute updates, lifecycle actions, strict readback, per-job panic recovery, cancellation, result recording, and terminal state. A discriminated typed event stream carries started/log/progress/result/readback/diagnostic/panic/terminal payloads through a non-blocking critical-event queue; result and terminal events are never coalesced or dropped. Gio binds `fiery.Client` once per session, consumes runner events, and keeps only presentation state. Deterministic fake-client tests cover success, RIP evidence, lifecycle failure, constraint cleanup, cancellation, recovery, storage failure, worker bounds, and result/event parity. The full audit, race tests, Darwin ARM64 compilation, secret-safe GUI build, and GUI smoke test pass.

### Stage 3 — Extract connection, monitoring, presets, and administration

Deliverables:

- Move connection approval/invalidation into backend state.
- Move workload polling policy, backoff, and generation guards.
- Move safe preset mapping/revalidation.
- Move administration interlocks, inventory lease, recovery, and clear verification while native confirmation remains a frontend request.

Exit gate: connection, external Busy, preset, and destructive-operation safeguards are testable headlessly and unchanged in Gio.

Status (2026-09-02): complete on `refactor/platform-neutral-core`. The application layer now owns exact-draft connection approval/invalidation and internal credential resolution; Overview polling intervals, bounded job-probe limits, exponential backoff, workload/automation Busy composition, and atomic generation guards; safe preset capture plus capability-aware canonical revalidation; and administration interlocks, inventory leases, exact typed confirmation, pre-clear count revalidation, empty-inventory verification, and recovery polling. Gio retains editors, native confirmations, HTTP adapter invocation, the thin polling loop, and rendering; backend state and safeguards remain authoritative. Headless tests cover connection replacement, external workload state, stale monitor generations, preset migration/reconciliation, operation interlocks, inventory expiry/server binding, clear verification, and recovery status. Full quality/race/security checks, Darwin ARM64 compilation, secret-safe GUI build, and GUI smoke testing pass.

### Stage 4 — Core extraction integration

Deliverables:

- Remove duplicate orchestration from `internal/appgio`.
- Produce the secret-injected Gio executable using the established safe build.
- Complete Windows live regression checklist where a live server is required.
- Merge `refactor/platform-neutral-core` into `main`, tag the extracted-core checkpoint, and push.

Exit gate: Gio is still the production application, but all Wails-required use cases are available through platform-neutral services.

Status (2026-09-02): complete. Temporary Gio connection and administration mirrors were removed, leaving `application.ConnectionState` and `application.AdministrationState` authoritative. All automated/race/static/security/Darwin compile gates pass, and the secret-injected Gio executable passes Windows startup, no-shell-child, and graceful-close smoke testing. The reusable live Fiery and future side-by-side checklist is `docs/wails3-regression-checklist.md`; Fiery-dependent rows remain explicitly pending until the target server, credentials, approved files, and isolated destructive-test environment are available. The integrated checkpoint is merged to `main` and tagged `wails-core-extracted-20260902` before any Wails dependency or code is added.

### Stage 5 — Wails 3 shell

Branch from updated `main`: `feature/wails3-ui`

Deliverables:

- Pin Wails 3 and frontend lockfiles.
- Add a distinct Wails command/build target and application data identity.
- Implement shutdown, navigation, connection, Overview, and read-only capabilities first.
- Do not overwrite `bin/api-automation.exe`; use a preview name.
- Add generated bindings deterministically and keep secrets out of frontend assets/source maps.

Exit gate: preview launches and exits cleanly on Windows; no mutation workflows are enabled yet.

Status (2026-09-02): complete on `feature/wails3-ui`. Both the Go module and installed generation CLI are pinned to `v3.0.0-beta.16`. `cmd/api-automation-wails` embeds a dependency-free, lockfile-controlled frontend under a distinct application/window identity and builds only as `bin/api-automation-wails-preview.exe`. `internal/appwails` exposes credential-safe connection snapshots, exact-draft test/apply, manual Overview refresh, and normalized read-only capability DTOs; no automation, job, preset mutation, file, result, or administration method is bound. Generated name-based bindings use the bundled Wails runtime and reproduce byte-for-byte. The production-tag Windows preview passed PE subsystem/embedded-secret/frontend-secret checks, visual startup inspection, no-shell-child smoke, and graceful `WM_CLOSE`; Gio remains unchanged and production-authoritative.

### Stage 6 — Wails Windows parity

Order:

1. Test Settings and file dialogs.
2. Job Properties, search, presets, numeric/custom-range controls, and constraint feedback.
3. Planning preview and automation controls.
4. Execution progress, cancellation, and live results.
5. Result inspection and Excel export.
6. Activity logs and diagnostics.
7. Manual job actions and guarded Administration.

Exit gate: automated parity matrix and controlled live tests match Gio; both executables install/run side by side.

### Stage 7 — macOS enablement

Deliverables:

- Build on macOS, not by assuming a Windows cross-build is releasable.
- Replace native platform adapters with Wails/macOS implementations.
- Use macOS Application Support/Logs locations and Keychain-compatible secret handling.
- Validate self-signed Fiery TLS behavior, file/folder permissions, cancellation, wake/sleep, and large result exports.
- Decide Apple Silicon-only versus universal binaries.
- Add signing, hardened runtime, notarization, and release verification.

Exit gate: signed/notarized field candidate passes the same backend parity suite and Mac-specific smoke tests.

### Stage 8 — Adoption and Gio retirement decision

- Release Wails as preview/beta beside Gio.
- Complete multiple field cycles and compare diagnostics/results.
- Keep rollback artifacts and configuration migration reversible.
- Retire Gio only after explicit acceptance; removal is a separate logical commit and release decision.

## Verification matrix

Every backend-extraction commit must run:

- `gofmt`
- `go test ./...`
- `go test -race ./...`
- `staticcheck ./...`
- `go vet -all ./...`
- `go mod verify`
- `go mod tidy -diff`
- `govulncheck ./...`
- Darwin compile of all platform-neutral packages
- Secret-safe Gio build, PE subsystem check, embedded-key presence check without printing it, no shell/curl children, and graceful `WM_CLOSE`

Additional parity fixtures must lock:

- NZ-1000 capability audit and disputed-ID diagnostics.
- Custom page range overriding selected `Range1`, direct `EFPageRange`, page-count validation, and strict readback.
- U+FEFF output-profile wire identity.
- Constraint conflict semantics and expected-rejection classification.
- Every lifecycle mode and terminal-failure classification.
- External CWS/client workload Busy and return to Idle.
- Worker/case/copies limits.
- Preset secret exclusion and stale reconciliation.
- Result JSONL/XLSX agreement.
- Administration confirmation/revalidation/recovery safeguards.

## Configuration and data compatibility

- Gio remains owner of the existing configuration until Wails compatibility is proven.
- Wails preview initially uses a separate application identifier and data directory.
- Import from Gio settings is copy-based and versioned; never destructively migrate the only copy.
- Preset and result readers should be backward compatible. Writers add schema versions before changing shape.
- Logs and captures identify frontend, application version, platform, and operation ID without credentials.

## Defect handling during migration

When a defect is found:

1. Add the smallest regression test that reproduces it.
2. Fix it in the lowest authoritative shared package, not independently in both frontends.
3. Commit the fix separately from unrelated migration work.
4. If the defect affects the stable Gio release, land/cherry-pick it to `main`, rebuild and validate Gio, then merge it back into migration branches.
5. Push the logical commit and report impact, verification, remaining work, and next stage.

Known evidence gap: the latest capability audit proves 78 displayed / 187 excluded / zero removed / 24 constrained and excludes the eight known backend leaks, but the operator has not yet identified the remaining controls that visually disagree with CWS. Do not guess or add model-specific blacklists; keep exact diagnostics and resolve IDs from field evidence.

## Reporting cadence

At the end of each stage:

- Commit logically and push the active branch.
- Report branch and commit.
- Summarize behavior moved or added.
- List verification completed and any live validation still pending.
- List defects found and their disposition.
- State the next stage and its first task.

No Wails preview becomes the production binary without an explicit parity gate, even though implementation commits and pushes are pre-authorized for this migration.
