const t = (key, ...subs) => chrome.i18n.getMessage(key, subs) || key;

for (const element of document.querySelectorAll("[data-i18n]")) {
  element.textContent = t(element.dataset.i18n);
}

const portInput = document.getElementById("port");
const tokenInput = document.getElementById("token");
const saveButton = document.getElementById("save");
const testButton = document.getElementById("test");
const status = document.getElementById("status");
const infoCard = document.getElementById("info");

function setStatus(message, isError = false) {
  status.textContent = message;
  status.classList.toggle("error", isError);
}

// readBaked returns the port and token Segra wrote into the copy it
// exported, or null for a copy loaded from the repository.
async function readBaked() {
  try {
    const response = await fetch(chrome.runtime.getURL("config.json"), { cache: "no-store" });
    if (!response.ok) return null;
    const data = await response.json();
    return data && typeof data === "object" ? data : null;
  } catch {
    return null;
  }
}

async function load() {
  const stored = await chrome.storage.local.get({ port: 47823, token: "" });
  let { port, token } = stored;
  if (!token) {
    const baked = await readBaked();
    if (baked && baked.token) {
      port = Number(baked.port) || port;
      token = String(baked.token);
      await chrome.storage.local.set({ port, token });
    }
  }
  portInput.value = String(port);
  tokenInput.value = token;
}

async function save() {
  const port = Number(portInput.value);
  if (!Number.isInteger(port) || port < 1024 || port > 65535) {
    setStatus(t("errorPort"), true);
    return;
  }
  await chrome.storage.local.set({ port, token: tokenInput.value.trim() });
  setStatus(t("optionsSaved"));
  await test();
}

async function test() {
  testButton.disabled = true;
  saveButton.disabled = true;
  infoCard.hidden = true;

  const answer = await chrome.runtime.sendMessage({ type: "info" });
  testButton.disabled = false;
  saveButton.disabled = false;

  if (!answer || !answer.ok) {
    setStatus((answer && answer.error) || t("errorServer"), true);
    return;
  }

  const info = answer.info;
  document.getElementById("info-version").textContent = info.version || "";
  document.getElementById("info-dir").textContent = info.downloadDir || "";
  document.getElementById("info-connections").textContent = String(info.connections || "");
  infoCard.hidden = false;
  setStatus(t("optionsOk", info.version || ""));
}

saveButton.addEventListener("click", save);
testButton.addEventListener("click", test);
tokenInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") save();
});

load().then(test);
