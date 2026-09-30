# The installable app — manifest, service worker, offline page

Margince can be installed from the browser as an app: it gets its own window,
its own icon and its own place in the launcher or on the home screen. It is the
same web app from the same origin, not a second client. This page says what
the pieces are, what the service worker is allowed to do, and why it is
allowed so little.

## What ships today

| Piece | Where | What it does |
|---|---|---|
| Manifest | `frontend/public/manifest.webmanifest` | names the app, its colours, `start_url` and `display: standalone`, which is what makes it installable |
| Icons | `frontend/public/`: `icon.svg`, `favicon.ico`, `favicon-96x96.png`, `apple-touch-icon.png`, `web-app-manifest-192x192.png` and `-512x512.png` | the icons the browser tab, iOS and the manifest name; `frontend/src/app/sharepreview.test.ts` holds each to the size it declares |
| Service worker | source `frontend/src/offline/serviceworker.js`, emitted as `/sw.js` by `frontend/scripts/vite-pwa.ts` | answers a navigation the network could not complete with the offline page, and the offline page's own script; nothing else |
| Offline page | `frontend/src/offline/page.ts` (markup), `present.ts` (block, retry, reload on reconnect), `entry.ts`, `offline.css`; emitted as `/assets/offline-<hash>.html` | tells the reader the device cannot reach Margince, in their language, with a retry |
| Registration and install state | `frontend/src/app/pwa.ts` | registers the worker in a production build; keeps the browser's install offer for the app to present |

