import { formatActivityWhen } from "../format/activitywhen";
import type { Locale } from "../i18n/locale";
import type { WorklistItem } from "./worklisttypes";

type T = (key: "worklist.when.scheduled", values: { when: string }) => string;

/**
 * When a meeting that owes an outcome was scheduled. Nobody has said it took
 * place, so the row names the booked time, not a held one. On the record's
 * clock, so every colleague reads the same day.
 */
export function scheduledText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  record: string,
): string | null {
  if (item.source !== "meeting_outcome" || !item.occurred_at) return null;
  return t("worklist.when.scheduled", {
    when: formatActivityWhen(
      item.occurred_at,
      item.occurred_precision,
      locale,
      record,
    ),
  });
}
