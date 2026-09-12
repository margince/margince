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
  if (item.source === "brief_item" && item.kind) {
    const signal = item.kind;
    if ((BRIEF_SIGNALS as readonly string[]).includes(signal)) {
      return `worklist.signal.${signal}` as MessageKey;
    }
  }
  return `worklist.category.${item.category}` as MessageKey;
}

/**
 * The eyebrow as WORDS, which is `cause_label` where the row has one.
 *
 * `system` is the category that needed this. It is the queue's catch-all — the
 * product reporting on ITSELF: a mailbox that stopped, an automation that
 * failed, mail that bounced, a privacy deadline. Six categories name a kind of
 * work a reader recognises and the seventh named the software, so a column of
 * different broken things all read "System" and a reader ran down it learning
 * nothing about any of them.
 *
 * `cause_label` is the server's own words for the condition — minted by the
 * lane that knows which record IS the condition, never sampled from a member's
 * title. Nothing drew it before this.
 *
 * MOST SYSTEM ROWS STILL SAY "System", and that is the honest state rather than
 * an oversight. Three producers mint a label today — capture health names the
 * account, AI work health names the task, a failed automation names the rule
 * (renderhealthlanes.go). Sync health, bounces, undelivered mail, failed
 * approvals, privacy deadlines and relationship decay mint none, so those rows
 * keep the category word. Naming them needs each producer to say which record
 * IS its condition, which is work in the lane and not here; a browser inventing
 * a name from a row's title would be the sampled-member guess the contract
 * refuses. A folded incident is the same case: the fold mints a fresh row and
 * puts the group's name in `batch.label`, not here, so it keeps the word too —
 * and its headline already reads "{cause} failed {count} times".
 *
 * Null where the row has none, and the caller keeps the category word: absent
 * is a real answer here, and an empty eyebrow would be worse than a vague one.
 */
export function conditionOf(item: WorklistItem): string | null {
  return item.category === "system" && item.cause_label
    ? item.cause_label
    : null;
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
