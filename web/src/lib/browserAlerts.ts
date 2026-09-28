import { api, type AlertPrefs } from "../api/client";

const STORAGE_KEY = "gitseer-browser-alerts";

let enabledCache = false;

export function setBrowserAlertsEnabledCache(on: boolean): void {
  enabledCache = on;
  try {
    if (on) localStorage.setItem(STORAGE_KEY, "1");
    else localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
}

export type BrowserAlertEvent = {
  title?: string;
  severity?: string;
  repo_full?: string;
  deep_link?: string;
  id?: number;
};

function urlBase64ToUint8Array(base64String: string): Uint8Array {
  const padding = "=".repeat((4 - (base64String.length % 4)) % 4);
  const base64 = (base64String + padding).replace(/-/g, "+").replace(/_/g, "/");
  const raw = atob(base64);
  const out = new Uint8Array(raw.length);
  for (let i = 0; i < raw.length; i++) out[i] = raw.charCodeAt(i);
  return out;
}

export function notificationPermission(): NotificationPermission | "unsupported" {
  if (typeof window === "undefined" || !("Notification" in window)) return "unsupported";
  return Notification.permission;
}

export function pushSupported(): boolean {
  return (
    typeof window !== "undefined" &&
    "serviceWorker" in navigator &&
    "PushManager" in window &&
    "Notification" in window
  );
}

export async function ensureServiceWorker(): Promise<ServiceWorkerRegistration | null> {
  if (!pushSupported()) return null;
  const base = (window.__GITSEER_BASE__ || "").replace(/\/$/, "");
  const swUrl = `${base}/sw.js`;
  const scope = `${base}/` || "/";
  return navigator.serviceWorker.register(swUrl, { scope });
}

export async function enableBrowserAlerts(): Promise<AlertPrefs> {
  if (!("Notification" in window)) {
    throw new Error("This browser does not support notifications");
  }
  const perm = await Notification.requestPermission();
  if (perm !== "granted") {
    throw new Error("Notification permission was not granted");
  }

  let prefs = (await api.alertPrefs()).prefs;
  prefs = (
    await api.updateAlertPrefs({
      browser_enabled: true,
      push_enabled: prefs.push_configured ? true : prefs.push_enabled,
      min_severity: prefs.min_severity,
    })
  ).prefs;

  if (prefs.push_configured && prefs.vapid_public_key && pushSupported()) {
    const reg = await ensureServiceWorker();
    if (!reg) throw new Error("Service worker registration failed");
    await navigator.serviceWorker.ready;
    let sub = await reg.pushManager.getSubscription();
    if (!sub) {
      sub = await reg.pushManager.subscribe({
        userVisibleOnly: true,
        applicationServerKey: urlBase64ToUint8Array(prefs.vapid_public_key) as BufferSource,
      });
    }
    const json = sub.toJSON();
    await api.pushSubscribe({
      endpoint: json.endpoint || sub.endpoint,
      keys: {
        p256dh: json.keys?.p256dh || "",
        auth: json.keys?.auth || "",
      },
    });
    prefs = (await api.alertPrefs()).prefs;
  }

  try {
    localStorage.setItem(STORAGE_KEY, "1");
  } catch {
    /* ignore */
  }
  enabledCache = true;
  return prefs;
}

export async function disableBrowserAlerts(): Promise<AlertPrefs> {
  if (pushSupported()) {
    try {
      const reg = await navigator.serviceWorker.getRegistration();
      const sub = await reg?.pushManager.getSubscription();
      if (sub) {
        const endpoint = sub.endpoint;
        await sub.unsubscribe().catch(() => undefined);
        await api.pushUnsubscribe({ endpoint }).catch(() => undefined);
      }
    } catch {
      /* ignore */
    }
  }
  const prefs = (
    await api.updateAlertPrefs({ browser_enabled: false, push_enabled: false })
  ).prefs;
  try {
    localStorage.removeItem(STORAGE_KEY);
  } catch {
    /* ignore */
  }
  enabledCache = false;
  return prefs;
}

export function showBrowserNotification(ev: BrowserAlertEvent): void {
  if (!("Notification" in window) || Notification.permission !== "granted") return;
  if (typeof document !== "undefined" && document.visibilityState === "visible") {
    // Focused tab: skip OS banner (page already updating via SSE).
    return;
  }
  const title = ev.title?.trim() || "GitSeer attention";
  const bodyParts = [ev.repo_full, ev.severity].filter(Boolean);
  const n = new Notification(title, {
    body: bodyParts.join(" · "),
    tag: ev.id ? `gitseer-attention-${ev.id}` : "gitseer-attention",
  });
  n.onclick = () => {
    try {
      window.focus();
      const link = ev.deep_link;
      if (link) {
        if (link.startsWith("http://") || link.startsWith("https://")) {
          window.location.href = link;
        } else {
          const base = window.__GITSEER_BASE__ || "";
          window.location.href = `${base}${link.startsWith("/") ? link : `/${link}`}`;
        }
      }
    } finally {
      n.close();
    }
  };
}

export function browserAlertsLocallyEnabled(): boolean {
  if (enabledCache) return true;
  try {
    return localStorage.getItem(STORAGE_KEY) === "1";
  } catch {
    return false;
  }
}
