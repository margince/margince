import { bootstrapCopy } from "./bootstrapcopy_gen";
import type { MessageKey } from "./en";
import type { Locale } from "./locale";

// `Locale` is written in ./locale, which must load without the catalogs. The
// annotation fails the build when a locale is added to only one of the two.
export const catalogs: Record<Locale, Partial<Record<MessageKey, string>>> = {
  en: { ...bootstrapCopy },
  de: { ...bootstrapCopy },
  vi: { ...bootstrapCopy },
};

export type CatalogLevel = "essential" | "complete";
export const loadedCatalogs = new Set<Locale>();
const essentialCatalogs = new Set<Locale>();
const catalogFailures = new Map<string, unknown>();
const catalogLoads = new Map<string, Promise<void>>();
const catalogLoaders = {
  en: () => import("./en").then((module) => module.en),
  de: () => import("./de").then((module) => module.de),
  vi: () => import("./vi").then((module) => module.vi),
};
const essentialLoaders = {
  en: () => import("./essential/en_gen.json").then((module) => module.default),
  de: () => import("./essential/de_gen.json").then((module) => module.default),
  vi: () => import("./essential/vi_gen.json").then((module) => module.default),
};

export function localeLoaded(locale: Locale, level: CatalogLevel): boolean {
  return (
    loadedCatalogs.has(locale) ||
    (level === "essential" && essentialCatalogs.has(locale))
  );
}

function loadCatalog(locale: Locale, level: CatalogLevel): Promise<void> {
  if (localeLoaded(locale, level)) return Promise.resolve();
  const key = `${locale}/${level}`;
  const pending = catalogLoads.get(key);
  if (pending) return pending;
  catalogFailures.delete(key);
  const loader = level === "complete" ? catalogLoaders : essentialLoaders;
  const loading = loader[locale]()
    .then((catalog) => {
      Object.assign(catalogs[locale], catalog);
      (level === "complete" ? loadedCatalogs : essentialCatalogs).add(locale);
    })
    .catch((error: unknown) => {
      catalogLoads.delete(key);
      catalogFailures.set(key, error);
      throw error;
    });
  catalogLoads.set(key, loading);
  return loading;
}

export function loadLocale(locale: Locale): Promise<void> {
  return loadCatalog(locale, "complete");
}

export function loadEssentialLocale(locale: Locale): Promise<void> {
  return loadCatalog(locale, "essential");
}

export function localeReady(
  locale: Locale,
  level: CatalogLevel = "complete",
): Promise<void> {
  const key = `${locale}/${level}`;
  if (catalogFailures.has(key)) throw catalogFailures.get(key);
  return loadCatalog(locale, level);
}

export function resetLocaleFailures(): void {
  catalogFailures.clear();
}
