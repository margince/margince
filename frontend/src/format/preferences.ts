import { useSyncExternalStore } from "react";
import type { components } from "../api/schema";

type Installation = components["schemas"]["InstallationSettings"];
export type DateFormat = NonNullable<Installation["date_format"]>;
export type TimeFormat = NonNullable<Installation["time_format"]>;
export type DateTimePreferences = Readonly<{
  dateFormat: DateFormat;
  timeFormat: TimeFormat;
}>;

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
  const day = parts.find((part) => part.type === "day")?.value ?? "";
  const month = parts.find((part) => part.type === "month")?.value ?? "";
  const year = parts.find((part) => part.type === "year")?.value ?? "";
  if (format === "dmy") return `${day}.${month}.${year}`;
  if (format === "mdy") return `${month}/${day}/${year}`;
  return `${year}-${month}-${day}`;
}
