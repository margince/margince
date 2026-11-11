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
import type { RefusedRecipient } from "../../lib/sendstatus";

// SendReview is the work a refusal left behind, as a surface needs it.
export type SendReview = Readonly<{
  reviewId: string;
  // What this caller may do about it. Empty is a real answer: an agent, or a
  // refusal with no message to decide about. The reference still travels so a
  // human can pick the review up.
  actions: readonly SendReviewAction[];
  // Each refused address with the engine's reason. Empty when the server named
  // none this build can read, which then reads as a refusal with no cause.
  refusals: readonly RefusedRecipient[];
}>;

// The actions a surface knows how to draw. A server naming one this build has
// never heard of is dropped rather than rendered raw. An enum value such as
// `direct_send` tells a reader nothing as a button label.
//
// One: directing this one message, with a reason. The server's standing
// per-contact `overrule` is not drawn, because an overrule here covers one
// message and never the next one to the same contact.
//
// Dropping is safe because the reference survives. A rep still sees that a
// review exists and can open it; what they lose is one shortcut, which is the
// cost of a client older than its server.
export const SEND_REVIEW_ACTIONS = ["direct_send"] as const;
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
    refusals: refusalsIn(details.refusals),
  };
}

// refusalsIn keeps each entry with an address and a reason. Dropping one
// malformed entry is safe: the status never reads fewer refusals as ready.
function refusalsIn(raw: unknown): readonly RefusedRecipient[] {
  if (!Array.isArray(raw)) {
    return [];
  }
  const out: RefusedRecipient[] = [];
  for (const entry of raw) {
    if (typeof entry !== "object" || entry === null) {
      continue;
    }
    const address: unknown = Reflect.get(entry, "address");
    const reasonCode: unknown = Reflect.get(entry, "reason_code");
    const category: unknown = Reflect.get(entry, "category");
    const requested: unknown = Reflect.get(entry, "requested_category");
    if (typeof address !== "string" || typeof reasonCode !== "string") {
      continue;
    }
    out.push({
      address,
      reasonCode,
      category: typeof category === "string" ? category : undefined,
      requestedCategory: typeof requested === "string" ? requested : undefined,
      // Only a stated yes counts. Absent is not permission to guess one.
      canBeOverruled: Reflect.get(entry, "can_be_overruled") === true,
    });
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
