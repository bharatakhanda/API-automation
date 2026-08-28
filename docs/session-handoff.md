# Session handoff

Updated: 2026-08-28

## Project

- Root: `C:\Users\harsh\Desktop\API_Automation`
- Repository: `https://github.com/bharatakhanda/API-automation`
- Branch: `main`
- Built EXE: `C:\Users\harsh\Desktop\API_Automation\bin\api-automation.exe`
- Do not commit/push `DATA/`.
- Do not push unless the user explicitly requests it.
- Secret key is injected into the EXE by linker flag from `.local/secrets.json`; it is not committed.

## Git state at handoff

- Local branch is ahead of `origin/main` by 17 commits.
- Latest local commit: `65dfbb5 feat: add one-click API trace capture`
- Important recent commits:
  - `65dfbb5 feat: add one-click API trace capture`
  - `a4c8fd1 fix: align Fiery ticket updates with Postman flow`
  - `d8f309a chore: log detailed job readback diagnostics`
  - `e0a85f8 fix: prefer v5 job readback for verification`
  - `e2a167e fix: wait longer for final attribute readback`
  - `ba04231 fix: require rip mode for rip-readback capabilities`
  - `7b3f6c3 feat: support multiple run modes`

`7924ca3` tried PUT-first attribute updates, but that behavior was superseded by `a4c8fd1`, which restored the EFI-reference flow: POST v4 first and POST v5 fallback.

## Current investigation

The server accepts `EFResolution` updates, but application verification previously did not receive `EFResolution` in the job GET response even after RIP. Postman can show `EFResolution` for a processed job.

Latest audit found and fixed:

1. Attributes had been set as soon as a job was visible, potentially before spooling completed. The app now waits for `status=done spooling` before setting attributes.
2. Job-ticket updates now follow the supplied EFI reference: `POST /live/api/v4/jobs/{id}`, with v5 POST fallback.
3. Login now retains all returned session cookies.
4. Job attribute parsing preserves direct `data.item` values so nested metadata cannot overwrite fields such as `EFResolution`.
5. Verification reads v5 first and logs detailed `READBACK` JSON.
6. On mismatch, exact raw base GET responses are logged as `POSTMAN_COMPARE` for v5 and v4.

Strict pass rule remains: set value must equal GET value.

## One-click diagnostic workflow

A new **Capture API trace** button is on the Capabilities page.

User steps:

1. Enter server details and test connection.
2. Select one supported test file/folder.
3. Get server capabilities.
4. Select one capability value, preferably `EFResolution=360x720dpi`.
5. Click **Capture API trace**.

The app automatically:

- logs in;
- imports one file to Hold;
- captures raw v5/v4 job GET responses;
- waits for done spooling;
- captures pre-update responses;
- sets selected attributes;
- captures immediate post-update responses;
- runs Process and Hold;
- waits for done ripping;
- captures post-RIP and final responses;
- performs strict verification.

Report path:

`<exe-folder>\captures\api-trace-YYYYMMDD-HHMMSS.json`

The report excludes password and secret key. It contains exact raw v5/v4 GET bodies at each stage.

## Next action after restart

Ask the user to run the latest EXE and use **Capture API trace** for one file and one `EFResolution` value. Then inspect the latest:

- `D:\logs`
- `D:\captures`
- especially `api-trace-*.json`

Compare `stages[].responses` for:

- `done spooling before update`
- `immediately after update`
- `done ripping`
- `final verification`

Determine whether the exact v5 raw body ever contains `EFResolution`, and whether the update persists only after spooling.

## Validation

At handoff, these passed:

- `go test ./...`
- `CGO_ENABLED=1 go test -race ./...`

The EXE was rebuilt after the latest changes.
