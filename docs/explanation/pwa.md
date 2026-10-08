<!-- prose:plain -->
# The app you can install: manifest, service worker, offline page

Margince can be installed from the browser as an app. It gets its own window, its own icon and its
own place in the launcher or on the home screen. It is the same web app from the same origin. Below:
what the parts are, what the service worker may do, and why its job is so small.

## What ships today

| Part | Where | What it does |
|---|---|---|
| Manifest | `frontend/public/manifest.webmanifest` | names the app, its colors, `start_url` and `display: standalone`, which makes it something you can install |
| Icons | `frontend/public/`: `icon.svg`, `favicon.ico`, `favicon-96x96.png`, `apple-touch-icon.png`, `web-app-manifest-192x192.png` and `-512x512.png` | the icons the browser tab, iOS and the manifest name; `frontend/src/app/sharepreview.test.ts` holds each one to what it declares |
| Service worker | source `frontend/src/offline/serviceworker.js`, written out as `/sw.js` by `frontend/scripts/vite-pwa.ts` | answers a page request the network could not complete with the offline page, and the offline page's own script; nothing else |
| Offline page | `frontend/src/offline/page.ts` (the HTML), `present.ts` (block, retry, load again when the network is back), `entry.ts`, `offline.css`; written out as `/assets/offline-<hash>.html` | tells the reader the device cannot reach Margince, in their language, with a retry |
| Sign-up and install state | `frontend/src/app/pwa.ts` | signs up the worker in a production build; keeps the browser's install offer for the app to show |

What the client stores (the language, theme and the rest) lives behind
`frontend/src/app/storage.ts`. The offline page reads the stored language through it, the same way
every other reader does.

## The worker answers two requests, and none is the app

The rule is short: the worker answers **two requests, and no others.** A page request (a
`navigation`) whose network fetch fails gets the offline page from `Cache Storage`, or a network
error when the cache has no copy. A request for the offline page's own script gets that script from
the cache, or from the network when the cache has none. Every other request gets no `respondWith`
at all and takes the browser's own path. A page request that gets a 404 or a 500 passes through;
only a refused fetch (no network, no route to the host) gets the offline page.

That is a smaller job than most service workers take on. A worker that answers the app shell from a
cache can pin a browser to an old build. The browser keeps serving that build's `index.html`, and
the JavaScript file it names by hash, past every later deploy.

This one answers only the case where the fetch fails, so it cannot. The offline script does not
change that. Its name carries a hash of its content and nothing in the app loads it, so serving it
from the cache can pin no build. When the network works, the app always comes from the server.

Page requests into what the API owns on this origin are never answered by the worker, not even
offline. So an OAuth consent, an MCP `/.well-known` document or a webhook URL opened in a tab fails
the way the browser fails it. `vite-pwa.ts` reads the list from the keys of the dev server's proxy
in `frontend/vite.config.ts` (`/v1`, `/setup`, `/oauth`, `/mcp`, `/.well-known`, `/webhooks`,
`/healthz`, `/readyz`, `/metrics`). It matches them by whole path part: the way the served app
routes them, rather than the way the dev server does. `/mcp` and `/mcp/…` go to the network as they
are. `/mcp-apps/…` is shipped files, so a failed page request there gets the offline page.

The desktop launcher keeps its own copy of the list (`apiPrefixes` in `desktop/launcher/web.go`).
`frontend/vite-proxy.test.ts` fails when that copy and the proxy keys disagree.

The worker does **not** use `navigation preload`. With it on, the browser requests every page in
scope before the worker decides, including the ones the worker then leaves alone. A page request
left alone is then fetched a second time. That is two GETs of an address that may carry a one-use
token, such as an email confirm link or an OAuth return. The app routes by hash, so the worker sees
a page request only when the app starts or loads again. The start time that `preload` would cut is a
cost only now and then.

## How a build changes the worker

`frontend/scripts/vite-pwa.ts` runs inside `vite build` and writes out three files. One is `sw.js`
at the site root. Under `assets/` are the offline page (`offline-<hash>.html`) and its one script
(`offline-<hash>.js`). Each is named for its content.

