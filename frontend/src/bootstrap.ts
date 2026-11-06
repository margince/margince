import { loadLocale } from "./i18n/catalogs";
import { preferredLocale } from "./i18n/locale";
import { reloadOnStaleBuild } from "./lib/stalebuild";

reloadOnStaleBuild();
loadLocale(preferredLocale()).catch((error: unknown) => {
  console.error("The selected language could not be loaded", error);
});
void import("./main");
