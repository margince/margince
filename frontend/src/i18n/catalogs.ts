import { bootstrapCopy } from "./bootstrapcopy_gen";
import type { MessageKey } from "./en";
import type { Locale } from "./locale";

// `Locale` is written in ./locale, which must load without the catalogs. The
// annotation fails the build when a locale is added to only one of the two.
export const catalogs: Record<Locale, Partial<Record<MessageKey, string>>> = {
  en: bootstrapCopy,
  de: bootstrapCopy,
  vi: bootstrapCopy,
};

export const loadedCatalogs = new Set<Locale>();
const catalogFailures = new Map<Locale, unknown>();
const catalogLoads = new Map<Locale, Promise<void>>();

export function loadLocale(locale: Locale): Promise<void> {
  if (loadedCatalogs.has(locale)) return Promise.resolve();
  const pending = catalogLoads.get(locale);
  if (pending) return pending;
  catalogFailures.delete(locale);
  const loading = (
    locale === "en"
      ? import("./en").then((module) => module.en)
      : locale === "de"
        ? import("./de").then((module) => module.de)
        : import("./vi").then((module) => module.vi)
  )
    .then((catalog) => {
      catalogs[locale] = catalog;
      loadedCatalogs.add(locale);
    })
    .catch((error: unknown) => {
      catalogLoads.delete(locale);
      catalogFailures.set(locale, error);
      throw error;
    });
  catalogLoads.set(locale, loading);
  return loading;
}

export function localeReady(locale: Locale): Promise<void> {
  if (catalogFailures.has(locale)) throw catalogFailures.get(locale);
  return loadLocale(locale);
}

export function resetLocaleFailures(): void {
  catalogFailures.clear();
}
