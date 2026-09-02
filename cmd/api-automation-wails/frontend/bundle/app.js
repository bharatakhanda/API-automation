import { Events } from "/wails/runtime.js";
import { Service } from "./bindings/api-automation/internal/appwails/index.js";

const pages = {
  connection: ["Server connection", "Test and explicitly apply a server before opening the workspace."],
  overview: ["Overview", "Live status and capability readiness from the shared Fiery backend."],
  settings: ["Test Settings", "Choose session-only source files through native dialogs."],
  properties: ["Job Properties", "Select exact advertised values and validated numeric ranges."],
  automation: ["Automation", "Preview backend-owned plans, execute typed run modes, and cancel safely."],
  results: ["Results", "Inspect typed lifecycle and strict set/get evidence, then export Excel."],
  logs: ["Activity Logs", "Review bounded frontend activity while complete results stay disk-backed."],
  administration: ["Administration", "Use guarded job and server operations with native confirmations."],
};

let state = { connection: {} };
let metadata;
let capabilityView;
let runState = { status: "Not started", progress: {} };
let currentPage = "connection";
let overviewRefreshPending = false;
const formState = { selectedValues: {}, numericInputs: {}, copiesInput: "1", customPageRange: "" };
const localLogs = [];

const byId = (id) => document.getElementById(id);
const notice = (message, error = false) => {
  const node = byId("notice");
  node.textContent = message;
  node.classList.toggle("error", error);
};
const errorText = (error) => error?.message || String(error || "Unknown backend error");
const appendLog = (message) => {
  if (!message) return;
  localLogs.push(`${new Date().toLocaleTimeString()}  ${message}`);
  if (localLogs.length > 1000) localLogs.splice(0, localLogs.length - 1000);
  renderLogs();
};
const draft = () => ({ ipAddress: byId("server-ip").value.trim(), secretKey: byId("secret-key").value.trim(), password: byId("password").value });

function setBusy(button, busy, label) {
  if (!button.dataset.label) button.dataset.label = button.textContent;
  button.disabled = busy;
  button.textContent = busy ? label : button.dataset.label;
}

function updateGates() {
  const connected = Boolean(state.connection?.hasActive);
  const capable = connected && Boolean(capabilityView?.optionCount);
  document.querySelectorAll(".nav.gated").forEach((button) => { button.disabled = !connected; });
  document.querySelectorAll(".nav.capability-gated").forEach((button) => { button.disabled = !capable; });
}

function updateConnection(connection) {
  state.connection = connection;
  const connected = Boolean(connection.hasActive);
  const badge = byId("connection-badge");
  badge.textContent = connected ? `Connected · ${connection.activeIpAddress}` : "Not connected";
  badge.className = `badge ${connected ? "good" : "neutral"}`;
  byId("apply-connection").disabled = !connection.testOk;
  byId("test-state").textContent = connection.testStatus || "Not tested";
  byId("cancel-change").hidden = !connection.changing;
  if (!byId("server-ip").value && connection.activeIpAddress) byId("server-ip").value = connection.activeIpAddress;
  updateGates();
}

async function showPage(name) {
  if (!pages[name]) return;
  if (name !== "connection" && !state.connection?.hasActive) return notice("Test and apply a server connection first.", true);
  if (["properties", "automation"].includes(name) && !capabilityView?.optionCount) return notice("Get capabilities from Overview first.", true);
  if (name === "connection" && state.connection?.hasActive && !state.connection.changing) {
    const result = await Service.StartConnectionChange();
    updateConnection(result.connection);
    notice(result.message);
  }
  document.querySelectorAll(".page").forEach((page) => page.classList.remove("active"));
  document.querySelectorAll(".nav[data-page]").forEach((button) => button.classList.toggle("active", button.dataset.page === name));
  byId(`page-${name}`).classList.add("active"); currentPage = name;
  byId("page-title").textContent = pages[name][0];
  byId("page-subtitle").textContent = pages[name][1];
  if (name === "overview") await refreshOverview();
  if (name === "results" || name === "logs") await refreshRunState();
  if (name === "administration") await refreshAdministration();
}

async function testConnection() {
  const button = byId("test-connection"); setBusy(button, true, "Testing…");
  byId("apply-connection").disabled = true; notice("Testing the staged connection through the Go backend…");
  try {
    const result = await Service.TestConnection(draft()); updateConnection(result.connection);
    notice(`${result.message}. Press Apply connection to unlock the preview.`); appendLog(`Connection test passed for ${draft().ipAddress}.`);
  } catch (error) {
    notice(`Connection test failed: ${errorText(error)}`, true); updateConnection((await Service.State()).connection);
  } finally { setBusy(button, false, ""); }
}