The page lives under `assets/` for a deploy that updates one server at a time. A copy of the server
still on the last build answers a name it does not have with 404 (in nginx, `/assets/` has
`try_files $uri =404`). The install of the new worker then fails and runs again, instead of storing
the app shell in its cache as the offline page.

The source of the worker is JavaScript with no build step. It is written out as it is, after one
line that sets `self.__MARGINCE_SW_SETTINGS__` to this build's settings:

- **the cache name**, `margince-offline-<release>-<digest>`. The release is
  `MARGINCE_RELEASE_VERSION` (`dev` when not set). `<digest>` is a hash of the offline page, the
  source of the worker and the list of paths it passes through. `workerCacheName` in `vite-pwa.ts`
  builds it. `vite-pwa.test.ts` holds that the same build keeps the name. It also holds that any
  change to those inputs, or to the release, renames it;
- the offline page's address and the address of its script, named by hash;
- the start of each path the worker passes through.

A new release or a changed page gives `sw.js` new bytes. The browser checks `/sw.js` on a page
request, finds it changed and installs the new worker. That worker stores the offline page and its
script under its own cache name (both, or the install fails) and calls `skipWaiting()`. On
`activate` it deletes **every** cache whose name is not its own (the old `margince-shell-v1`
included) and calls `clients.claim()`.

The browser fetches the worker script past its HTTP cache. `pwa.ts` signs it up with
`updateViaCache: "none"`. Browsers also keep a worker script in the HTTP cache for a day at most, in
any case.

The nginx server still sends `Cache-Control: no-cache` for `/sw.js` and `/manifest.webmanifest`, for
any shared cache between it and the browser. It answers a missing one with 404 rather than the app
shell. A browser refuses an HTML page as a worker script, and keeps the worker it has. The offline
page and its script stay in the cache for a year as files that never change, which their names by
hash make safe. The desktop launcher (`desktop/launcher/web.go`) serves these files with no cache
headers of its own and needs none, for the same reason.

The build's own tests load the worker it writes into a stand-in for the scope it runs in, and run
it: `frontend/scripts/vite-pwa.test.ts`.

## The offline page

The page shows when the device cannot reach Margince at all. It carries one block per language the
app ships, with the text from the `offline.*` keys of the catalogs. Its CSS is part of the page,
with `tokens.css` and `base.css` built in. So its colors, type and button are the product's own in
both themes.

Its script is the one thing it loads. The site's content security policy allows no script written
inside the page, so the script is a file of its own under `/assets/`. The worker answers it from the
cache it installed it into. The browser's HTTP cache is not enough: `vite preview` and the desktop
launcher send no cache headers, and any browser may drop an entry.

The script shares the app's code rather than copying it. `startTheme()` from
`frontend/src/app/theme.ts` sets the stored theme, so a `light` or `dark` theme the reader set holds
offline too, and `system` follows the device. `preferredLocale()` from `frontend/src/i18n/locale.ts`
chooses the block. That module carries no catalog. It takes the stored language, then the browser's
language, then English: the same answer the app gives before the account sets a language.

The script sets the title, and makes "Retry" load the address the reader asked for (route included).
It also loads the page again by itself on the browser's `online` event, which is what the page's
text promises. If the script still fails to load, the page reads in English. Its retry is then a
normal link that loads the page again without the route.

## Sign-up and the install offer

`registerServiceWorker()` in `frontend/src/app/pwa.ts` signs up `/sw.js` with scope `/`. It does so
only in a production build, and only where the browser has service workers. It also waits for the
window's `load` event, so installing the worker never slows the app's first load. A failed sign-up
is logged with `console.warn` and changes nothing else.

`frontend/src/app/serviceworker-registrar.test.ts` fails any shipped module but `pwa.ts` that names
`navigator.serviceWorker` in code. It reads every kind of script under `frontend/src` or the
frontend of an extension. So a second module that signs up a worker cannot come in without notice.

`listenForInstall()` runs from `main.tsx` before React shows the first screen, because the browser
can make its offer before React starts. It returns the function that undoes it. `useInstallState()`
answers one of:

| State | Meaning |
|---|---|
| `installed` | running as the installed app (`display-mode: standalone`, or iOS's `navigator.standalone`; a browser without `matchMedia` reads as not installed), or accepted or installed during this visit |
| `available` | the browser offered to install; `prompt()` asks it once and answers `accepted` or `dismissed`. A browser that refuses to show its dialog (a spent offer) answers `dismissed` and logs a warning, so the row never keeps offering a press that cannot work |
| `dismissed` | the reader turned the offer down; it holds until the browser offers again (`available`) or the app is installed |
| `manual-ios` | an iPhone or iPad browser, where Add to Home Screen is a step by hand |
| `unavailable` | nothing this page can offer |

## Network state

`frontend/src/app/connectivity.ts` holds one of three states. The shell's banner
(`app/connectivitybanner.tsx`) says which problem holds:

- **offline**: the browser reports no network (`navigator.onLine` and the
  `online`/`offline` events). Reads pause on every surface, as they always did.
- **unreachable**: a request to the api rejected at the network level, outlived
  its client deadline, or got a bare 502, 503 or 504. (A bare 5xx is a proxy
  saying the api is down; the api's own 5xx carries a problem body.) A
  `/healthz` probe sent at once failed too. One refused path on a working server declares nothing.
  None of it counts on a model route, where a long wait is the work.

Only a screen that states the problem holds it open, because a pause that nothing explains is a page
that never loads. Two screens do: the shell's banner, and the connection screen that a failed first
session check shows. That screen checks once more as it opens, so the check can let the reader in
without help. A public page (unsubscribe, settings, booking, a `buyer room`) states none: an error
there is its own.

Once the problem is declared, `/healthz` is asked again after 2 seconds, then after twice as long
each time, up to 30 seconds. It is never asked while the tab is not shown, and it is asked at once
when the tab or network comes back. A `2xx` or any API answer ends it. Writes never wait
(`networkMode: "always"`).

A write the offline device never sent says it is not saved. A write cut off on the way may have
reached the server, so it says so. It asks the reader to check before sending it again. A `5xx` from
a proxy keeps the shared error line.

## Install on this device

Settings → Account has a "This device" section with one row, `frontend/src/screens/thisdevice.tsx`,
built from `useInstallState()` alone:

- `available`: Install asks the browser, and waits while its install window is open.
- `dismissed`: a line of text naming the browser's menu and the install icon in its address line.
  Chromium does not offer again in this page session.
- `manual-ios`: one line: Share, then Add to Home Screen.
- `installed`: the row says so, with nothing to click.
- `unavailable`: no row and no section. A browser that cannot install is missing a part on this
  device; it has not refused.

When the button is removed and takes focus with it, focus moves to the text or to the "Installed"
that takes its place. Focus in any other place stays where it is. On macOS, the Safari menu entry
File › Add to Dock gets no row: it needs macOS 14, and Safari reports every macOS as 10.15.7.

## Turning the worker off when it goes wrong

If a shipped worker does harm, ship a `sw.js` at the same address that removes itself. Every browser
that holds the wrong worker fetches `/sw.js` on its next page request and installs this one. This
one then removes itself and every cache:

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

Keep it at `/sw.js` for as long as any browser may still hold the old worker. Deleting `sw.js`
instead does not remove the worker. The nginx server answers 404 and the browser's update check
fails. What a browser does with the worker it already holds is then up to the browser.

## Left for later

None of these ships today, and each needs its own decision before it ships:

- **Push messages.** A `push` handler in the worker, and a push sign-up the API stores per seat.
- **Count on the app icon.** `navigator.setAppBadge()` from the app for the Worklist count; no
  worker change needed while the app is open.
- **Share target.** A `share_target` entry in the manifest, so a shared link or file can land in
  capture.
- **Links for the launcher.** `shortcuts` in the manifest, for the list of links the launcher shows.
- **Install images.** `screenshots` in the manifest, for the fuller install window.
- **Offline data.** Reading records without a connection. This is the one that would change the rule
  above. It needs a design for data that is out of date and for writes made offline, before any
  response goes into the cache.

## Where to go next

- [frontend-architecture.md](frontend-architecture.md): the app this worker serves, and the gates
  that hold it.
- [desktop-distribution.md](desktop-distribution.md): the other way Margince is served, on the local
  machine.
- [../deployment.md](../deployment.md): the nginx and ingress setup the headers above live in.