Client storage (the reader's language, theme and the rest) lives behind
`frontend/src/app/storage.ts`; the offline page reads the stored language
through it like every other reader does.

## The worker answers two requests, and neither is the app

The rule is short: **the worker answers two requests from Cache Storage and
no others.** A page navigation whose network fetch failed outright gets the
offline page. A request for the offline page's own script gets that script,
from the cache first. Every other request gets no `respondWith` at all and
takes the browser's own path. A navigation that reaches the server and comes
back with a 404 or a 500 is passed through untouched; only a fetch that
rejects (no network, no route to the host) gets the offline page.

That is narrower than a typical PWA on purpose. An earlier worker cached the
app shell cache-first under a fixed name, `margince-shell-v1`, so its eviction
step never deleted anything: a browser that loaded the app once kept serving
that build's `index.html`, and the content-hashed bundle it named, past every
deploy after it. A shipped screen read as missing for days. A worker that can
answer the app's own shell from a cache can pin a browser to a build; one that
answers only the failure case cannot. The offline script does not change
that: its name carries a hash of its content and nothing in the app loads it,
so serving it from the cache can pin no build. When the network works, the app
always comes from the server.

Navigations into what the api owns on this origin are not intercepted at all,
not even offline, so an OAuth consent, an MCP discovery document or a
webhook URL opened in a tab fails the way the browser fails it. The plugin
reads the list from the keys of the dev server's proxy in
`frontend/vite.config.ts` (`/v1`, `/setup`, `/oauth`, `/mcp`, `/.well-known`,
`/webhooks`, `/healthz`, `/readyz`, `/metrics`) and matches them by whole path
segment, the way the served app routes them rather than the way the dev server
does: `/mcp` and `/mcp/…` go to the network untouched, while `/mcp-apps/…`
is shipped files, and a failed navigation there gets the offline page. The
desktop launcher keeps its own copy of the list (`apiPrefixes` in
`desktop/launcher/web.go`), and `frontend/vite-proxy.test.ts` fails when that
copy and the proxy keys disagree.

The worker does **not** use navigation preload. With it on, the browser
requests every navigation in scope before the worker decides, including the
ones the worker then leaves alone, and a navigation left alone is fetched a
second time: two GETs of an address that may carry a single-use token, such as
an email confirmation or an OAuth callback. The app routes by hash, so the
worker sees a navigation only when the app launches or reloads, and the
start-up time preload would hide is paid rarely.

## How a build changes the worker

`frontend/scripts/vite-pwa.ts` runs inside `vite build` and emits three
files: `sw.js` at the site root, and under `assets/` the offline page
(`offline-<hash>.html`) and its one script (`offline-<hash>.js`), each named
for its content. The page lives under `assets/` for a rolling deploy: a
replica still on the previous build answers a name it lacks with 404 (nginx's
`/assets/` location is `try_files $uri =404`), so the new worker's install
fails and is retried instead of caching the app shell as the offline page.
The worker's source is plain JavaScript, emitted untouched after one line that
sets `self.__MARGINCE_SW_SETTINGS__` to this build's settings:

- **the cache name**, `margince-offline-<release>-<digest>`, where the release
  is `MARGINCE_RELEASE_VERSION` (`dev` when unset) and the digest is a hash of
  the offline page, the worker's source and the pass-through list;
  `workerCacheName` in `vite-pwa.ts` builds it, and `vite-pwa.test.ts` holds
  that an identical build keeps the name and that any change to those inputs,
  or to the release, renames it;
- the offline page's address and its script's content-hashed address;
- the pass-through prefixes.

A new release or a changed page gives `sw.js` new bytes. The browser checks
`/sw.js` on navigation, finds it changed and installs the new worker, which
precaches the offline page and its script under its own cache name (both or
the install fails) and calls `skipWaiting()`.
On `activate` it deletes **every** cache whose name is not its own (the old
`margince-shell-v1` included) and calls `clients.claim()`.

The browser fetches the worker script past its HTTP cache: `pwa.ts` registers
with `updateViaCache: "none"`, and browsers cap a worker script's HTTP freshness
at a day in any case. nginx still sends `Cache-Control: no-cache` for `/sw.js`
and `/manifest.webmanifest`, for any shared cache in front of it, and answers a
missing one with 404 rather than the app shell: a browser refuses an HTML page
as a worker script and keeps the worker it had. The offline page and its
script are cached for a year as immutable, which their content-hashed names
make safe. The desktop launcher (`desktop/launcher/web.go`) serves these files
with no cache headers of its own and needs none, for the same reason.

The build's own tests load the emitted worker into a stand-in for its global
scope and drive it: `frontend/scripts/vite-pwa.test.ts`.

## The offline page

The page is shown when the device cannot reach Margince at all. It carries one
block per locale the app ships, with the copy from the `offline.*` keys of the
catalogs, and its styles inline: `tokens.css` and `base.css` compiled into the
document, so its colours, type and button are the product's own in both
themes.

Its script is the one thing it loads. The site's content-security policy
allows no inline script, so the script is a file of its own under `/assets/`,
and the worker answers it from the cache it installed it into. The browser's
HTTP cache is not enough: `vite preview` and the desktop launcher send no
caching headers, and any browser may drop an entry.

The script shares the app's code rather than copying it. `startTheme()` from
`frontend/src/app/theme.ts` sets the stored theme, so an explicit light or dark
choice holds offline too and "system" follows the device. `preferredLocale()`
from `frontend/src/i18n/locale.ts`, a module that carries no catalog, picks the
block: the stored pick, then the browser's language, then English, the same
answer the app gives before the account says otherwise. The script sets the
title, makes "Retry" reload the address the reader asked for (route included),
and reloads by itself on the browser's `online` event, which is what the
page's sentence promises. Should the script still fail to load, the page reads
in English and its retry is a plain link that reloads the page without the
route.

## Registration and the install offer

`registerServiceWorker()` in `frontend/src/app/pwa.ts` registers `/sw.js` with
scope `/`, only in a production build, only where the browser has service
workers, and only after the window's `load` event, so installing the worker
never competes with the app's first load. A failed registration is logged with
`console.warn` and changes nothing else.
`frontend/src/app/serviceworker-registrar.test.ts` fails any shipped module but
`pwa.ts` that names `navigator.serviceWorker` in code, in any script dialect
under `frontend/src` or an extension's frontend, so a second registrar cannot
appear quietly.

`listenForInstall()` runs from `main.tsx` before the first render, because the
browser can make its offer before React mounts. It returns the function that
removes its listeners. `useInstallState()` answers one of:

| State | Meaning |
|---|---|
| `installed` | running as the installed app (`display-mode: standalone`, or iOS's `navigator.standalone`; a browser without `matchMedia` reads as not installed), or accepted or installed during this visit |
| `available` | the browser offered to install; `prompt()` asks it once and answers `accepted` or `dismissed`. A browser that refuses to show its dialog (a spent offer) answers `dismissed` and logs a warning, so the row never keeps offering a press that cannot work |
| `dismissed` | the reader turned the offer down; it holds until the browser offers again (`available`) or the app is installed |
| `manual-ios` | an iPhone or iPad browser, where Add to Home Screen is done by hand |
| `unavailable` | nothing this page can offer |

## Connectivity

`frontend/src/app/connectivity.ts` holds one of three states, and the shell's
banner (`app/connectivitybanner.tsx`) says which outage holds:

- **offline**: the browser reports no network (`navigator.onLine` and the
  `online`/`offline` events). Reads pause on every surface, as they always did.
- **unreachable**: a request to the api rejected at the network level, outlived
  its client deadline, or got a bare 502, 503 or 504 (a proxy saying the api is
  down; the api's own 5xx carries a problem body), and a `/healthz` probe sent
  at once failed too. One refused path on a working server declares nothing.
  None of it counts on a model route, where a long wait is the work.

**Only a surface that states the outage holds it open**, because a pause nothing
explains is a page that never loads: the shell's banner, and the connection
screen a failed first session check draws, which checks once more as it opens
so the probe can let the reader in unaided. A public page (unsubscribe,
preferences, booking, a buyer room) states none: a failure there is its own.

Once declared, `/healthz` is asked again after 2 seconds, doubling to a
30-second ceiling, never while the tab is hidden and at once when the tab or
network comes back; a 2xx or any api answer clears it. Writes never wait
(`networkMode: "always"`). One the offline device never sent says it was not
saved; one cut off in flight may have landed, so it says so and asks the reader
to check before retrying. A proxy's 5xx keeps the shared failure line.

## Install on this device

Settings → Account has a "This device" panel with one row,
`frontend/src/screens/thisdevice.tsx`, drawn from `useInstallState()` alone:

- `available`: Install asks the browser and pends while its dialog is open.
- `dismissed`: a sentence naming the browser's menu and its address-bar
  install icon. Chromium does not offer again in this page session.
- `manual-ios`: one sentence, Share and then Add to Home Screen.
- `installed`: the row says so, with nothing to press.
- `unavailable`: no row and no panel, since a browser that cannot install is a
  capability this device lacks, not a refusal.

Focus that falls with the button moves to the sentence or the "Installed" that
replaces it; focus anywhere else stays put. macOS Safari's File › Add to Dock
gets no row: it needs macOS 14, and Safari reports every macOS as 10.15.7.

## Turning the worker off in an emergency

If a shipped worker ever misbehaves, ship a `sw.js` at the same address that
removes itself. Every browser that holds the bad worker fetches `/sw.js` on
its next navigation, installs this one, and this one takes itself and every
cache away:

```js
self.addEventListener("install", () => self.skipWaiting());
self.addEventListener("activate", (event) => {
  event.waitUntil(
    (async () => {
      for (const name of await caches.keys()) await caches.delete(name);
      await self.registration.unregister();
      for (const client of await self.clients.matchAll({ type: "window" })) {
        client.navigate(client.url);
      }
    })(),
  );
});
```

Keep it at `/sw.js` for as long as any browser might still hold the old
worker. Deleting `sw.js` instead is not a removal: nginx answers 404, the
browser's update check fails, and what a browser does with the worker it
already holds is then up to the browser.

## Left for later

Each of these is a deliberate absence, not an oversight, and each needs its own
decision before it lands:

- **Push notifications.** A `push` handler in the worker and a subscription
  the api stores per seat.
- **App badging.** `navigator.setAppBadge()` from the app for the worklist
  count; no worker change needed while the app is open.
- **Share target.** A `share_target` entry in the manifest, so a shared link
  or file can land in a capture flow.
- **Shortcuts.** `shortcuts` in the manifest for the launcher's jump list.
- **Screenshots.** `screenshots` in the manifest for the richer install
  dialog.
- **Offline data.** Reading records without a connection. This is the one that
  would change the rule above, and it needs a design for staleness and for
  writes made offline before any response is cached.

## Where to go next

[frontend-architecture.md](frontend-architecture.md) (the app this worker
serves, and the gates that hold it) ·
[desktop-distribution.md](desktop-distribution.md) (the other way Margince is
served, on loopback) · [../deployment.md](../deployment.md) (the nginx and
ingress setup the headers above live in).
