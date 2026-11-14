import { registerStartupSession } from "./app/startup-session";
import { loadEssentialLocale, loadLocale } from "./i18n/catalogs";
import { preferredLocale } from "./i18n/locale";
import { bootsOnEssentialCopy } from "./lib/essentialscreens";
import { restoreReturnRoute } from "./lib/returnroute";
import { parseHash } from "./lib/router";
import { readPreparedAnalyticsSession } from "./lib/session.read";
import { reloadOnStaleBuild } from "./lib/stalebuild";

reloadOnStaleBuild();
restoreReturnRoute();
if (parseHash(location.hash).screen === "analytics") {
  void import("./App.analytics").catch((error: unknown) =>
    console.error("Analytics code could not be loaded", error),
  );
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
}
const locale = preferredLocale();
const languageFailed = (error: unknown) =>
  console.error("The selected language could not be loaded", error);
if (bootsOnEssentialCopy(location.hash)) {
  loadEssentialLocale(locale).catch(languageFailed);
  // The rest of the app reads the complete catalog; it loads once the first
  // screen has had the main thread.
  addEventListener("load", () => {
    const later = (complete: () => void) =>
      typeof requestIdleCallback === "function"
        ? requestIdleCallback(complete)
        : setTimeout(complete, 0);
    later(() => void loadLocale(locale).catch(languageFailed));
  });
} else {
  loadLocale(locale).catch(languageFailed);
}
void import("./main");
