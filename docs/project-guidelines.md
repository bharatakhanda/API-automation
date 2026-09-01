# Project Guidelines and Working Agreements

This document captures durable project rules and discussion points for quick retrieval during future development.

## Development workspace

- Local development folder: `C:\Users\harsh\Desktop\API_Automation`
- GitHub repository: `https://github.com/bharatakhanda/API-automation`
- Primary branch: `main`

## Technology stack

- Language: Go
- Desktop UI: Gio (`gioui.org`), with Windigo used only for native Windows dialogs.
- Target platform: Windows desktop application.
- GUI threading rule: run Gio through `app.Main`; protect background state with synchronization and request redraws with `Window.Invalidate`.

## Engineering expectations

Think and implement like:

- a senior Go developer,
- an API automation architect,
- and a senior UX engineer.

The application must be:

- performant,
- stable,
- responsive,
- maintainable,
- testable,
- and suitable for enterprise usage.

## Backend and automation architecture rules

- Keep business logic separated from the UI.
- Use clean package boundaries under `internal/`.
- Use `context.Context` for cancellation, timeouts, and request lifecycle control.
- Use concurrency where it improves responsiveness or throughput.
- Never block the UI thread with network calls or long-running automation.
- Use controlled concurrency, not unbounded goroutine spawning.
- Prefer worker-pool style execution for workflows and batch runs; accept 1 through 1000 parallel jobs while retaining explicit cancellation and bounded case generation.
- Reuse HTTP clients and transports for connection pooling.
- Add tests for core execution and runner behavior.
- Treat errors as first-class output; expose them clearly in execution results and logs.
- Excel exports must include a non-secret run summary and a results sheet with fixed Job ID, Job Name, Result, mode, final job status/state/error, and lifecycle-verification columns followed by dynamic paired Set/Get attribute columns.
- Never export passwords, API keys, session cookies, or other authentication material.

## UI and UX rules

- Design the UI as a modern enterprise desktop application.
- Favor a workspace-oriented layout with clear navigation and primary actions.
- Keep status visible and make results and activity logs easy to access from separate workspace pages during execution.
- Show a visible, draggable vertical scrollbar whenever workspace content exceeds the available height.
- Organize discovered features into focused category tabs (Job info, Layout, Substrate, Color and Image, Finishing, VDP, Installable options, and Other/Advanced) and display every server-advertised value within its capability; do not impose an arbitrary UI-only value cap.
- Provide feature search across labels, API keys, categories, defaults, and advertised values.
- Provide Select all at both the category and individual capability heading levels. Reset must clear checkbox and numeric selections and restore automation controls while preserving connection details, discovery data, file paths, and saved presets.
- Render Fiery `efirange` properties from their server-provided min/max/increment/precision metadata. Numeric inputs may contain one value, comma-separated values, or inclusive ranges and must be bounded before combination generation.
- Keep optional Scale input unset by default and send it only when the user enters a validated value; the imported job and Fiery server remain authoritative for support.
- Save local reusable setting presets atomically under the user's configuration directory. Presets may contain feature selections, numeric inputs, strategy, worker/case limits, file-selection mode, and run modes, but never credentials, cookies, or file paths. Validate restored values against current discovery.
- Treat Copies as a validated numeric capability input from 1 through 9999: comma-separated entries become generation-axis values, one entry applies to every generated job, and inclusive ranges are expanded and randomized within the independently configured Max cases limit. Copies input must never modify Max cases.
- Treat Fiery `EFPageRange=Range1` as a custom text value rather than a checkbox. Accept Copies-style page syntax (`1,3,5-8` and `5 to 8`) as one page-selection expression, send `EFPageRange=Range1` with normalized `DPP_PAGE_RANGE`, and validate the highest requested page against the imported job's authoritative original page count after spooling. Never clamp an invalid range silently.
- Keep run cancellation, Fiery job cancellation, and permanent job deletion distinct. Manual job cancel may proceed only for processing/ripping, waiting-to-print, or printing states; manual delete may target any state and must require explicit confirmation.
- Cancel-while-Processing/Ripping, Cancel-while-Waiting-to-Print, Cancel-while-Printing, and Delete automation modes must use separate imported jobs and condition-based state waits. Delete must remove only its dedicated test job.
- Process and Hold/RIP results must not pass solely because Set equals Get. After processing, require successful Fiery status/state and raster/page evidence, and fail on error, PDL error, canceled/aborted processing, unsupported PDL, or missing expected raster evidence.
- Apply discovered constraints in two stages: filter combinations only when selected values explicitly contradict published compatibility metadata, then ask the imported job's Fiery constraint endpoint before updating constrained settings when supported. Cache endpoint unavailability and keep the attribute-update response authoritative on older servers.
- GUI shutdown must cancel the root application context, stop accepting background work, wait only for a bounded interval, and avoid long blocking result-file synchronization.
- Preserve UI responsiveness during automation runs.
- Keep widget mutation on the Gio event goroutine; synchronize shared background state and call `Window.Invalidate` after updates.
- Prefer clean spacing, predictable labels, and operational clarity over decorative complexity.
- Current UX direction is documented in `docs/ux.md`.

