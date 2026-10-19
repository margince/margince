import type { Locale } from "../i18n";

// Exported because a relative-time formatter needs the same mapping. With two
// tables, a page could format one date in German and the next in English.
export const INTL_LOCALE: Record<Locale, string> = {
  de: "de-DE",
  en: "en-GB", // A100: unconfigured English is en-GB, not en-US
  vi: "vi-VN",
};
