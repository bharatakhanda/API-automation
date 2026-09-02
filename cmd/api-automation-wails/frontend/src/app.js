import { Service } from "./bindings/api-automation/internal/appwails/index.js";

const pages = {
  connection: ["Server connection", "Test and explicitly apply a server before opening the preview workspace."],
  overview: ["Overview", "Live read-only status from the shared Fiery backend."],
  capabilities: ["Capabilities", "Inspect normalized Job Properties without enabling mutation workflows."],
};

let state;
let capabilityView;

const byId = (id) => document.getElementById(id);
const notice = (message, error = false) => {
  const node = byId("notice");
  node.textContent = message;
  node.classList.toggle("error", error);
};
const errorText = (error) => error?.message || String(error || "Unknown backend error");
const draft = () => ({
  ipAddress: byId("server-ip").value.trim(),
  secretKey: byId("secret-key").value.trim(),
  password: byId("password").value,
});

function setBusy(button, busy, label) {
  if (!button.dataset.label) button.dataset.label = button.textContent;
  button.disabled = busy;
  button.textContent = busy ? label : button.dataset.label;
}

function updateConnection(connection) {
  state.connection = connection;
  const connected = Boolean(connection.hasActive);
  const badge = byId("connection-badge");
  badge.textContent = connected ? `Connected · ${connection.activeIPAddress}` : "Not connected";
  badge.className = `badge ${connected ? "good" : "neutral"}`;
  document.querySelectorAll(".nav.gated").forEach((button) => { button.disabled = !connected; });
  byId("apply-connection").disabled = !connection.testOK;
  byId("test-state").textContent = connection.testStatus || "Not tested";
  byId("cancel-change").hidden = !connection.changing;
  if (!byId("server-ip").value && connection.activeIPAddress) byId("server-ip").value = connection.activeIPAddress;
}

async function showPage(name) {
  if (!pages[name]) return;
  if (name !== "connection" && !state?.connection?.hasActive) {
    notice("Test and apply a server connection first.", true);
    return;
  }
  if (name === "connection" && state?.connection?.hasActive && !state.connection.changing) {
    const result = await Service.StartConnectionChange();
    updateConnection(result.connection);
    notice(result.message);
  }
  document.querySelectorAll(".page").forEach((page) => page.classList.remove("active"));
  document.querySelectorAll(".nav[data-page]").forEach((button) => button.classList.toggle("active", button.dataset.page === name));
  byId(`page-${name}`).classList.add("active");
  byId("page-title").textContent = pages[name][0];
  byId("page-subtitle").textContent = pages[name][1];
  if (name === "overview") await refreshOverview();
}

async function testConnection() {
  const button = byId("test-connection");
  setBusy(button, true, "Testing…");
  byId("apply-connection").disabled = true;
  notice("Testing the staged connection through the Go backend…");
  try {
    const result = await Service.TestConnection(draft());
    updateConnection(result.connection);
    notice(`${result.message}. Press Apply connection to unlock the preview.`);
  } catch (error) {
    notice(`Connection test failed: ${errorText(error)}`, true);
    const fresh = await Service.State();
    updateConnection(fresh.connection);
  } finally {
    setBusy(button, false, "");
  }
}

async function applyConnection() {
  const button = byId("apply-connection");
  setBusy(button, true, "Applying…");
  try {
    const result = await Service.ApplyConnection(draft());
    updateConnection(result.connection);
    byId("secret-key").value = "";
    byId("password").value = "";
    notice(result.message);
    await showPage("overview");
  } catch (error) {
    notice(`Connection was not applied: ${errorText(error)}`, true);
  } finally {
    setBusy(button, false, "");
    button.disabled = !state.connection.testOK;
  }
}

async function cancelConnectionChange() {
  try {
    const result = await Service.CancelConnectionChange();
    updateConnection(result.connection);
    byId("server-ip").value = result.connection.activeIPAddress || "";
    byId("secret-key").value = "";
    byId("password").value = "";
    notice(result.message);
    if (result.connection.hasActive) await showPage("overview");
  } catch (error) {
    notice(errorText(error), true);
  }
}

