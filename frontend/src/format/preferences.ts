import { useSyncExternalStore } from "react";
import type { components } from "../api/schema";

type Installation = components["schemas"]["InstallationSettings"];
export type DateFormat = NonNullable<Installation["date_format"]>;
export type TimeFormat = NonNullable<Installation["time_format"]>;
export type DateTimePreferences = Readonly<{
  dateFormat: DateFormat;
  timeFormat: TimeFormat;
}>;

export type DatePart = "day" | "month" | "year";

/** How a reader writes a day: which part comes first, and what stands between. */
export type DatePattern = Readonly<{
  order: readonly [DatePart, DatePart, DatePart];
  separator: string;
}>;

/** The patterns an installation can name; `locale` leaves it to the UI language. */
export const FIXED_DATE_PATTERNS: Readonly<
  Record<Exclude<DateFormat, "locale">, DatePattern>
> = {
  dmy: { order: ["day", "month", "year"], separator: "." },
  mdy: { order: ["month", "day", "year"], separator: "/" },
  ymd: { order: ["year", "month", "day"], separator: "-" },
};

const DEFAULTS: DateTimePreferences = {
  dateFormat: "locale",
  timeFormat: "locale",
};
let current = DEFAULTS;
const listeners = new Set<() => void>();

// Formatters also serve non-React readers. One snapshot keeps those readers and
// mounted locale consumers on the same installation preference without remounting forms.
export function dateTimePreferences(): DateTimePreferences {
  return current;
}

export function setDateTimePreferences(next: DateTimePreferences) {
  if (
    current.dateFormat === next.dateFormat &&
    current.timeFormat === next.timeFormat
  )
    return;
  current = next;
  for (const notify of listeners) notify();
}

function subscribe(notify: () => void) {
  listeners.add(notify);
  return () => {
    listeners.delete(notify);
  };
}

export function useDateTimePreferences(): DateTimePreferences {
  return useSyncExternalStore(subscribe, dateTimePreferences, () => DEFAULTS);
}

export function hourCycle(): Intl.DateTimeFormatOptions["hourCycle"] {
  const format = current.timeFormat;
  return format === "24h" ? "h23" : format === "12h" ? "h12" : undefined;
}

export function formatPreferredDate(
  formatter: Intl.DateTimeFormat,
  instant: Date,
): string {
  const format = current.dateFormat;
  if (format === "locale") return formatter.format(instant);
  const parts = formatter.formatToParts(instant);
  const pattern = FIXED_DATE_PATTERNS[format];
  return pattern.order
    .map((type) => parts.find((part) => part.type === type)?.value ?? "")
    .join(pattern.separator);
}
