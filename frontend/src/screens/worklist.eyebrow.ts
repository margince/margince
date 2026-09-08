import type { MessageKey } from "../i18n/en";
import type { WorklistItem } from "./worklist.queries";

/** The signals the overnight run can name, and the only ones this eyebrow trusts. */
const BRIEF_SIGNALS = [
  "closing_soon",
  "stalled",
  "opportunity",
  "moved",
] as const;

/**
 * The word above a queue row.
 *
 * Normally the row's category, which is the queue's own grouping. A brief row
 * is the exception: every one of them sits in `deals_at_risk`, because that is
 * where the worklist folds them against the sibling risk row for the same deal
 * — but the night knows WHY it picked each one, and a deal it ranked as winnable
 * announced as "Deal at risk" is the label the reader learns to distrust.
 *
 * Only the four signals the backend derives are honoured. A value from a newer
 * build falls back to the category rather than reaching `t` with a key that has
 * no translation, which would render the key itself.
 */
export function eyebrowKeyFor(item: WorklistItem): MessageKey {
  if (item.source === "brief_item" && item.kind) {
    const signal = item.kind;
    if ((BRIEF_SIGNALS as readonly string[]).includes(signal)) {
      return `worklist.signal.${signal}` as MessageKey;
    }
  }
  return `worklist.category.${item.category}` as MessageKey;
}
