import { providerName } from "../format/connectorprovider";
import type { Translator } from "../i18n";
import type { MessageKey } from "../i18n/en";
import { aiTaskLabel } from "./ai-task-label";
import type { WorklistItem } from "./worklisttypes";

/** The signals the overnight run can name, and the only ones this eyebrow trusts. */
const BRIEF_SIGNALS = [
  "closing_soon",
  "stalled",
  "opportunity",
  "moved",
] as const;

export function nonBuyerRequest(item: WorklistItem): boolean {
  return (
    item.source === "customer_waiting" &&
    (item.because?.some((reason) => reason.kind === "non_buyer_request") ??
      false)
  );
}

// Labels follow the row's evidence when its broad category would misstate the work.
export function eyebrowKeyFor(item: WorklistItem): MessageKey {
  if (item.source === "failed_approval")
    return "worklist.source.failed_approval";
  if (nonBuyerRequest(item)) return "worklist.source.awaiting_reply";
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
  if (item.ai_task || item.source === "ai_work_health")
    return aiTaskLabel(item.ai_task, t, item.cause_label);
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
