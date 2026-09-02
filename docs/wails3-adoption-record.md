# Wails preview field-adoption record

Use one copy of this record for every controlled Gio/Wails field cycle. Do not promote Wails or retire Gio from a single successful run. Destructive rows require an isolated Fiery and explicit operator-approved disposable jobs.

## Cycle metadata

- Date/time and operator:
- Workstation OS/build and CPU architecture:
- Gio commit/version/executable SHA-256:
- Wails commit/version/executable or app ZIP SHA-256:
- Wails frontend/backend identity shown in diagnostics:
- Fiery address (redacted if the record leaves the test environment):
- Fiery model, serial, software version, API version, and queue inventory:
- Approved source files and expected page counts:
- Capability/environment/normalization evidence paths:
- Gio JSONL/XLSX/diagnostic paths:
- Wails JSONL/XLSX/diagnostic paths:
- Operation IDs:

Never paste passwords, configured secrets, session cookies, Authorization headers, or secret-bearing build metadata into this record.

## Preconditions

- [ ] Gio stable fallback and its matching rollback hash are available.
- [ ] Wails is installed under the distinct preview name and does not replace Gio files, shortcuts, configuration, or results.
- [ ] Both applications can run side by side.
- [ ] The target, credentials, approved files, queues, preset names, and allowable administration actions are written into the test authorization.
- [ ] Job inventory is empty or every existing job is identified; destructive tests use an isolated target.
- [ ] The same capability snapshot, file set, run modes, values, Copies range, page range, strategy, workers, and case limit will be used in both shells.

## Required observations

Complete the matching rows in `docs/wails3-regression-checklist.md` and attach evidence for:

1. Connection failure/success, reconnect invalidation, and no credential leakage.
2. Capability counts, exact option/value identities, exclusion reasons, preflight, and saved normalization evidence.
3. Local and server preset behavior, including stale-value reconciliation.
4. Plan counts, deterministic combinations, selected run modes, and constraint skips/conflicts.
5. Direct `EFPageRange` behavior with no `DPP_PAGE_RANGE` and leading U+FEFF `EFOutProfile` wire identity.
6. Approved lifecycle execution, strict set/get evidence, terminal classification, cancellation, bounded workers, and cleanup.
7. External-workload Busy-to-Idle transitions and bounded monitoring behavior.
8. UI/result/log/JSONL/XLSX agreement and a large export.
9. Native confirmations, inventory leases, generation invalidation, recovery monitoring, and—only when approved—destructive administration.
10. On macOS: Finder/Gatekeeper launch, dialogs and permissions, Application Support paths, wake/sleep recovery, signing, notarization, and staple validation.

## Result comparison

Compare complete disk-backed records rather than the frontend's bounded recent rows:

```bash
go run ./cmd/compare-automation-results \
  -gio '/path/to/gio/automation-run-results.jsonl' \
  -wails '/path/to/wails/automation-run-results.jsonl' \
  > field-cycle-comparison.json
```

Exit code `0` means semantic multisets match. Generated job IDs, durations, and completion order are intentionally ignored. Lifecycle, verdict, status/state/error, last event, detail, job name, and exact set/get maps must match. Exit code `1` means a semantic difference and includes bounded representative records; exit code `2` means input or decoding failure.

If an expected platform-specific difference exists, explain it and obtain explicit acceptance rather than editing the evidence. Fix real backend differences in shared Go code and rerun both shells.

## Cycle decision

- [ ] Accepted with no unexplained semantic or safety difference.
- [ ] Rejected; defect links and rollback action recorded.
- [ ] Inconclusive; missing evidence and required rerun recorded.

Reviewer/operator signatures:

- Automation owner:
- Fiery environment owner:
- Release owner:

## Promotion and rollback rules

- Keep `bin/api-automation.exe`, `gio-stable-20260902`, and `wails-core-extracted-20260902` available until explicit retirement approval.
- Preview installation must not overwrite Gio's executable or data. Removal of the preview means deleting only its app/executable and the separately approved `API Automation Wails Preview` data directory.
- Do not migrate Gio settings destructively. Any future importer must copy, version, validate, and leave the original untouched.
- A Wails defect immediately returns operators to Gio; preserve Wails diagnostics/results before uninstalling when safe.
- Wails becomes the default only after repeated accepted Windows and macOS cycles, signed/notarized packaging, release-owner approval, and a documented support window.
- Gio retirement/removal requires a separate explicit decision, commit, release, and rollback plan. It is never implied by completing this template.
