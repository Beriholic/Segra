// Segra's MV3 service worker: context-menu submission, port discovery and
// the API client shared with the popup and options pages.

const DEFAULT_PORT = 47823;
const PORT_SCAN = 10;

const t = (key, ...subs) => {
  const message = chrome.i18n.getMessage(key, subs);
  return message || key;
};

async function getConfig() {
  const stored = await chrome.storage.local.get({ port: DEFAULT_PORT, token: "" });
  return {
    port: Number(stored.port) || DEFAULT_PORT,
    token: String(stored.token || ""),
  };
}

async function probe(port) {
  try {
    const response = await fetch(`http://127.0.0.1:${port}/api/health`, {
      cache: "no-store",
      signal: AbortSignal.timeout(1500),
    });
    if (!response.ok) return null;
    const data = await response.json();
    return data && data.app === "segra" ? data : null;
  } catch {
    return null;
  }
}

// findServer tries the configured port first, then the default range, so
// that Segra moving to the next free port doesn't break the extension.
async function findServer(preferred) {
  const ports = [preferred];
  for (let i = 0; i < PORT_SCAN; i += 1) ports.push(DEFAULT_PORT + i);
  const seen = new Set();
  for (const port of ports) {
    if (!port || seen.has(port)) continue;
    seen.add(port);
    const info = await probe(port);
    if (info) return { port, info };
  }
  return null;
}

async function api(path, init = {}) {
  const { port, token } = await getConfig();
  const server = await findServer(port);
  if (!server) throw new Error("server");

  const headers = new Headers(init.headers || {});
  headers.set("Authorization", `Bearer ${token}`);
  if (init.body) headers.set("Content-Type", "application/json");

  let response;
  try {
    response = await fetch(`http://127.0.0.1:${server.port}${path}`, {
      ...init,
      headers,
      cache: "no-store",
      signal: AbortSignal.timeout(15000),
    });
  } catch {
    throw new Error("server");
  }

  const text = await response.text();
  let data = null;
  try {
    data = text ? JSON.parse(text) : null;
  } catch {
    data = null;
  }
  if (response.status === 401) throw new Error("token");
  if (!response.ok) throw new Error((data && data.error) || `HTTP ${response.status}`);
  return data;
}

async function submitTask({ url, referer, filename, dir }) {
  const body = { url };
  if (referer) body.referer = referer;
  if (filename) body.fileName = filename;
  if (dir) body.dir = dir;
  body.userAgent = navigator.userAgent;
  return api("/api/tasks", { method: "POST", body: JSON.stringify(body) });
}

function explain(error) {
  switch (String(error && error.message)) {
    case "server":
      return t("errorServer");
    case "token":
      return t("errorToken");
    default:
      return String(error && error.message ? error.message : error);
  }
}

async function notify(title, message) {
  try {
    await chrome.notifications.create({
      type: "basic",
      iconUrl: "icons/128.png",
      title,
      message,
    });
  } catch {
    // Notifications are optional.
  }
}

function guessName(rawURL) {
  try {
    const url = new URL(rawURL);
    const last = url.pathname.split("/").filter(Boolean).pop();
    return last ? decodeURIComponent(last) : "";
  } catch {
    return "";
  }
}

function hostOf(rawURL) {
  try {
    return new URL(rawURL).hostname;
  } catch {
    return rawURL;
  }
}

chrome.runtime.onInstalled.addListener(() => {
  chrome.contextMenus.removeAll(() => {
    chrome.contextMenus.create({
      id: "segra-link",
      title: t("contextLink"),
      contexts: ["link"],
    });
    chrome.contextMenus.create({
      id: "segra-media",
      title: t("contextMedia"),
      contexts: ["image", "video", "audio"],
    });
  });
});

chrome.contextMenus.onClicked.addListener(async (info, tab) => {
  const url = info.linkUrl || info.srcUrl;
  if (!url) return;
  try {
    const task = await submitTask({
      url,
      referer: info.pageUrl || (tab && tab.url),
      filename: guessName(url),
    });
    const name = (task && task.fileName) || guessName(url) || hostOf(url);
    await notify(t("notifSent"), name);
  } catch (error) {
    await notify(t("notifFailed"), explain(error));
  }
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (!message || typeof message.type !== "string") return false;

  if (message.type === "submit") {
    submitTask(message)
      .then((task) => sendResponse({ ok: true, task }))
      .catch((error) => sendResponse({ ok: false, error: explain(error) }));
    return true;
  }

  if (message.type === "status") {
    getConfig()
      .then(async ({ port, token }) => {
        const server = await findServer(port);
        sendResponse({
          ok: Boolean(server),
          port: server ? server.port : port,
          version: server ? server.info.version : "",
          token: Boolean(token),
        });
      })
      .catch(() => sendResponse({ ok: false }));
    return true;
  }

  if (message.type === "info") {
    api("/api/info")
      .then((info) => sendResponse({ ok: true, info }))
      .catch((error) => sendResponse({ ok: false, error: explain(error) }));
    return true;
  }

  return false;
});
