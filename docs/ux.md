# UX Direction

API Automation should feel like a modern enterprise operations tool, not a toy request sender.

## Design principles

1. **Workspace-first layout**: persistent navigation, clear page title, and one primary work area.
2. **Enterprise density**: compact controls with visible structure, suitable for long-running professional use.
3. **Progressive disclosure**: keep the first screen focused on request execution while leaving space for collections, environments, and history.
4. **Non-blocking execution**: all automation runs in the background; the UI must remain responsive.
5. **Operational clarity**: status, result grid, duration, and activity log are always visible.
6. **Consistency over decoration**: with native Win32/Windigo controls, prefer clean spacing, predictable labels, and stable behavior over fragile custom painting.

## Current shell

The initial shell uses:

- left-side workspace navigation,
- server connection inputs for IP address and secret key,
- test folder and file selection controls,
- request builder header area,
- primary Run/Cancel actions,
- full-row execution result grid,
- persistent activity log.

Future UI increments should add collections, environments, variables, assertions, and run history without breaking this layout model. Server credentials and test asset selection should remain prominent because they are prerequisites for execution.
