import { formatDateTime } from "../format/format";
import type { Locale } from "../i18n/locale";
import type { WorklistItem } from "./worklist.queries";

type T = (key: "worklist.when.held", values: { when: string }) => string;

/**
 * When a meeting that owes an outcome took place. Without it every such row
 * reads alike, and a reader opens each to learn which meeting it is.
 */
export function heldText(
  item: WorklistItem,
  t: T,
  locale: Locale,
  viewer: string,
): string | null {
  if (item.source !== "meeting_outcome" || !item.occurred_at) return null;
  return t("worklist.when.held", {
    when: formatDateTime(item.occurred_at, locale, viewer),
  });
}