async function applyConnection() {
  const button = byId("apply-connection"); setBusy(button, true, "Applying…");
  try {
    const result = await Service.ApplyConnection(draft()); updateConnection(result.connection);
    if (result.changed) { capabilityView = undefined; resetCapabilityForm(); populateCapabilityGroups(); renderCapabilities(); populateServerPresets(); }
    byId("secret-key").value = ""; byId("password").value = ""; notice(result.message); appendLog(`Applied connection ${result.connection.activeIpAddress}.`);
    await showPage("overview");
  } catch (error) { notice(`Connection was not applied: ${errorText(error)}`, true); }
  finally { setBusy(button, false, ""); button.disabled = !state.connection.testOk; }
}

async function cancelConnectionChange() {
  try {
    const result = await Service.CancelConnectionChange(); updateConnection(result.connection);
    byId("server-ip").value = result.connection.activeIpAddress || ""; byId("secret-key").value = ""; byId("password").value = "";
    notice(result.message); if (result.connection.hasActive) await showPage("overview");
  } catch (error) { notice(errorText(error), true); }
}

async function refreshOverview() {
  if (overviewRefreshPending) return;
  overviewRefreshPending = true;
  const button = byId("refresh-overview"); setBusy(button, true, "Refreshing…");
  try {
    const overview = await Service.RefreshOverview();
    byId("overview-server").textContent = overview.serverName || overview.serverAddress;
    byId("overview-model").textContent = overview.pressModel || overview.serverAddress;
    byId("overview-status").textContent = overview.status; byId("overview-detail").textContent = overview.detail;
    byId("overview-options").textContent = String(overview.optionCount);
    byId("overview-checked").textContent = `Checked ${new Date(overview.checkedAt).toLocaleTimeString()} · ${overview.latencyMs} ms`;
    notice("Overview refreshed.");
  } catch (error) { notice(`Overview refresh failed: ${errorText(error)}`, true); }
  finally { overviewRefreshPending = false; setBusy(button, false, ""); }
}

async function discoverCapabilities(event) {
  const button = event?.currentTarget || byId("discover-capabilities"); setBusy(button, true, "Discovering…");
  notice("Reading and normalizing Fiery capabilities…");
  try {
    capabilityView = await Service.DiscoverCapabilities(); resetCapabilityForm(); populateCapabilityGroups(); renderCapabilities(); populateServerPresets(); updateGates(); await refreshPresetList();
    byId("overview-options").textContent = String(capabilityView.optionCount);
    notice(`Loaded ${capabilityView.optionCount} capabilities from ${capabilityView.serverName || "the active Fiery"}. Preflight: ${capabilityView.preflightStatus || "not available"}${capabilityView.captureWarnings?.length ? `; ${capabilityView.captureWarnings.length} evidence warning(s)` : ""}.`); appendLog(`Capability discovery: ${capabilityView.optionCount} applicable, ${capabilityView.excludedCount} excluded, preflight ${capabilityView.preflightStatus || "unknown"}.`); for (const path of capabilityView.capturePaths || []) appendLog(`Saved capability evidence: ${path}`);
  } catch (error) { notice(`Capability discovery failed: ${errorText(error)}`, true); }
  finally { setBusy(button, false, ""); }
}

function resetCapabilityForm() {
  formState.selectedValues = {}; formState.numericInputs = {}; formState.copiesInput = "1"; formState.customPageRange = "";
}
const displayValue = (value) => String(value ?? "").replace(/^\uFEFF/, "");

