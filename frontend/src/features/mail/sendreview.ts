// What a rep can do about a send the engine refused.
//
// A refusal used to be a dead end wearing an explanation: "a recipient has not
// granted consent" and a link to the contact page, where there is nothing to
// change — the engine refused on a judgement about the message, not on a
// setting anybody can toggle. The rep read it, and stopped.
//
// The server now answers a refusal with the review it opened and what THIS
// caller may do about it. This reads that, and nothing more: which actions
// appear is the server's decision, because it is the only side that knows who
// is asking and what the review is in a state to accept.
//
// We do not infer the action. A surface that decided for itself would offer the
// button to colleagues the server will refuse. A button that fails when pressed
// is worse than one that is absent.

import { ProblemError } from "../../lib/problem";
import { problemDetails } from "../../lib/problembody";

// OverrulableRecipient is one refused contact the rep may vouch for, and the
// send category the vouch covers.
export type OverrulableRecipient = Readonly<{
  contactId: string;
  category: string;
}>;

// SendReview is the work a refusal left behind, as a surface needs it.
export type SendReview = Readonly<{
  reviewId: string;
  // What the overrule must vouch for: every refused contact, or nothing.
  overrulable: readonly OverrulableRecipient[];
  // What this caller may do about it. Empty is a real answer: an agent, or a
  // refusal with no message to decide about. The reference still travels so a
  // human can pick the review up.
  actions: readonly SendReviewAction[];
}>;

// The actions a surface knows how to draw. A server naming one this build has
// never heard of is dropped rather than rendered raw. An enum value such as
// `direct_send` tells a reader nothing as a button label.
//
// TWO, and a caller sees one of them. A holder of the communication exception
// grant directs the send; anyone else overrules a machine refusal with a reason.
// The server decides, because it is the side that knows.
//
// Dropping is safe because the reference survives. A rep still sees that a
// review exists and can open it; what they lose is one shortcut, which is the
// cost of a client older than its server.
export const SEND_REVIEW_ACTIONS = ["overrule", "direct_send"] as const;
export type SendReviewAction = (typeof SEND_REVIEW_ACTIONS)[number];

// sendReviewOf reads the review a refusal named, or null when it named none.
//
// NULL IS ORDINARY. Most refusals this surface sees are not consent refusals at
// all — a disconnected mailbox, a shared unsubscribe link — and those leave no
// review because there is no message being held for a decision.
export function sendReviewOf(error: unknown): SendReview | null {
  if (!(error instanceof ProblemError)) {
    return null;
  }
  const details = problemDetails(error.problem);
  if (!details) {
    return null;
  }
  const reviewId = details.review_id;
  if (typeof reviewId !== "string" || reviewId === "") {
    return null;
  }
  return {
    reviewId,
    actions: actionsIn(details.available_actions),
    overrulable: overrulableIn(details.overrulable),
  };
}

// overrulableIn keeps each well-formed entry. One malformed entry drops the
// whole list: an overrule that skipped a contact would leave the send refused.
function overrulableIn(raw: unknown): readonly OverrulableRecipient[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  const out: OverrulableRecipient[] = [];
  for (const entry of raw) {
    if (typeof entry !== "object" || entry === null) {
      return [];
    }
    const { contact_id: contactId, category } = entry as Record<
      string,
      unknown
    >;
    if (
      typeof contactId !== "string" ||
      contactId === "" ||
      typeof category !== "string" ||
      category === ""
    ) {
      return [];
    }
    out.push({ contactId, category });
  }
  return out;
}

// actionsIn keeps the actions this build can draw, in the server's own order.
// The order is the server's because it is the one that knows which is the
// likely next move.
function actionsIn(raw: unknown): readonly SendReviewAction[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  const known: SendReviewAction[] = [];
  for (const value of raw) {
    if (typeof value !== "string") {
      continue;
    }
    const action = SEND_REVIEW_ACTIONS.find((a) => a === value);
    // Each action once, whatever the server repeated: a button drawn twice is
    // a reader wondering which one is different.
    if (action && !known.includes(action)) {
      known.push(action);
    }
  }
  return known;
}
