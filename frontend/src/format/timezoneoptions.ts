// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Locale } from "../i18n";
import { forReader } from "./collate";
import { INTL_LOCALE } from "./intllocale";
import { browserZone, isRenderableZone, UTC_ZONE } from "./timezone";

// Zones ICU still lists under the name IANA has since replaced. A reader looks
// for today's city, so the list shows that name and finds the old one too.
const RENAMED_ZONES: Readonly<Record<string, string>> = {
  "Africa/Asmera": "Africa/Asmara",
  "America/Buenos_Aires": "America/Argentina/Buenos_Aires",
  "America/Catamarca": "America/Argentina/Catamarca",
  "America/Coral_Harbour": "America/Atikokan",
  "America/Cordoba": "America/Argentina/Cordoba",
  "America/Godthab": "America/Nuuk",
  "America/Indianapolis": "America/Indiana/Indianapolis",
  "America/Jujuy": "America/Argentina/Jujuy",
  "America/Louisville": "America/Kentucky/Louisville",
  "America/Mendoza": "America/Argentina/Mendoza",
  "Asia/Calcutta": "Asia/Kolkata",
  "Asia/Katmandu": "Asia/Kathmandu",
  "Asia/Rangoon": "Asia/Yangon",
  "Asia/Saigon": "Asia/Ho_Chi_Minh",
  "Atlantic/Faeroe": "Atlantic/Faroe",
  "Europe/Kiev": "Europe/Kyiv",
  "Pacific/Enderbury": "Pacific/Kanton",
  "Pacific/Ponape": "Pacific/Pohnpei",
  "Pacific/Truk": "Pacific/Chuuk",
};

const FORMER_NAMES = new Map<string, string[]>();
for (const [former, current] of Object.entries(RENAMED_ZONES)) {
  FORMER_NAMES.set(current, [...(FORMER_NAMES.get(current) ?? []), former]);
}

/**
 * The current IANA name of `zone`. An older browser that cannot render the
 * new name keeps the old one, so a pick never breaks the dates it formats.
 */
export function currentZoneName(zone: string): string {
  const current = RENAMED_ZONES[zone];
  return current && isRenderableZone(current) ? current : zone;
}

/** City-first labels keep typeahead useful; values remain IANA identifiers. */
export function timezoneOptions(locale: Locale, selected: string, at: Date) {
  const zones = new Set([
    UTC_ZONE,
    currentZoneName(browserZone()),
    ...(typeof Intl.supportedValuesOf === "function"
      ? Intl.supportedValuesOf("timeZone").map(currentZoneName)
      : []),
    ...(selected ? [selected] : []),
  ]);
  return Array.from(zones, (zone) => timezoneOption(locale, zone, at)).sort(
    (left, right) => forReader(left.label, right.label, locale),
  );
}

// The filter also matches the IANA id and a former city name, so "Saigon" and
// "Europe/Berlin" find their zone as well as the label does.
export function timezoneOption(locale: Locale, zone: string, at: Date) {
  const named = currentZoneName(zone);
  const formerCities = (FORMER_NAMES.get(named) ?? []).map((former) =>
    (former.split("/").pop() ?? former).replaceAll("_", " "),
  );
  return {
    value: zone,
    label: timezoneOptionLabel(locale, named, at),
    keywords: [zone, ...formerCities],
  };
}

function timezoneOptionLabel(locale: Locale, zone: string, at: Date): string {
  if (zone === UTC_ZONE) return UTC_ZONE;
  const parts = zone.replaceAll("_", " ").split("/");
  const city = parts.pop() ?? zone;
  const place = parts.length ? `${city} (${parts.join(" / ")})` : city;
  try {
    const offset = new Intl.DateTimeFormat(INTL_LOCALE[locale], {
      timeZone: zone,
      timeZoneName: "shortOffset",
    })
      .formatToParts(at)
      .find((part) => part.type === "timeZoneName")?.value;
    return offset ? `${place} · ${offset.replace("GMT", "UTC")}` : place;
  } catch (error) {
    if (!(error instanceof RangeError)) throw error;
    // Preserve a saved zone when the browser's timezone database is older.
    return place;
  }
}
