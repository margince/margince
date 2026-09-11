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
// WE DO NOT INFER THE ACTION. A surface that decided for itself that a rep may
// ask for a decision would offer the button to people the server will refuse —
// and a button that fails when pressed is worse than one that is absent,
// because the rep has been told they may do something they may not.

import { ProblemError } from "./common";

// SendReview is the work a refusal left behind, as a surface needs it.
export type SendReview = Readonly<{
  reviewId: string;
  // What this caller may do about it. Empty is a real answer: an agent, or a
  // refusal with no message to decide about. The reference still travels so a
  // person can pick the review up.
  actions: readonly SendReviewAction[];
}>;

// The actions a surface knows how to draw. A server naming one this build has
// never heard of is dropped rather than rendered raw: an enum value is not
// English, and putting `request_decision` in front of a reader as a button
// label tells them nothing.
//
// TWO, and a caller sees one of them. Which depends on whether they hold the
// authority to overrule the engine themselves — the server decides, because it
// is the side that knows.
//
// Dropping is safe because the reference survives. A rep still sees that a
// review exists and can open it; what they lose is one shortcut, which is the
// cost of a client older than its server.
export const SEND_REVIEW_ACTIONS = ["request_decision", "direct_send"] as const;
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
  const details = detailsOf(error.problem);
  if (!details) {
    return null;
  }
  const reviewId = details.review_id;
  if (typeof reviewId !== "string" || reviewId === "") {
    return null;
  }
  return { reviewId, actions: actionsIn(details.available_actions) };
}

// detailsOf reaches the problem's own details object without trusting its
// shape. The value arrives as `unknown` from the transport and a body that
// disagrees with the contract is a body this surface must survive rather than
// throw on: a rep whose send was refused should not also see a crash.
function detailsOf(problem: unknown): Record<string, unknown> | null {
  if (typeof problem !== "object" || problem === null) {
    return null;
  }
  const details = (problem as { details?: unknown }).details;
  if (
    typeof details !== "object" ||
    details === null ||
    Array.isArray(details)
  ) {
    return null;
  }
  return details as Record<string, unknown>;
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