async function refreshOverview() {
  const button = byId("refresh-overview");
  setBusy(button, true, "Refreshing…");
  try {
    const overview = await Service.RefreshOverview();
    byId("overview-server").textContent = overview.serverName || overview.serverAddress;
    byId("overview-model").textContent = overview.pressModel || overview.serverAddress;
    byId("overview-status").textContent = overview.status;
    byId("overview-detail").textContent = overview.detail;
    byId("overview-options").textContent = String(overview.optionCount);
    byId("overview-checked").textContent = `Checked ${new Date(overview.checkedAt).toLocaleTimeString()} · ${overview.latencyMs} ms`;
    notice("Overview refreshed.");
  } catch (error) {
    notice(`Overview refresh failed: ${errorText(error)}`, true);
  } finally {
    setBusy(button, false, "");
  }
}

function renderCapabilities() {
  const query = byId("capability-search").value.trim().toLocaleLowerCase();
  const list = byId("capability-list");
  list.replaceChildren();
  if (!capabilityView) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "Discover capabilities to inspect the normalized model.";
    list.append(empty);
    return;
  }
  const options = capabilityView.options.filter((option) => [option.id, option.label, option.group, option.value, ...(option.values || [])].join(" ").toLocaleLowerCase().includes(query));
  byId("capability-summary").textContent = `${options.length} shown · ${capabilityView.optionCount} applicable · ${capabilityView.excludedCount} excluded`;
  for (const option of options) {
    const card = document.createElement("article");
    card.className = "capability";
    const head = document.createElement("div");
    head.className = "capability-head";
    const title = document.createElement("h3");
    title.textContent = option.label;
    const id = document.createElement("code");
    id.textContent = option.id;
    head.append(title, id);
    const detail = document.createElement("p");
    if (option.numeric) detail.textContent = `Numeric range ${option.min}–${option.max}, increment ${option.increment}`;
    else detail.textContent = `${option.values?.length || 0} advertised value(s) · current: ${option.value || "—"}`;
    const group = document.createElement("span");
    group.className = "group-pill";
    group.textContent = option.group;
    card.append(head, detail, group);
    list.append(card);
  }
  if (!options.length) {
    const empty = document.createElement("div");
    empty.className = "empty";
    empty.textContent = "No capabilities match this search.";
    list.append(empty);
  }
}

async function discoverCapabilities() {
  const button = byId("discover-capabilities");
  setBusy(button, true, "Discovering…");
  notice("Reading and normalizing Fiery capabilities…");
  try {
    capabilityView = await Service.DiscoverCapabilities();
    renderCapabilities();
    byId("overview-options").textContent = String(capabilityView.optionCount);
    notice(`Loaded ${capabilityView.optionCount} read-only capabilities from ${capabilityView.serverName || "the active Fiery"}.`);
  } catch (error) {
    notice(`Capability discovery failed: ${errorText(error)}`, true);
  } finally {
    setBusy(button, false, "");
  }
}

document.querySelectorAll(".nav[data-page]").forEach((button) => button.addEventListener("click", () => showPage(button.dataset.page).catch((error) => notice(errorText(error), true))));
for (const id of ["server-ip", "secret-key", "password"]) byId(id).addEventListener("input", () => { byId("apply-connection").disabled = true; byId("test-state").textContent = "Retest required"; });
byId("test-connection").addEventListener("click", testConnection);
byId("apply-connection").addEventListener("click", applyConnection);
byId("cancel-change").addEventListener("click", cancelConnectionChange);
byId("refresh-overview").addEventListener("click", refreshOverview);
byId("discover-capabilities").addEventListener("click", discoverCapabilities);
byId("capability-search").addEventListener("input", renderCapabilities);

try {
  state = await Service.State();
  capabilityView = state.capabilities;
  updateConnection(state.connection);
  renderCapabilities();
  notice(`${state.version} is ready. Mutation workflows remain disabled.`);
} catch (error) {
  state = { connection: {} };
  notice(`Preview backend failed to initialise: ${errorText(error)}`, true);
}
