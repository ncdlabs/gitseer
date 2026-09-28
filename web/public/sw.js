/* GitSeer service worker — Web Push OS alerts when no app window is open. */
self.addEventListener("install", (event) => {
  event.waitUntil(self.skipWaiting());
});

self.addEventListener("activate", (event) => {
  event.waitUntil(self.clients.claim());
});

self.addEventListener("push", (event) => {
  event.waitUntil(handlePush(event));
});

self.addEventListener("notificationclick", (event) => {
  event.notification.close();
  const raw = event.notification.data || {};
  const path = typeof raw.deep_link === "string" ? raw.deep_link : "/attention";
  event.waitUntil(openDeepLink(path));
});

async function handlePush(event) {
  let data = { title: "GitSeer", body: "New attention alert", deep_link: "/attention" };
  try {
    if (event.data) {
      data = { ...data, ...event.data.json() };
    }
  } catch (_) {
    /* ignore malformed payload */
  }

  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  if (windows.length > 0) {
    // An open tab handles alerts via SSE; avoid duplicate OS banners.
    for (const client of windows) {
      try {
        client.postMessage({ type: "gitseer:push", payload: data });
      } catch (_) {
        /* ignore */
      }
    }
    return;
  }

  const title = data.title || "GitSeer";
  await self.registration.showNotification(title, {
    body: data.body || "",
    tag: data.id ? `gitseer-attention-${data.id}` : "gitseer-attention",
    data: { deep_link: data.deep_link || "/attention" },
    renotify: true,
  });
}

async function openDeepLink(target) {
  let url = target;
  try {
    if (target.startsWith("http://") || target.startsWith("https://")) {
      url = target;
    } else {
      const base = self.registration.scope.replace(/\/$/, "");
      url = `${base}${target.startsWith("/") ? target : `/${target}`}`;
    }
  } catch (_) {
    url = "/attention";
  }
  const windows = await self.clients.matchAll({ type: "window", includeUncontrolled: true });
  for (const client of windows) {
    if ("focus" in client) {
      await client.focus();
      if ("navigate" in client && typeof client.navigate === "function") {
        try {
          await client.navigate(url);
        } catch (_) {
          /* ignore */
        }
      }
      return;
    }
  }
  if (self.clients.openWindow) {
    await self.clients.openWindow(url);
  }
}
