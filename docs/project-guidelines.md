# Project Guidelines and Working Agreements

This document captures durable project rules and discussion points for quick retrieval during future development.

## Development workspace

- Local development folder: `C:\Users\harsh\Desktop\API_Automation`
- GitHub repository: `https://github.com/bharatakhanda/API-automation`
- Primary branch: `main`

## Technology stack

- Language: Go
- Desktop UI: Windigo (`github.com/rodrigocfd/windigo`)
- Target platform: Windows desktop application
- GUI threading rule: lock the main goroutine to the OS thread with `runtime.LockOSThread()`.

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
- Prefer worker-pool style execution for workflows and batch runs.
- Reuse HTTP clients and transports for connection pooling.
- Add tests for core execution and runner behavior.
- Treat errors as first-class output; expose them clearly in execution results and logs.

## UI and UX rules

- Design the UI as a modern enterprise desktop application.
- Favor a workspace-oriented layout with clear navigation and primary actions.
- Keep status, results, and activity logs visible during execution.
- Preserve UI responsiveness during automation runs.
- Marshal background updates onto the Windigo UI thread with `wnd.UiThread`.
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
- Use only Fiery API v5 for newly implemented server calls.
- Login endpoint: `POST /live/api/v5/login`.
- Login payload uses `username`, `password`, and `accessrights` where `accessrights` maps to the user-provided secret key.
- Do not hardcode credentials from the temporary `DATA/` folder into source code.
- The server returns an authenticated session cookie via `Set-Cookie`.
- Test files are imported as jobs with multipart upload to `POST /live/api/v5/jobs`.
- Import form fields:
  - `file`: selected test file.
  - `queue`: `hold`.
- Keep-alive/status check uses `GET /live/api/v5/status`.
- Capability capture saves snapshots next to the EXE under `captures/server-capabilities-snapshot-YYYYMMDD-HHMMSS.json`.
- Snapshot files must not include the secret key, password, or session cookie.
- Fiery installations may use self-signed certificates, so the server client supports controlled insecure TLS for this environment.
- Job operation endpoints available for future workflow steps should use v5 paths, for example:
  - `PUT /live/api/v5/jobs/{jobId}/rip`
  - `PUT /live/api/v5/jobs/{jobId}/press_print`
  - `PUT /live/api/v5/jobs/{jobId}/print`
  - `DELETE /live/api/v5/jobs/{jobId}`
  - `POST /live/api/v5/jobs/{jobId}` for attribute updates.
