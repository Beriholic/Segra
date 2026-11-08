// config.js holds the settings the worker and the popup share: where Segra
// listens, the token it wants, and whether downloads come here at all.

export const DEFAULT_PORT = 47823;

// readBaked returns the port and token Segra wrote into the copy it
// exported, or null for a copy loaded from the repository.
export async function readBaked() {
  try {
    const response = await fetch(chrome.runtime.getURL("config.json"), { cache: "no-store" });
    if (!response.ok) return null;
    const data = await response.json();
    return data && typeof data === "object" ? data : null;
  } catch {
    return null;
  }
}

// readConfig is the settings the user saved, seeded on first use from the
// config.json of an exported copy, so that loading one needs no setup.
export async function readConfig() {
  const stored = await chrome.storage.local.get({ port: DEFAULT_PORT, token: "", intercept: true });
  const config = {
    port: Number(stored.port) || DEFAULT_PORT,
    token: String(stored.token || ""),
    intercept: stored.intercept !== false,
  };
  if (!config.token) {
    const baked = await readBaked();
    if (baked) {
      config.port = Number(baked.port) || config.port;
      config.token = String(baked.token || "");
      if (config.token) {
        await chrome.storage.local.set({ port: config.port, token: config.token });
      }
    }
  }
  return config;
}
