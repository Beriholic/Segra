import { readConfig } from "./config.js";

const t = (key, ...subs) => chrome.i18n.getMessage(key, subs) || key;

for (const element of document.querySelectorAll("[data-i18n]")) {
  element.textContent = t(element.dataset.i18n);
}

const portInput = document.getElementById("port");
const tokenInput = document.getElementById("token");
const interceptInput = document.getElementById("intercept");
const saveButton = document.getElementById("save");
const testButton = document.getElementById("test");
const status = document.getElementById("status");
const dot = document.getElementById("dot");
const server = document.getElementById("server");

function setStatus(message, isError = false) {
  status.textContent = message;
  status.classList.toggle("error", isError);
}

// check asks the worker where Segra is, and says so in the footer and the
// dot. It returns what the worker answered, or null.
async function check() {
  const answer = await chrome.runtime.sendMessage({ type: "status" });
  if (answer && answer.ok) {
    dot.classList.add("online");
    dot.title = t("statusOnline");
    server.textContent = answer.version
      ? `127.0.0.1:${answer.port} · v${answer.version}`
      : `127.0.0.1:${answer.port}`;
    return answer;
  }
  dot.classList.remove("online");
  dot.title = t("errorServer");
  server.textContent = t("popupNoServer");
  return null;
}

async function load() {
  const config = await readConfig();
  portInput.value = String(config.port);
  tokenInput.value = config.token;
  interceptInput.checked = config.intercept;
  await check();
}

async function save() {
  const port = Number(portInput.value);
  if (!Number.isInteger(port) || port < 1024 || port > 65535) {
    setStatus(t("errorPort"), true);
    return;
  }
  await chrome.storage.local.set({ port, token: tokenInput.value.trim() });
  setStatus(t("popupSaved"));
  await check();
}

async function test() {
  testButton.disabled = true;
  const answer = await check();
  testButton.disabled = false;
  if (answer) {
    setStatus(t("popupConnected", answer.version || ""));
  } else {
    setStatus(t("errorServer"), true);
  }
}

saveButton.addEventListener("click", save);
testButton.addEventListener("click", test);
for (const input of [portInput, tokenInput]) {
  input.addEventListener("keydown", (event) => {
    if (event.key === "Enter") save();
  });
}

interceptInput.addEventListener("change", async () => {
  await chrome.storage.local.set({ intercept: interceptInput.checked });
  setStatus(t("popupSaved"));
});

load();
