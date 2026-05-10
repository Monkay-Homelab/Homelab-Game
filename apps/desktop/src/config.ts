declare global {
  interface Window {
    __APP_CONFIG__?: {
      API_URL?: string;
    };
  }
}

const DEFAULT_API_URL = 'https://api.homelab.living';

export const API_URL = window.__APP_CONFIG__?.API_URL || DEFAULT_API_URL;

export const WS_URL = API_URL.replace(/^https:\/\//, 'wss://').replace(/^http:\/\//, 'ws://');
