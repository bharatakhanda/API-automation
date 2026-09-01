# UX Direction

API Automation should feel like a modern enterprise operations tool, not a toy request sender.

## Design principles

1. **Workspace-first layout**: persistent navigation, clear page title, and one primary work area.
2. **Enterprise density**: compact controls with visible structure, suitable for long-running professional use.
3. **Progressive disclosure**: keep the first screen focused on request execution while leaving space for collections, environments, and history.
4. **Non-blocking execution**: all automation runs in the background; the UI must remain responsive.
5. **Operational clarity**: status, result grid, duration, and activity logs are clearly accessible in dedicated workspace pages.
6. **Consistency over decoration**: with Gio controls and native Windows dialogs, prefer clean spacing, predictable labels, and stable behavior over fragile custom painting.

## Current shell

The current shell uses:

- left-side workspace navigation,
- server connection inputs for IP address and secret key,
- test folder and file selection controls,
- request builder header area,
- primary Run/Cancel actions and a header Reset that restores automation defaults without clearing server details, discovery data, or file paths,
- dedicated full-row execution results page,
- separate activity-logs page with the complete diagnostic-file location,
- a visible, draggable vertical scrollbar whenever page content exceeds the viewport,
- category tabs for Job info, Layout, Substrate, Color and Image, Finishing, VDP, Installable options, and Other/Advanced so only one focused feature area is normally rendered,
- a search bar that finds features by label, Fiery API key, category, current/default value, or advertised value across all tabs,
- every server-advertised capability value shown under its capability heading with no arbitrary display cap,
- Select all controls on both category headings and individual capability headings; numeric/range fields are not changed by these checkboxes,
- validated numeric inputs for server-advertised `efirange` properties and optional Scale, accepting one value, comma-separated values, and inclusive ranges,
- a Copies text field accepting comma-separated values and inclusive `5-10` / `5 to 10` ranges from 1 through 9999 while respecting the independently configured Max cases limit,
- local reusable presets for selections, numeric values, generation settings, and run modes; credentials and file paths are excluded,
- separate confirmed Cancel job and Delete job controls on Results; cancel accepts processing/ripping, waiting-to-print, and printing states while delete accepts any state,
- dedicated Cancel-while-Processing/Ripping, Cancel-while-Waiting-to-Print, Cancel-while-Printing, and Delete run modes that each create a separate job and use condition-based state waits,
- bounded coordinated shutdown that cancels background operations and does not hold the process open on a slow server request or long-run result finalization,
- lifecycle-aware PASS/FAIL evaluation that shows final Fiery status/state/error evidence for processed jobs rather than relying only on Set/Get equality,
- Excel export from Results with a Summary sheet, lifecycle/status evidence, and dynamic per-attribute Set/Get columns.

Future UI increments should add collections, environments, variables, assertions, and run history without breaking this layout model. Server credentials and test asset selection should remain prominent because they are prerequisites for execution.
