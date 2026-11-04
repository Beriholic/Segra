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

async function load() {
  const stored = await chrome.storage.local.get({ port: 47823, token: "" });
  portInput.value = String(stored.port);
  tokenInput.value = stored.token;
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
