import { addressedScreenCode } from "./App.screencode";
import { registerStartupSession } from "./app/startup-session";
import { loadEssentialLocale, loadLocale } from "./i18n/catalogs";
import { effectiveDisplayLocale, preferredLocale } from "./i18n/locale";
import { paintsFromEssentialCopy } from "./lib/essentialscreens";
import { listenForInstall, registerServiceWorker } from "./lib/pwa";
import { restoreReturnRoute } from "./lib/returnroute";
import { parseHash } from "./lib/router";
import {
  readPreparedAnalyticsSession,
  sessionSeenNow,
} from "./lib/session.read";
import { reloadOnStaleBuild } from "./lib/stalebuild";

reloadOnStaleBuild();
restoreReturnRoute();
// main.tsx can run after the window's load event and Chrome's install offer, so this script catches both.
listenForInstall();
registerServiceWorker();
const route = parseHash(location.hash);
// Fetched beside the startup code rather than after the shell's gates.
addressedScreenCode(route)?.().catch((error: unknown) =>
  console.error("The screen's code could not be loaded", error),
);
if (route.screen === "analytics") {
  const controller = new AbortController();
  const prepared = readPreparedAnalyticsSession(controller.signal).then(
    (snapshot) => () => snapshot,
    (error: unknown) => () => {
      throw error;
    },
  );
  const fail = (error: unknown) =>
    console.error("Analytics startup could not be prepared", error);
  const preparation = Promise.all([
    import("./app/analytics-prefetch"),
    import("./app/queryclient.instance"),
  ]).then(([{ prefetchAnalytics }, { queryClient }]) => {
    void prefetchAnalytics(queryClient, async (signal) => {
      const abort = () => controller.abort(signal?.reason);
      if (signal?.aborted) abort();
      signal?.addEventListener("abort", abort, { once: true });
      try {
        return (await prepared)();
      } finally {
        signal?.removeEventListener("abort", abort);
      }
    }).catch(fail);
  });
  registerStartupSession(preparation.catch(fail));
} else if (sessionSeenNow()) {
  const preparation = Promise.all([
    import("./app/startup-reads"),
    import("./app/queryclient.instance"),
  ]).then(([{ prefetchStartupReads }, { queryClient }]) =>
    prefetchStartupReads(queryClient),
  );
  registerStartupSession(
    preparation.catch((error: unknown) =>
      console.error("The startup reads could not be prepared", error),
    ),
  );
}
const locale = preferredLocale();
const languageFailed = (error: unknown) =>
  console.error("The selected language could not be loaded", error);
loadEssentialLocale(locale).catch(languageFailed);
if (paintsFromEssentialCopy(location.hash)) {
  // The screen draws from the essential catalog alone; the rest of the app's
  // copy waits until the first screen has had the main thread.
  addEventListener("load", () => {
    const later = (complete: () => void) =>
      typeof requestIdleCallback === "function"
        ? requestIdleCallback(complete, { timeout: 5_000 })
        : setTimeout(complete, 0);
    later(
      () => void loadLocale(effectiveDisplayLocale()).catch(languageFailed),
    );
  });
} else {
  loadLocale(locale).catch(languageFailed);
}
void import("./main");