function filteredCapabilityOptions() {
  if (!capabilityView) return [];
  const query = byId("capability-search").value.trim().toLocaleLowerCase();
  const group = byId("capability-group").value;
  return capabilityView.options.filter((option) => (!group || option.group === group) && [option.id, option.label, option.group, option.value, ...(option.values || [])].join(" ").toLocaleLowerCase().includes(query));
}
function populateCapabilityGroups() {
  const select = byId("capability-group"), prior = select.value; select.replaceChildren(new Option("All categories", ""));
  for (const group of [...new Set((capabilityView?.options || []).map((option) => option.group))].sort()) select.add(new Option(group, group));
  if ([...select.options].some((item) => item.value === prior)) select.value = prior;
}
function renderCapabilities() {
  const list = byId("capability-list"); list.replaceChildren();
  if (!capabilityView) { const empty = document.createElement("div"); empty.className = "empty"; empty.textContent = "Get capabilities from Overview first."; return list.append(empty); }
  const options = filteredCapabilityOptions();
  byId("capability-summary").textContent = `${options.length} shown · ${capabilityView.optionCount} applicable · ${capabilityView.excludedCount} excluded`;
  for (const option of options) {
    const card = document.createElement("article"); card.className = "capability";
    const head = document.createElement("div"); head.className = "capability-head";
    const title = document.createElement("h3"); title.textContent = option.label; const id = document.createElement("code"); id.textContent = option.id; head.append(title, id); card.append(head);
    const group = document.createElement("span"); group.className = "group-pill"; group.textContent = `${option.group}${option.constraintCount ? ` · ${option.constraintCount} constraint value(s)` : ""}`; card.append(group);
    if (option.numeric) {
      const input = document.createElement("input"); input.className = "numeric-input"; input.placeholder = `Range ${option.min}–${option.max}; comma lists and inclusive ranges supported`;
      input.value = option.id === "num copies" ? formState.copiesInput : (formState.numericInputs[option.id] || "");
      input.addEventListener("input", () => { if (option.id === "num copies") formState.copiesInput = input.value; else formState.numericInputs[option.id] = input.value; }); card.append(input);
    } else {
      const values = document.createElement("div"); values.className = "capability-values";
      for (const value of option.values || []) {
        const label = document.createElement("label"); const box = document.createElement("input"); box.type = "checkbox"; box.checked = (formState.selectedValues[option.id] || []).includes(value);
        box.addEventListener("change", () => { const selected = new Set(formState.selectedValues[option.id] || []); box.checked ? selected.add(value) : selected.delete(value); formState.selectedValues[option.id] = [...selected]; });
        label.append(box, document.createTextNode(displayValue(value))); values.append(label);
      }
      card.append(values);
    }
    if (option.id === "EFPageRange") {
      const range = document.createElement("input"); range.className = "numeric-input"; range.placeholder = "Custom pages, e.g. 1,3,5-8 (replaces menu values)"; range.value = formState.customPageRange;
      range.addEventListener("input", () => { formState.customPageRange = range.value; }); card.append(range);
    }
    list.append(card);
  }
  if (!options.length) { const empty = document.createElement("div"); empty.className = "empty"; empty.textContent = "No capabilities match this search."; list.append(empty); }
}

function selectVisibleValues() {
  for (const option of filteredCapabilityOptions()) if (!option.numeric) formState.selectedValues[option.id] = [...(option.values || [])];
  renderCapabilities();
}
function resetProperties() { resetCapabilityForm(); renderCapabilities(); notice("Job Property selections and numeric inputs reset."); }

function populateServerPresets() {
  const select = byId("server-preset"), prior = select.value; select.replaceChildren(new Option("None", ""));
  for (const preset of capabilityView?.presets || []) select.add(new Option(`${preset.name} (${preset.id})`, preset.id));
  if ([...select.options].some((item) => item.value === prior)) select.value = prior;
}

function planningInput() {
  return { selectedValues: formState.selectedValues, numericInputs: formState.numericInputs, copiesInput: formState.copiesInput, customPageRange: formState.customPageRange, valueSource: byId("value-source").value, strategy: byId("strategy").value, testIntent: byId("test-intent").value, maxCases: Number(byId("max-cases").value) };
}
function fileSelection() { return { folderPath: byId("folder-path").value.trim(), filePath: byId("file-path").value.trim(), mode: byId("file-mode").value }; }
function selectedRunModeIDs() { return [...document.querySelectorAll(".run-mode:checked")].map((box) => box.value); }
function selectedRunModeLabels() { const selected = new Set(selectedRunModeIDs()); return metadata.runModes.filter((mode) => selected.has(mode.id)).map((mode) => mode.label); }

