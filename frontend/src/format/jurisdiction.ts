import type { Locale } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { INTL_LOCALE } from "./format";

// A jurisdiction pack is named by its ISO 3166-1 alpha-2 code. The reader sees
// the country's own name in their language, from the platform's region table,
// so no list of countries lives in this tree.

const regionNames = new Map<Locale, Intl.DisplayNames>();

function regionNamesFor(locale: Locale): Intl.DisplayNames {
  const held = regionNames.get(locale);
  if (held) {
    return held;
  }
  const made = new Intl.DisplayNames(INTL_LOCALE[locale], { type: "region" });
  regionNames.set(locale, made);
  return made;
}

/** The country a pack code names, in the reader's language. */
export function jurisdictionName(code: string, locale: Locale): string {
  try {
    return regionNamesFor(locale).of(code.toUpperCase()) ?? code.toUpperCase();
  } catch {
    // A code the platform table does not know (a reserved one in a test
    // build) is still a code the reader can quote.
    return code.toUpperCase();
  }
}

export type CalendarPeriod = Readonly<{
  years: number;
  months: number;
  days: number;
}>;

/**
 * The calendar parts of an ISO 8601 date period ("P6Y", "P1M", "P1Y6M2D").
 * Null for anything else. The server spells periods only this way, so another
 * shape is shown as written.
 */
export function parsePeriod(iso: string): CalendarPeriod | null {
  const match = /^P(?:(\d+)Y)?(?:(\d+)M)?(?:(\d+)D)?$/.exec(iso);
  if (!match || iso === "P") return null;
  return {
    years: Number(match[1] ?? 0),
    months: Number(match[2] ?? 0),
    days: Number(match[3] ?? 0),
  };
}

/**
 * The rule a request deadline names: the GDPR fallback, or a pack's own rule
 * set as the server spells it ("vn v1").
 */
export function ruleText(
  rule: string,
  t: (key: MessageKey, params?: Record<string, string>) => string,
): string {
  return rule === "gdpr"
    ? t("jurisdiction.rule.gdpr")
    : t("jurisdiction.rule.pack", { rule });
}
