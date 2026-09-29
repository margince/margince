// Answers one request from Cache Storage: a navigation the network could not
// complete. The build emits this file after one line setting the global below.
const settings = self.__MARGINCE_SW_SETTINGS__;
const offlinePage = new URL(settings.offlinePage, self.location.origin).href;

async function install() {
  const cache = await self.caches.open(settings.cacheName);
  await cache.add(new Request(offlinePage, { cache: "reload" }));
  // The page's script reaches the page through the HTTP cache, never through
  // this worker, so it is fetched once here and read to the end to land there.
  const script = await self.fetch(settings.offlineScript);
  await script.arrayBuffer();
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
  if (event.request.mode !== "navigate") {
    return;
  }
  const { pathname } = new URL(event.request.url);
  // Prefix-matched the way the dev server matches its proxy keys: /mcp owns /mcp-apps.
  if (settings.passThrough.some((prefix) => pathname.startsWith(prefix))) {
    return;
  }
  event.respondWith(navigate(event));
});
