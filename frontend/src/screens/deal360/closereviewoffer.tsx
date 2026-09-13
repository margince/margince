import type { components } from "../../api/schema";
import { useCanWrite } from "../../app/capability";
import type { useT } from "../../i18n";
import {
  type ReviewTemplate,
  templateForOutcome,
  useReviewTemplates,
} from "../outcomereview.queries";
import { WON_REASON_LABELS, type WonReason } from "../winreason";
import { OutcomeReviewModal } from "./outcomereviewmodal";

type Deal = components["schemas"]["Deal"];

/** What a just-closed deal knows about itself, as the close handed it back. */
export type ClosedDeal = Readonly<{
  dealId: string;
  outcome: "won" | "lost";
  closingOccurrenceId: string;
  // The reason the close dialog required before it would close the deal. It
  // answers the review's own first question in different words, so it arrives
  // as that question's draft answer rather than being typed a second time.
  reason: string;
}>;

/**
 * The outcome review, offered at the moment the deal closes.
 *
 * Closing a deal already asks WHY — a lost deal cannot be closed without a
 * reason, and a win with no paperwork behind it has to name one. The review
 * then asks the same question again in its own words ("Why did we lose?"),
 * from a panel nothing points to, on a page the reader has usually left.
 * People answered the close and never saw the review, so the fuller answer
 * was missing on exactly the deals somebody had already thought about.
 *
 * This offers it while the deal is still in mind, with that first answer
 * already filled in. The close is NOT held open waiting for it: the deal is
 * closed and stays closed whether this is answered or dismissed. What is on
 * offer is the rest of the questions, not the outcome.
 *
 * Rendered by whichever surface owns the close dialog, so the board and the
 * record page make the same offer rather than one of them forgetting to.
 */
export function CloseReviewOffer({
  closed,
  onDismiss,
}: Readonly<{
  // The deal that just closed, or null when nothing is on offer.
  closed: ClosedDeal | null;
  onDismiss: () => void;
}>) {
  // The permission the SERVER checks, asked the way the review panel asks it.
  // A review is written as an activity, so `activity:create` decides it — not
  // the deal's writability, which a reader may hold without the grant to log
  // anything against it.
  const canWrite = useCanWrite("activity", "create");
  const { data: templates } = useReviewTemplates();
  const template = closed
    ? templateForOutcome(templates, closed.outcome)
    : undefined;
  // Everything the offer needs has to be true at once: the grant, a template
  // that asks the questions, and a closing to hang the answers on. Any of them
  // missing means no offer at all rather than a dialog that cannot save.
  if (!closed || !canWrite || !template) {
    return null;
  }
  return (
    <OutcomeReviewModal
      open
      onClose={onDismiss}
      dealId={closed.dealId}
      closingOccurrenceId={closed.closingOccurrenceId}
      template={template}
      prefill={firstAnswer(template, closed.reason)}
    />
  );
}

/**
 * The typed reason, filed under the question it already answers.
 *
 * The FIRST question of the template, not a key spelled here. Both seeded
 * templates open with the question the close dialog asks — "Why did we win?",
 * "Why did we lose?" — and the templates are editable, so hard-coding
 * `why_we_lost` would silently stop prefilling the day somebody reworded their
 * own first question. Reading position instead keeps the answer landing where
 * the reader just looked.
 *
 * Nothing is prefilled when the close carried no words: a win closed against a
 * signed contract is one click and states no reason at all, and seeding an
 * empty string would only make a required field look answered.
 */
function firstAnswer(
  template: ReviewTemplate,
  reason: string,
): Record<string, string> | undefined {
  const first = template.questions[0];
  if (!first || reason.trim() === "") {
    return undefined;
  }
  return { [first.key]: reason };
}

/**
 * The just-closed deal as the review offer needs it, or null when there is
 * nothing to offer.
 *
 * Null on a close the server recorded without a closing occurrence — an
 * outcome nobody can point at takes no review, the same rule the review panel
 * keeps — and null on a deal whose status came back as anything but won or
 * lost, which is not a close at all.
 */
export function closedDealOf(deal: Deal, reason: string): ClosedDeal | null {
  const outcome =
    deal.status === "won" || deal.status === "lost" ? deal.status : null;
  if (!outcome || !deal.closing_occurrence_id) {
    return null;
  }
  return {
    dealId: deal.id,
    outcome,
    closingOccurrenceId: deal.closing_occurrence_id,
    reason,
  };
}

export function isSavedDeal(result: unknown): result is Deal {
  return (
    typeof result === "object" &&
    result !== null &&
    typeof (result as Deal).id === "string" &&
    typeof (result as Deal).version === "number"
  );
}

/**
 * The reason this close stated, in words the review can start from.
 *
 * A LOST deal always has free text: the dialog will not close it without one,
 * so it passes through as typed. A WON deal usually has nothing — a win with a
 * signed contract behind it is one click and states no reason at all — and
 * where the server did ask, the answer is a picked reason rather than prose.
 * That picked reason is handed over as its own label, plus the detail where
 * "something else" was chosen, because "Something else" alone answers nothing
 * a reviewer can read later.
 */
export function closeReason(
  said: Readonly<{
    wonAsked: boolean;
    lost: string;
    won: WonReason | "";
    detail: string;
    t: ReturnType<typeof useT>;
  }>,
): string {
  if (!said.wonAsked) {
    return said.lost.trim();
  }
  if (said.won === "") {
    return "";
  }
  const label = said.t(WON_REASON_LABELS[said.won]);
  const detail = said.detail.trim();
  return detail === "" ? label : `${label} — ${detail}`;
}
