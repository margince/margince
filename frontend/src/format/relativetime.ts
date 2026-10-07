import type { Locale } from "../i18n";
import { INTL_LOCALE } from "./format";

/**
 * An instant as an offset from `now` in the largest whole unit, in the reader's
 * language ("3 hours ago", "in 12 minutes"). For a status that is read against
 * the clock, where an absolute time makes the reader do the subtraction.
 */
export function formatRelativeTime(
  at: string,
  locale: Locale,
  now: Date = new Date(),
): string {
  const seconds = Math.round((new Date(at).getTime() - now.getTime()) / 1000);
  const abs = Math.abs(seconds);
  const [unit, size] =
    abs >= 86_400
      ? (["day", 86_400] as const)
      : abs >= 3_600
        ? (["hour", 3_600] as const)
        : (["minute", 60] as const);
  return new Intl.RelativeTimeFormat(INTL_LOCALE[locale], {
    numeric: "auto",
  }).format(Math.trunc(seconds / size), unit);
}
