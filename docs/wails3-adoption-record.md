# Wails adoption and legacy-shell retirement record

Decision date: 2026-09-02

## Decision

The operator confirmed that the Wails application works correctly and explicitly requested removal of the legacy desktop shell from active source. Wails is therefore the sole desktop implementation and `cmd/api-automation` is its canonical entrypoint.

This decision approves source adoption. It does not waive destructive Fiery safeguards or the separate signed/notarized physical-Mac release gate.

## Preserved rollback checkpoints

- `gio-stable-20260902`: stable pre-extraction historical implementation.
- `wails-core-extracted-20260902`: validated shared-core checkpoint.
- `wails-preview-accepted-20260902`: accepted Wails checkpoint immediately before retirement cleanup.

These tags preserve source history without retaining two UI stacks in active source.

## Adoption changes

- Removed the legacy desktop entrypoint and adapter package.
- Removed Gio and Windigo module dependencies.
- Promoted the Wails command to `cmd/api-automation` and Windows output to `bin/api-automation.exe`.
- Retained `internal/application` as the platform-neutral authority and `internal/appwails` as the credential-safe desktop adapter.
- Changed macOS bundle/executable naming to the canonical API Automation identity.
- Retained the accepted `API Automation Wails Preview` application-data directory internally so existing presets and JSONL history are not discarded by promotion.
- Generalized result comparison to baseline/candidate stores:

```bash
go run ./cmd/compare-automation-results \
  -baseline '/path/to/baseline-results.jsonl' \
  -candidate '/path/to/candidate-results.jsonl'
```

## Ongoing release gates

- Use `docs/wails3-regression-checklist.md` for controlled live Fiery validation.
- Destructive rows require an isolated Fiery, explicit operator approval, and disposable jobs.
- Build macOS artifacts natively and complete signing, notarization, Gatekeeper, and physical-Mac checks before macOS distribution.
- Preserve logs, captures, complete JSONL stores, exports, build hashes, and operation IDs outside the repository for release evidence.
