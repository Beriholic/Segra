// Segra's MV3 service worker: context-menu submission, port discovery and
// the API client shared with the popup and options pages.

import { DEFAULT_PORT, readConfig } from "./config.js";

const PORT_SCAN = 10;

const t = (key, ...subs) => {
  const message = chrome.i18n.getMessage(key, subs);
  return message || key;
};

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
  const { port, token } = await readConfig();
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

async function submitTask({ url, referer, cookie }) {
  const body = { url };
  if (referer) body.referer = referer;
  if (cookie) body.cookie = cookie;
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

// cookiesFor is the header the browser would send with url, so that a file
// behind a login survives the trip through the app. Chrome answers with the
// cookies of the sites the extension may read, and nothing for the rest.
async function cookiesFor(url) {
  try {
    const cookies = await chrome.cookies.getAll({ url });
    return cookies.map((c) => `${c.name}=${c.value}`).join("; ");
  } catch {
    return "";
  }
}

// dropDownload takes a download out of the browser, list and file alike.
async function dropDownload(id) {
  try {
    await chrome.downloads.cancel(id);
  } catch {
    // It finished before we got here: it stays, beside Segra's copy.
  }
  try {
    await chrome.downloads.erase({ id });
  } catch {
    // Erasing is tidying, not the point.
  }
}

// interceptDownload hands a download the browser started to Segra and takes
// the browser's copy away. Segra is asked first: a download the app refuses
// stays where it is, and the browser is never left holding nothing.
async function interceptDownload(item) {
  // The ones this extension started, as the download handed back below.
  if (item.byExtensionId === chrome.runtime.id) return;

  const url = item.finalUrl || item.url || "";
  if (!/^https?:/i.test(url)) return;

  const { intercept } = await chrome.storage.local.get({ intercept: true });
  if (!intercept) return;

  const { port } = await readConfig();
  // Segra is not running: the browser keeps what it started, as it would
  // with the extension switched off.
  if (!(await findServer(port))) return;

  // No file name: the browser only knows the one it guessed from the URL
  // before the response arrived, which for a redirect is a signed path or
  // an id. Segra reads the real one from Content-Disposition.
  try {
    await submitTask({
      url,
      referer: item.referrer || "",
      cookie: await cookiesFor(url),
    });
  } catch (error) {
    // Segra said no, so the browser takes the download back rather than
    // lose it: this one is started by the extension, and skipped above.
    chrome.downloads.download({ url }).catch(() => {});
    await notify(t("notifFailed"), explain(error));
    return;
  }

  await dropDownload(item.id);
  // No notification on the way in: the app already shows the download, and
  // a browser that downloads in bulk would ring once per file.
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
    });
    const name = (task && task.fileName) || guessName(url) || hostOf(url);
    await notify(t("notifSent"), name);
  } catch (error) {
    await notify(t("notifFailed"), explain(error));
  }
});

chrome.downloads.onCreated.addListener((item) => {
  interceptDownload(item).catch(() => {});
});

chrome.runtime.onMessage.addListener((message, _sender, sendResponse) => {
  if (!message || typeof message.type !== "string") return false;

  if (message.type === "status") {
    readConfig()
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
