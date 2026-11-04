const t = (key) => chrome.i18n.getMessage(key) || key;

for (const element of document.querySelectorAll("[data-i18n]")) {
  element.textContent = t(element.dataset.i18n);
}

const urlInput = document.getElementById("url");
const nameInput = document.getElementById("name");
const dirInput = document.getElementById("dir");
const sendButton = document.getElementById("send");
const status = document.getElementById("status");
const dot = document.getElementById("dot");
const server = document.getElementById("server");
const optionsLink = document.getElementById("options");

function guessName(raw) {
  try {
    const url = new URL(raw);
    const last = url.pathname.split("/").filter(Boolean).pop();
    if (!last) return "";
    return decodeURIComponent(last);
  } catch {
    return "";
  }
}

async function refreshStatus() {
  const answer = await chrome.runtime.sendMessage({ type: "status" });
  if (answer && answer.ok) {
    dot.classList.add("online");
    dot.title = t("statusOnline");
    server.textContent = `127.0.0.1:${answer.port}`;
  } else {
    dot.classList.remove("online");
    dot.title = t("errorServer");
    server.textContent = t("popupNoServer");
  }
  return answer;
}

async function prefill() {
  try {
    const [tab] = await chrome.tabs.query({ active: true, currentWindow: true });
    if (tab && tab.url && /^https?:/i.test(tab.url)) {
      urlInput.value = tab.url;
      nameInput.value = guessName(tab.url);
    }
  } catch {
    // The popup works without the active tab.
  }
}

urlInput.addEventListener("input", () => {
  if (!nameInput.value || nameInput.value === guessName(urlInput.dataset.last || "")) {
    nameInput.value = guessName(urlInput.value);
  }
  urlInput.dataset.last = urlInput.value;
});

urlInput.addEventListener("keydown", (event) => {
  if (event.key === "Enter") sendButton.click();
});

sendButton.addEventListener("click", async () => {
  const url = urlInput.value.trim();
  if (!url) {
    status.textContent = t("errorUrl");
    status.classList.add("error");
    urlInput.focus();
    return;
  }

  sendButton.disabled = true;
  status.classList.remove("error");
  status.textContent = t("popupSending");

  const answer = await chrome.runtime.sendMessage({
    type: "submit",
    url,
    filename: nameInput.value.trim(),
    dir: dirInput.value.trim(),
  });

  if (answer && answer.ok) {
    status.textContent = t("popupSent");
    setTimeout(() => window.close(), 700);
    return;
  }

  sendButton.disabled = false;
  status.classList.add("error");
  status.textContent = (answer && answer.error) || t("errorServer");
});

optionsLink.addEventListener("click", (event) => {
  event.preventDefault();
  chrome.runtime.openOptionsPage();
});

refreshStatus();
prefill();