async function previewPlan() {
  const button = byId("preview-plan"); setBusy(button, true, "Generating…");
  try {
    const plan = await Service.PreviewPlan(planningInput()); renderPlan(plan); notice(`Generated ${plan.combinationCount} bounded combination(s).`);
  } catch (error) { notice(`Plan failed: ${errorText(error)}`, true); byId("plan-summary").textContent = errorText(error); }
  finally { setBusy(button, false, ""); }
}
function renderPlan(plan) {
  byId("plan-summary").textContent = `${plan.combinationCount} combination(s) · ${plan.axes.length} axis/axes · ${plan.constraintSkipped} positive conflict(s) skipped${plan.constraintWarning ? ` · ${plan.constraintWarning}` : ""}`;
  const list = byId("plan-list"); list.replaceChildren();
  for (const combination of plan.combinations) { const row = document.createElement("pre"); row.className = "plan-case"; row.textContent = Object.entries(combination).sort(([a],[b]) => a.localeCompare(b)).map(([key,value]) => `${key} = ${displayValue(value)}`).join("\n") || "Server baseline (no property update)"; list.append(row); }
  if (plan.truncated) { const row = document.createElement("div"); row.className = "empty"; row.textContent = "Preview limited to the first 100 cases; execution remains disk-backed."; list.append(row); }
}

async function chooseFolder() { try { const path = await Service.SelectTestFolder(); if (path) { byId("folder-path").value = path; appendLog(`Selected test folder: ${path}`); } } catch (error) { notice(errorText(error), true); } }
async function chooseFile() { try { const path = await Service.SelectTestFile(); if (path) { byId("file-path").value = path; byId("file-mode").value = "single"; appendLog(`Selected test file: ${path}`); } } catch (error) { notice(errorText(error), true); } }
async function validateFiles() { try { const result = await Service.ResolveTestFiles(fileSelection()); byId("file-summary").textContent = `${result.count} supported file(s) selected`; notice("Test file selection is valid."); } catch (error) { notice(`File selection invalid: ${errorText(error)}`, true); } }

async function refreshPresetList() {
  const select = byId("local-preset"), prior = select.value; select.replaceChildren(new Option("Select local preset", ""));
  try { for (const preset of await Service.ListPresets()) select.add(new Option(preset.name, preset.name)); if ([...select.options].some((item) => item.value === prior)) select.value = prior; }
  catch (error) { notice(`Preset list failed: ${errorText(error)}`, true); }
}
function presetInput() { return { name: byId("preset-name").value.trim(), selectedValues: formState.selectedValues, numericInputs: formState.numericInputs, copiesInput: formState.copiesInput, customPageRange: formState.customPageRange, strategy: byId("strategy").value, valueSource: byId("value-source").value, testIntent: byId("test-intent").value, constraintMode: byId("constraint-mode").value, maxCases: byId("max-cases").value, parallelJobs: byId("workers").value, runModeLabels: selectedRunModeLabels(), fileMode: byId("file-mode").value, serverPresetId: byId("server-preset").value }; }
async function savePreset() { try { await Service.SavePreset(presetInput()); await refreshPresetList(); byId("local-preset").value = byId("preset-name").value.trim(); notice("Local settings preset saved without credentials or paths."); } catch (error) { notice(`Preset save failed: ${errorText(error)}`, true); } }
async function loadPreset() {
  const name = byId("local-preset").value; if (!name) return notice("Select a local preset first.", true);
  try { const preset = await Service.LoadPreset(name); formState.selectedValues = preset.selectedValues || {}; formState.numericInputs = preset.numericInputs || {}; formState.copiesInput = preset.copiesInput || "1"; formState.customPageRange = preset.customPageRange || "";
    for (const [id, value] of [["strategy",preset.strategy],["value-source",preset.valueSource],["test-intent",preset.testIntent],["constraint-mode",preset.constraintMode],["max-cases",preset.maxCases],["workers",preset.parallelJobs],["file-mode",preset.fileMode],["server-preset",preset.serverPresetId]]) if (value) byId(id).value = value;
    const modes = new Set(preset.runModeLabels || []); document.querySelectorAll(".run-mode").forEach((box) => { const mode = metadata.runModes.find((item) => item.id === box.value); box.checked = modes.has(mode.label); }); renderCapabilities();
    byId("preset-name").value = preset.name; notice(`Preset loaded${preset.skippedCount ? `; ${preset.skippedCount} stale value(s) skipped` : ""}${preset.differentServer ? "; source server differs" : ""}.`);
  } catch (error) { notice(`Preset load failed: ${errorText(error)}`, true); }
}
async function deletePreset() { const name = byId("local-preset").value; if (!name) return notice("Select a local preset first.", true); try { await Service.DeletePreset(name); await refreshPresetList(); notice(`Deleted local preset ${name}.`); } catch (error) { notice(`Preset delete failed: ${errorText(error)}`, true); } }