## Git and repository rules

- Create logical commits for each meaningful change.
- Do not push unless explicitly instructed by the user.
- Remote: `origin` -> `https://github.com/bharatakhanda/API-automation.git`
- The repository has already been created under the `bharatakhanda` GitHub account.

## Current product direction

The product is an API automation tool, not only a simple REST client. Planned capabilities should align with:

- request execution,
- collections/workflows,
- environments and variables,
- assertions/validation,
- concurrent workflow runs,
- execution history,
- logs and diagnostics,
- and stable desktop UX for professional use.

## Server execution requirements

The application connects to a server before automation can run.

Required user-provided connection inputs:

- server IP address,
- secret key,
- server/admin password required by the Fiery login API.

Required test asset inputs:

- a local folder containing test files,
- a file selection strategy.

Supported file selection strategies:

- **All files**: every regular file in the selected folder is eligible for execution.
- **Single file**: the user selects one specific file, and it must be inside the selected folder.
- **Random file**: the application chooses one eligible file from the selected folder.

Rules:

- Automation must not start without a server IP address, secret key, and admin password.
- Automation must not start without a valid test folder.
- Random selection is owned by the application, not the user.
- File selection logic must remain testable outside the UI.

## Fiery API reference behavior

The temporary `DATA/` folder is reference-only and must never be committed. It contains old JavaScript/Python automation that informed the Go implementation.

Useful behavior incorporated into the Go application:

- Fiery server base URL: `https://{server}`.
- Prefer Fiery API v5 for newly implemented server calls while retaining deliberate v4 compatibility fallbacks where current servers require them.
- Login endpoint: `POST /live/api/v5/login`.
- Login payload uses `username`, `password`, and the user-provided secret/API key. Fiery v5 requires the JSON field `apikey`; only the v4 compatibility fallback uses the legacy field `accessrights`. A v5 request using `accessrights` may return HTTP 200 and a cookie but creates a restricted session that exposes only compact job data.
- Do not hardcode credentials from the temporary `DATA/` folder into source code.
- The server returns an authenticated session cookie via `Set-Cookie`.
- Test files are imported as jobs with multipart upload to `POST /live/api/v5/jobs`.
- Import form fields:
  - `file`: selected test file.
  - `queue`: selected run-mode queue; current lifecycle modes import into `hold` and then perform explicit actions.
- Capability capture includes `GET /live/api/v5/info` and saves snapshots next to the EXE under `captures/server-capabilities-snapshot-YYYYMMDD-HHMMSS.json`.
- Snapshot files must not include the secret key, password, or session cookie.
- Fiery installations may use self-signed certificates, so the server client supports controlled insecure TLS for this environment.
- Supported run modes:
  - Hold: import to hold queue only.
  - Process and Hold: import to hold, then `rip`.
  - RIP: import to hold, then `rip`.
  - Press Print: import to hold, then RIP, move to production, and `press_print`.
  - Ready to Print: import to hold, then RIP and move to production.
  - Print: import to hold, then RIP, move to production, `press_print`, and `print`.
- Job operation endpoints use the proven v4 operation first with v5 compatibility fallback:
  - `PUT /live/api/v{4|5}/jobs/{jobId}/rip`
  - `PUT /live/api/v{4|5}/jobs/{jobId}/press_print`
  - `PUT /live/api/v{4|5}/jobs/{jobId}/print`
  - `POST /live/api/v{4|5}/jobs/{jobId}` for attribute updates.
