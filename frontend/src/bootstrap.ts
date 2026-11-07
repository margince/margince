import { registerStartupSession } from "./app/startup-session";
import { loadEssentialLocale, loadLocale } from "./i18n/catalogs";
import { preferredLocale } from "./i18n/locale";
import { restoreReturnRoute } from "./lib/returnroute";
import { parseHash } from "./lib/router";
import { readPreparedAnalyticsSession } from "./lib/session.read";
import { reloadOnStaleBuild } from "./lib/stalebuild";

for (const link of document.querySelectorAll<HTMLLinkElement>(
  "link[data-font-stylesheet]",
)) {
  link.rel = "stylesheet";
  link.removeAttribute("as");
}

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
const load =
  parseHash(location.hash).screen === "analytics"
    ? loadEssentialLocale
    : loadLocale;
load(preferredLocale()).catch((error: unknown) => {
  console.error("The selected language could not be loaded", error);
});
void import("./main");