async function startRun() {
  const input = { files: fileSelection(), plan: planningInput(), workers: Number(byId("workers").value), runModeIds: selectedRunModeIDs(), serverPresetId: byId("server-preset").value, constraintMode: byId("constraint-mode").value };
  try { runState = await Service.StartAutomation(input); renderRunState(); appendLog(`Automation ${runState.operationId} started.`); notice("Automation started."); await showPage("results"); }
  catch (error) { notice(`Automation could not start: ${errorText(error)}`, true); }
}
async function cancelRun() { runState = await Service.CancelAutomation(); renderRunState(); notice("Cancellation requested."); }
async function refreshRunState() { try { runState = await Service.AutomationState(); renderRunState(); } catch (error) { notice(errorText(error), true); } }

function renderRunState() {
  const progress = runState.progress || {}, planned = Number(progress.planned || 0), executed = Number(progress.executed || 0); const percent = planned ? Math.min(100, executed / planned * 100) : 0;
  byId("overview-run").textContent = runState.status || "Not started"; byId("overview-progress").textContent = `${executed} / ${planned} tests`;
  byId("progress-bar").style.width = `${percent}%`; byId("result-summary").textContent = `${runState.status || "Not started"} · ${executed}/${planned} · ${progress.passed || 0} PASS · ${progress.failed || 0} FAIL · ${progress.errors || 0} ERROR${runState.error ? ` · ${runState.error}` : ""}`;
  byId("cancel-run").disabled = runState.status !== "Running"; byId("export-results").disabled = !runState.resultFileReady;
  const body = byId("result-rows"); body.replaceChildren();
  for (const result of [...(runState.results || [])].reverse()) { const row = document.createElement("tr"); const values = [result.result, result.jobName || result.jobId, result.mode, [result.jobStatus,result.jobState].filter(Boolean).join(" / "), `${result.durationMs || 0} ms`, result.detail || result.lifecycle]; values.forEach((value,index) => { const cell = document.createElement("td"); cell.textContent = value || "—"; if (index === 0) cell.className = `result-${String(value).toLowerCase()}`; row.append(cell); }); body.append(row); }
  renderLogs();
}
function renderLogs() { const backend = runState.logs || []; const combined = [...localLogs, ...backend.map((line) => `RUN  ${line}`)]; byId("log-output").textContent = combined.length ? combined.join("\n") : "No activity yet."; }
async function exportResults() { const button = byId("export-results"); setBusy(button, true, "Exporting…"); try { const result = await Service.ExportResults(); notice(`Excel report saved: ${result.path}`); appendLog(`Exported ${result.total} results to ${result.path}.`); } catch (error) { notice(`Export failed: ${errorText(error)}`, true); } finally { setBusy(button, false, ""); button.disabled = !runState.resultFileReady; } }

function renderAdministration(view) { const inventory = view.inventory || {}; byId("inventory-summary").textContent = inventory.server ? `${inventory.count} job(s) on ${inventory.server} · ${new Date(inventory.inspected).toLocaleTimeString()}` : "Not inspected"; if (view.message) notice(view.message); }
async function refreshAdministration() { try { renderAdministration(await Service.AdministrationState()); } catch (error) { notice(errorText(error), true); } }
async function inspectJobs() { try { const view = await Service.InspectJobs(); renderAdministration(view); appendLog(view.message); } catch (error) { notice(`Job inspection failed: ${errorText(error)}`, true); } }
async function controlServer(action, button) { setBusy(button, true, action === "restart" ? "Restarting…" : "Rebooting…"); try { const view = await Service.ControlServer(action); renderAdministration(view); appendLog(view.message); } catch (error) { notice(`Server operation: ${errorText(error)}`, true); } finally { setBusy(button, false, ""); } }
async function clearJobs() { const button = byId("clear-jobs"); setBusy(button, true, "Clearing…"); try { const view = await Service.ClearAllJobs(byId("clear-confirmation").value); byId("clear-confirmation").value = ""; renderAdministration(view); appendLog(view.message); } catch (error) { byId("clear-confirmation").value = ""; notice(`Clear all jobs: ${errorText(error)}`, true); await refreshAdministration(); } finally { setBusy(button, false, ""); } }
async function manageJob(action, button) { setBusy(button, true, action === "cancel" ? "Cancelling…" : "Deleting…"); try { const result = await Service.ManageJob(byId("job-id").value, action); notice(result.message); appendLog(result.message); } catch (error) { notice(`Job action failed: ${errorText(error)}`, true); } finally { setBusy(button, false, ""); } }

