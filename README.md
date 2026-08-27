# API Automation

Windows desktop API automation tool built with Go and Windigo.

## Current foundation

- Concurrent request execution engine with tuned HTTP transport.
- Workflow runner with configurable worker concurrency and cancellation.
- Windigo desktop shell for ad-hoc API execution.
- UI-safe background execution using `wnd.UiThread`.

## Commands

```powershell
go test ./...
go build -trimpath -ldflags "-s -w -H=windowsgui" -o bin/api-automation.exe ./cmd/api-automation
```

## Project layout

```text
cmd/api-automation        Application entrypoint
internal/app              Windigo desktop UI
internal/engine           HTTP execution engine
internal/model            Workflow/request/result models
internal/runner           Concurrent workflow runner
```
