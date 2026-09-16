import type { Locale } from "../i18n";
import { assertIanaZone, displayDay, INTL_LOCALE } from "./format";

/**
 * A day split into the three pieces a CARD's own date tile draws on separate
 * lines (weekday, day number, month) rather than a single string a screen
 * would have to parse back apart to size one piece bigger than the others.
 *
 * Weekday and month come back in whatever case Intl's own locale name uses
 * ("Thu", "Do", "Th 5"); a tile that wants the weekday shouted does that with
 * `text-transform`, not by asking this function to pick a case for every
 * locale it does not know.
 */
export function dateTileParts(
  utcIso: string,
  locale: Locale,
  zone: string,
): { weekday: string; day: string; month: string } {
  assertIanaZone(zone);
  const parts = new Intl.DateTimeFormat(INTL_LOCALE[locale], {
    timeZone: zone,
    weekday: "short",
    day: "numeric",
    month: "short",
  }).formatToParts(displayDay(utcIso, zone));
  const read = (type: string) =>
    parts.find((part) => part.type === type)?.value ?? "";
  return { weekday: read("weekday"), day: read("day"), month: read("month") };
}