function initialiseMetadata(value) {
  metadata = value; byId("max-cases").max = value.maximumMaxCases; byId("max-cases").value = value.defaultMaxCases; byId("workers").max = value.maximumWorkers; byId("workers").value = value.defaultWorkers;
  const container = byId("run-modes"); container.replaceChildren(); value.runModes.forEach((mode,index) => { const label = document.createElement("label"), box = document.createElement("input"); box.type = "checkbox"; box.className = "run-mode"; box.value = mode.id; box.checked = index === 0; label.append(box, document.createTextNode(mode.label)); container.append(label); });
}

for (const button of document.querySelectorAll(".nav[data-page]")) button.addEventListener("click", () => showPage(button.dataset.page).catch((error) => notice(errorText(error), true)));
for (const id of ["server-ip","secret-key","password"]) byId(id).addEventListener("input", () => { byId("apply-connection").disabled = true; byId("test-state").textContent = "Retest required"; });
byId("test-connection").addEventListener("click", testConnection); byId("apply-connection").addEventListener("click", applyConnection); byId("cancel-change").addEventListener("click", cancelConnectionChange);
byId("refresh-overview").addEventListener("click", refreshOverview); byId("discover-capabilities").addEventListener("click", discoverCapabilities); byId("refresh-capabilities").addEventListener("click", discoverCapabilities);
byId("browse-folder").addEventListener("click", chooseFolder); byId("browse-file").addEventListener("click", chooseFile); byId("validate-files").addEventListener("click", validateFiles);
byId("capability-search").addEventListener("input", renderCapabilities); byId("capability-group").addEventListener("change", renderCapabilities); byId("select-visible").addEventListener("click", selectVisibleValues); byId("reset-properties").addEventListener("click", resetProperties); byId("save-preset").addEventListener("click", savePreset); byId("load-preset").addEventListener("click", loadPreset); byId("delete-preset").addEventListener("click", deletePreset);
byId("preview-plan").addEventListener("click", previewPlan); byId("start-run").addEventListener("click", startRun); byId("cancel-run").addEventListener("click", cancelRun); byId("export-results").addEventListener("click", exportResults); byId("refresh-run-state").addEventListener("click", refreshRunState);
byId("inspect-jobs").addEventListener("click", inspectJobs); byId("restart-server").addEventListener("click", (event) => controlServer("restart", event.currentTarget)); byId("reboot-server").addEventListener("click", (event) => controlServer("reboot", event.currentTarget)); byId("clear-jobs").addEventListener("click", clearJobs); byId("cancel-job").addEventListener("click", (event) => manageJob("cancel", event.currentTarget)); byId("delete-job").addEventListener("click", (event) => manageJob("delete", event.currentTarget));

Events.On("automation:event", (incoming) => {
  const event = incoming?.data ?? incoming;
  if (event.operationId && runState.operationId && event.operationId !== runState.operationId) return;
  if (event.progress) runState.progress = event.progress;
  if (event.result?.result) { runState.results ||= []; runState.results.push(event.result.result); if (runState.results.length > 500) runState.results.shift(); }
  if (event.log?.message) { runState.logs ||= []; runState.logs.push(event.log.message); }
  if (event.terminal) { runState.status = event.terminal.status; runState.error = event.terminal.error; runState.storageError = event.terminal.storageError; setTimeout(refreshRunState, 100); }
  renderRunState();
});

try {
  const initial = await Promise.all([Service.State(), Service.Metadata(), Service.AutomationState()]);
  [state, metadata, runState] = initial;
  capabilityView = state.capabilities; initialiseMetadata(metadata); updateConnection(state.connection); byId("diagnostic-path").textContent = state.diagnosticPath ? `Diagnostic file: ${state.diagnosticPath}` : "Diagnostic file unavailable"; populateCapabilityGroups(); renderCapabilities(); populateServerPresets(); renderRunState();
  if (capabilityView) await refreshPresetList(); notice(`${state.version} is ready. Gio remains available as the production fallback.`);
  setInterval(() => { if (currentPage === "overview" && state.connection?.hasActive) refreshOverview(); }, Math.max(500, metadata.overviewIntervalMs));
} catch (error) { notice(`Preview backend failed to initialise: ${errorText(error)}`, true); }
