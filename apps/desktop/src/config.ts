declare global {
  interface Window {
    __APP_CONFIG__?: {
      API_URL?: string;
      API_URL_MAP?: string;
    };
  }
}

const DEFAULT_API_URL = 'https://api.homelab.living';

// Parse a "host1=url1,host2=url2,*=fallback" string into a Map.
function parseUrlMap(raw: string): Map<string, string> {
  const out = new Map<string, string>();
  for (const pair of raw.split(',')) {
    const eq = pair.indexOf('=');
    if (eq === -1) continue;
    const host = pair.slice(0, eq).trim();
    const url = pair.slice(eq + 1).trim();
    if (host && url) out.set(host, url);
  }
  return out;
}

function resolveApiUrl(): string {
  const cfg = window.__APP_CONFIG__;
  if (!cfg) return DEFAULT_API_URL;

  if (cfg.API_URL_MAP) {
    const entries = parseUrlMap(cfg.API_URL_MAP);
    const matched = entries.get(window.location.hostname) ?? entries.get('*');
    if (matched) return matched;
  }

  return cfg.API_URL || DEFAULT_API_URL;
}

export const API_URL = resolveApiUrl();

export const WS_URL = API_URL.replace(/^https:\/\//, 'wss://').replace(/^http:\/\//, 'ws://');
