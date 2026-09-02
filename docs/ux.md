# UX Direction

API Automation should feel like a modern enterprise operations tool, not a toy request sender.

## Design principles

1. **Clear prerequisites without wizard chrome**: keep the connection gate, but use direct navigation rather than numbered steps or previous/next controls.
2. **Enterprise density**: use compact, structured controls suitable for long-running professional work.
3. **Progressive disclosure**: separate connection, assets, properties, planning, evidence, logs, and destructive operations.
4. **Non-blocking execution**: keep network calls and automation out of frontend event handlers.
5. **Operational clarity**: keep health, progress, verdicts, and errors visible without exposing credentials.
6. **Consistency over decoration**: prefer stable controls, predictable labels, and native confirmations over fragile custom painting.

### Wails visual system

The application uses a quiet desktop canvas, one compact navigation rail, restrained 12px cards, subtle borders, a single solid-blue primary action, standard white secondary buttons, and red only for destructive operations. Inputs use a consistent 40px height; ordinary actions use a compact 36px height and content width. File/folder Browse actions are explicitly fixed to field height and must never stretch to fill a row or card. Action groups wrap instead of enlarging controls, keyboard focus remains visible, disabled and hidden states remain unambiguous, and responsive layouts collapse content columns without changing workflow semantics. Decoration must not compete with server health, run progress, evidence, or safety confirmations.

## Implemented information architecture

The directly navigable workspace pages are Connection, Overview, Test Settings, Job Properties, Automation, and Results. Activity Logs and Administration are separate operational pages. The UI does not display a “Guided Workflow” heading, step counters, navigation numbers/arrows, or previous/next footer buttons.

### Connection

- The remaining workspace is locked until **Test Connection** succeeds for the exact current draft and the user presses **OK**.
- Passwords and secret/API keys are never shown after configuration. Existing values are represented only as **Configured**; blank replacement fields keep the configured value.
- **Change Server Connection** stages a replacement while the current connection remains active. The replacement becomes active only after a successful test and explicit **OK**.
- A server change invalidates server identity/capabilities, Fiery server presets, Job Property widgets, manual job IDs, inventory evidence, and cached health. Test files, automation choices, historical results/logs, and saved local presets survive.

### Overview

Overview is a compact readiness dashboard rather than another settings form:

- The header contains matching blue **Change Server Connection** and **Get/Refresh Capabilities** buttons only on Overview; the generic IP/operation-status pill is not rendered.
- One combined **Server Details** card uses larger label/value rows for server name, IP address, connected press, Idle/Busy state, version, serial number, time zone, locale, uptime, and the latest lightweight API check. Disk and memory are not displayed.
- Capability readiness, Automation Progress, and the separately scoped **Reset Job Properties**, **Reset Automation**, and **Reset Files** actions use a denser three-card row on wide windows.
- Current Configuration and Administration promotional cards and the Continue to Test Settings footer are intentionally absent; Administration remains available from the dedicated sidebar page.

One centralized monitor reuses a Fiery client/session and polls documented `/live/api/v5/status` every second only while Overview is visible. After capability discovery has completed, it also uses the captured job workload and a bounded 64-job tail probe every two seconds so jobs started from CWS or another client can drive **Busy** even when `/status` remains `running/none`. `OVERVIEW_STATUS_POLL` and `OVERVIEW_JOB_POLL` record the evidence; ignored pagination and request failures back off to 30 seconds instead of repeatedly downloading the full inventory. Active application automation remains an immediate Busy override.

### Test Settings

Test Settings contains only local test assets:

- test folder,
- optional specific file,
- All / Specific / Random file selection.

Lifecycle, concurrency, test intent, and generation do not appear here.

### Job Properties

- The connected press identity is extracted from explicit server property/queue metadata (for example `NZ-1000`).
- Fiery `/properties` is treated as a broad PPD schema. A displayed control must be mapped in the documented Antares/Capella/Vela CWS taxonomy and independently pass server metadata, meaningful-choice, visibility, installed-configuration, context, and alias checks. This necessary-plus-sufficient split removes backend-only properties that previously passed broad scope heuristics while ensuring taxonomy alone never implies support.
- Every refresh writes a schema-v3 filter audit that reconciles raw properties, normalized options, and the exact category/section/property list rendered by Job Properties. The diagnostic log emits `CAPABILITY_FILTER_AUDIT`, per-property `CAPABILITY_UI_DECISION`, and `CAPABILITY_VALUE_DECISION` entries with metadata and reasons.
- Eligible categories follow the Antares/Capella/Vela hierarchy: Job Info, Media, Layout, Color, Image, Finishing, and VDP.
- Category tabs use one compact fixed-height horizontal row, fixed widths, selection-count badges, and horizontal overflow on narrow windows.
- Search spans labels, API IDs, categories, defaults, and values.
- Properties remain vertically stacked under Fiery sections, with exact API IDs and every eligible advertised value preserved.
- Reusable local preset controls and the read-only Fiery server-preset selector each appear once above the categories, not inside every category.
- Numeric, Copies, Scale, and page-range validation retain server-aware safety bounds. Exact advertised `EFPageRange` values—including `Range1`—remain available when custom text is blank. A non-empty custom expression is the sole page-range value for that plan, preventing Single Configuration from silently sending bare `Range1`; it is normalized, checked against `OrigPageCount`, sent directly as `EFPageRange`, and verified from that same field. `DPP_PAGE_RANGE` is never sent or accepted as substitute readback.
- `EFOutProfile` labels hide the server's leading `U+FEFF`, while automation preserves that code point on the wire so CWS receives the exact advertised profile identity.

### Automation

Automation separates four independent decisions:

1. **Test intent**
   - Positive Validation.
   - Expected Constraint Rejection.
2. **Value source**
   - Server Baseline: no explicit property update and no Fiery server preset.
   - Advertised Defaults.
   - User-Selected Values.
   - All Advertised Values for included properties, with a bounded preferred suite when nothing is included.
3. **Case generation**
   - Single Configuration.
   - All Combinations (Cartesian).
   - Pairwise.
   - Bounded Random Sample.
4. **Lifecycle and concurrency**
   - Fiery lifecycle modes.
   - 1–10 parallel jobs.
   - Max cases remains independently bounded to 10,000.

“Permutation” is intentionally not used for Cartesian combinations.

Expected Constraint Rejection keeps only combinations where an explicitly selected dependency matches one of Fiery's published incompatible values (the constraint arrays are not allowlists). **Validation Only** is the recommended default: it imports a disposable held job, asks Fiery to validate without applying invalid settings, and deletes the job. An expected conflict is PASS; no conflict is FAIL; timeout, unavailable endpoint, HTTP 500, unrelated rejection, server failure, or cleanup failure is ERROR. **Controlled Apply** is an advanced isolated-server option and recognizes only explicit client-side constraint responses as expected rejection.

### Results, logs, and administration

- Results remain a dedicated evidence page with lifecycle and strict Set/Get verdicts, manual single-job cancel/delete, and Excel export.
- Activity Logs remain separate and show the complete diagnostic-file location.
- Administration remains separate. Restart, reboot, inventory, and clear-all-jobs retain operation blocking, typed/native confirmation, revalidation, recovery monitoring, and jobs-only clearing safeguards.

## Reset scopes

- **Reset Job Properties** clears selected values, numeric/page inputs, the selected Fiery preset, search, and active category.
- **Reset Automation** restores Positive Validation, User-Selected Values, Single Configuration, Validation Only, Hold, one worker, and 100 Max cases.
- **Reset Files** clears folder/file paths and restores All files.
- None of these clears the active connection, capabilities, historical results/logs, or saved presets.
