import { providerName } from "../format/connectorprovider";
import type { Translator } from "../i18n";
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
 * The word above a queue row, where the row's own category is the word.
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
  if (item.source === "notice") return "brief.updates.title";
  if (item.source === "weekly_commitment")
    return "worklist.source.weekly_commitment";
  if (item.source === "awaiting_reply" || item.source === "meeting_follow_up")
    return "worklist.source.awaiting_reply";
  if (item.source === "brief_item" && item.kind) {
    const signal = item.kind;
    if ((BRIEF_SIGNALS as readonly string[]).includes(signal)) {
      return `worklist.signal.${signal}` as MessageKey;
    }
  }
  if (item.source === "brief_item") return "search.kind.deal";
  return `worklist.category.${item.category}` as MessageKey;
}

// A system row's named condition, or null so the caller keeps the category
// word: an empty eyebrow would be worse than a vague one.
export function conditionOf(item: WorklistItem, t: Translator): string | null {
  if (item.category !== "system") return null;
  // A capture connection is named by its provider, and its account rides the
  // row's detail.
  if (item.connection) return providerName(item.connection.provider, t);
  return item.cause_label ?? null;
}

/**
 * The kind cell's class, which depends on what the cell holds.
 *
 * A category word fits the track it was sized for. A named condition does not
 * — "Notify sales on a new lead" is several times "Meeting" — and the badge
 * inside is `nowrap`, so it ran out of the cell and under the title. The named
 * variant clips with an ellipsis rather than wrapping: the column's argument is
 * that a reader runs DOWN it, and one row growing to two lines breaks the scan.
 * The caller puts the full name in `title` for a pointer.
 */
export function kindClass(named: string | null): string {
  return named === null
    ? "worklist-row-kind"
    : "worklist-row-kind worklist-row-kind-named";
}
