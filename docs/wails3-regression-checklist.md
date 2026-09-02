# Gio-to-Wails regression checklist

Updated: 2026-09-02

This is the parity gate for the validated Gio fallback and each Wails preview build. Never run destructive checks against a production Fiery. Record server/model, application commit, executable hash, tester, and evidence paths outside the repository.

## Automated core and desktop checks

- [x] Unit and characterization tests: `go test ./...`
- [x] Race tests: `go test -race ./...`
- [x] Static analysis: `go vet -all ./...` and `staticcheck ./...`
- [x] Dependency integrity: `go mod verify` and `go mod tidy -diff`
- [x] Called-vulnerability scan: `govulncheck ./...`
- [x] Darwin ARM64 compilation of platform-neutral internal packages
- [x] No `os/exec`, `exec.Command`, or `curl` execution reference in `internal/appgio` or `internal/fiery`
- [x] Secret-injected Windows GUI build, PE subsystem 2, and embedded-secret byte verification
- [x] Windows startup/close smoke test with no shell or curl children and graceful `WM_CLOSE`
- [x] Exact Wails runtime/CLI `v3.0.0-beta.16` pin and byte-reproducible name-based bindings
- [x] Frontend lockfile/audit, JavaScript syntax, no source maps, and no configured-secret bytes in assets
- [x] Distinct production-tag Wails preview build, visual startup inspection, and graceful no-shell-child close
- [x] Backend-owned file resolution, exact custom-range planning, preset reconciliation, run-mode rejection, typed event/state cloning, credential redaction, and administration interlock tests
- [x] Gio and Wails preview simultaneous side-by-side startup and graceful close

## Live Fiery parity checks

These checks require the operator's target server, credentials, approved files, and an isolated environment. They remain pending for the extracted-core checkpoint and must be run against both shells during Wails parity testing.

### Connection and discovery

- [ ] Test a valid connection, press OK, and confirm the workspace unlocks.
- [ ] Edit the staged server without applying it; confirm the active workspace remains on the prior server.
- [ ] Retest/apply a changed server; confirm capabilities, server presets, job IDs, health, and administration inventory are invalidated while files/results/logs remain.
- [ ] Discover capabilities; inspect categories, search, numeric fields, Copies, Scale, custom page range, constraints, and server presets.

### Presets and planning

- [ ] Save/reload a local preset and confirm no server address, credentials, headers, cookies, or file paths are persisted.
- [ ] Load it after changing the capability/server-preset inventory; confirm stale values are skipped and reported.
- [ ] Generate all strategies and value-source modes within the case limit; confirm counts and published-constraint skips.

### Automation, lifecycle, and results

- [ ] Run a small known-success PDF/TIFF through Hold and Process-and-Hold/RIP; confirm lifecycle and strict set/get evidence.
- [ ] Run known XPS/invalid-media failures; confirm prompt FAIL/ERROR classification rather than timeout.
- [ ] Cancel an active run; confirm one terminal event, bounded worker shutdown, cleanup evidence, and readable JSONL output.
- [ ] Validate one Expected Constraint Rejection case in Validation Only; use Controlled Apply only with explicit test approval.
- [ ] Confirm `EFOutProfile` sends its leading U+FEFF identity and displays/readbacks canonically.
- [ ] Confirm custom `EFPageRange=5-10` is sent directly, `DPP_PAGE_RANGE` is absent, and approved RIP evidence reports six pages for a 12-page source.
- [ ] Export Excel and compare counts/verdict/lifecycle/attribute evidence with the UI, logs, trace, and JSONL.

### External workload and administration

- [ ] Start a job externally; confirm Overview changes Busy then returns Idle, including transient poll backoff.
- [ ] Confirm restart/reboot interlocks and native confirmations; on an isolated Fiery, verify recovery monitoring after an approved operation.
- [ ] Inspect job inventory and confirm expiry/server changes invalidate it.
- [ ] With disposable jobs and explicit destructive-operation approval only, enter exact `CLEAR ALL JOBS`, verify count revalidation, and confirm empty inventory.

## macOS gate

- [x] Build the Wails shell natively on Apple Silicon and Intel macOS runners; do not treat a Windows cross-build as a release gate.
- [x] Compile the platform-neutral packages for Darwin ARM64 from Windows as an additional portability check.
- [x] Choose universal `arm64` + `x86_64` packaging with a distinct preview bundle identity and declared macOS 13 minimum.
- [ ] Validate Application Support paths, in-memory-only credential handling, Fiery TLS, file/folder permissions, cancellation, wake/sleep, and large exports on physical Macs.
- [ ] Sign with hardened runtime, notarize, install on a clean supported macOS account, and rerun parity smoke tests.

## Evidence status

- Extracted Gio core automated gate: **PASS**
- Extracted Gio desktop startup/close smoke: **PASS**
- Extracted Gio live Fiery checklist: **PENDING operator environment**
- Wails Windows-parity implementation/automated desktop gate: **PASS**
- Wails live connection/capability/mutation side-by-side checklist: **PENDING operator environment**
