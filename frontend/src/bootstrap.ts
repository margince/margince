import { queryClient } from "./app/queryclient.instance";
import { loadEssentialLocale, loadLocale } from "./i18n/catalogs";
import { preferredLocale } from "./i18n/locale";
import { restoreReturnRoute } from "./lib/returnroute";
import { parseHash } from "./lib/router";
import { sessionOptions } from "./lib/session";
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
  void queryClient.prefetchQuery(sessionOptions);
  void import("./App.analytics").catch((error: unknown) =>
    console.error("Analytics code could not be loaded", error),
  );
  void import("./features/analytics/analytics.prefetch")
    .then(({ prefetchAnalytics }) => prefetchAnalytics(queryClient))
    .catch((error: unknown) =>
      console.error("Analytics startup could not be prepared", error),
    );
}
const load =
  parseHash(location.hash).screen === "analytics"
    ? loadEssentialLocale
    : loadLocale;
load(preferredLocale()).catch((error: unknown) => {
  console.error("The selected language could not be loaded", error);
});
void import("./main");
