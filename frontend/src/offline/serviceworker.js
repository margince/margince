// From Cache Storage it answers a navigation the network could not complete and
// the offline page's own script; the build puts its settings on a line above.
const settings = self.__MARGINCE_SW_SETTINGS__;
const offlinePage = new URL(settings.offlinePage, self.location.origin).href;
const offlineScript = new URL(settings.offlineScript, self.location.origin)
  .href;

async function install() {
  const cache = await self.caches.open(settings.cacheName);
  await cache.addAll([
    new Request(offlinePage, { cache: "reload" }),
    new Request(offlineScript, { cache: "reload" }),
  ]);
  await self.skipWaiting();
}

// Not navigation preload: a navigation left to the browser is then fetched twice,
// and a second GET can spend a single-use token.
async function activate() {
  const names = await self.caches.keys();
  await Promise.all(
    names
      .filter((name) => name !== settings.cacheName)
      .map((name) => self.caches.delete(name)),
  );
  await self.clients.claim();
}

async function cachedScript(request) {
  const cached = await self.caches.match(offlineScript, {
    cacheName: settings.cacheName,
  });
  return cached ?? self.fetch(request);
}

async function navigate(event) {
  try {
    return await self.fetch(event.request);
  } catch {
    const page = await self.caches.match(offlinePage, {
      cacheName: settings.cacheName,
    });
    return page ?? Response.error();
  }
}

self.addEventListener("install", (event) => {
  event.waitUntil(install());
});

self.addEventListener("activate", (event) => {
  event.waitUntil(activate());
});

self.addEventListener("fetch", (event) => {
  // Content-hashed and named by the offline page alone, so it can pin no build.
  if (event.request.url === offlineScript) {
    event.respondWith(cachedScript(event.request));
    return;
  }
  if (event.request.mode !== "navigate") {
    return;
  }
  const { pathname } = new URL(event.request.url);
  // The dev proxy's keys, matched by whole segment as the served app routes them.
  const owned = (prefix) =>
    pathname === prefix || pathname.startsWith(`${prefix}/`);
  if (settings.passThrough.some(owned)) {
    return;
  }
  event.respondWith(navigate(event));
});
