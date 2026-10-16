import type { components } from "../api/schema";
import { Badge } from "./atoms";

export type DecisionApproval = components["schemas"]["Approval"];

/**
 * How near the deadline is: the card's own reading of `expires_at`. This is the
 * one spelling of these thresholds in the product. A countdown badge takes its
 * tone from this function rather than carrying a second copy of the hours. A
 * deadline that reads urgent on the card and calm in the chip beside it is
 * worse than either reading on its own.
 */
export type DecisionUrgency = "calm" | "soon" | "urgent" | "lapsed";

const HOUR_MS = 60 * 60 * 1000;

/** Urgency from the milliseconds left. At or past zero the proposal has lapsed. */
export function decisionUrgency(msRemaining: number): DecisionUrgency {
  if (msRemaining <= 0) {
    return "lapsed";
  }
  if (msRemaining < HOUR_MS) {
    return "urgent";
  }
  return msRemaining < 6 * HOUR_MS ? "soon" : "calm";
}

/**
 * The deadline half of a staged proposal, which is all the readings below
 * touch. It is its own type for one field: `status` is a plain `string` here,
 * where the generated type has the wire union.
 *
 * The status vocabulary lives on the server and grows there. So a build is
 * always one deploy from meeting a status it has no word for. This tree settled
 * that for `kind` (`features/automations/approvalkind.ts`) the same way. It
 * renders what arrived rather than drop it. With the closed union, the compiler
 * would see the fallback in `verdictOf` as unreachable dead code. It would
 * still be the branch that runs the day the server adds a sixth status. A test
 * could not reach it at all.
 *
 * A full `Approval` satisfies this shape, so every existing caller passes one.
 */
export type DecisionDeadline = Readonly<{
  status: string;
  expires_at?: string | null;
}>;

/** When a staged proposal lapses, in wall-clock ms; null for one that never does. */
export function decisionExpiryMs(approval: DecisionDeadline): number | null {
  return approval.expires_at ? new Date(approval.expires_at).getTime() : null;
}

/**
 * Lapsed because the wire already stamped it (the server expires lazily at read
 * time), or because the live clock crossed `expires_at` since the list was
 * fetched. Both mean the same thing to a reader: it is no longer approvable, so
 * Accept is not drawn at all.
 */
export function decisionLapsed(
  approval: DecisionDeadline,
  now: number,
): boolean {
  const expiresAtMs = decisionExpiryMs(approval);
  return (
    approval.status === "expired" || (expiresAtMs != null && now >= expiresAtMs)
  );
}

/**
 * The countdown's tone, from the milliseconds left: warning under six hours,
 * danger under one, and nothing beyond (never inert grey text). The hours are
 * not spelled here either; `decisionUrgency` owns them. The card the chip sits
 * on tints its own edge from the same reading. With two copies of the
 * thresholds, one deadline can read urgent on the card and calm in the badge
 * beside it. That is worse than either reading alone.
 */
export function decisionUrgencyTone(
  msRemaining: number,
): "danger" | "warning" | undefined {
  return URGENCY_TONE[decisionUrgency(msRemaining)];
}

const URGENCY_TONE: Readonly<
  Record<DecisionUrgency, "danger" | "warning" | undefined>
> = {
  lapsed: "danger",
  urgent: "danger",
  soon: "warning",
  calm: undefined,
};

/** What a decided proposal ended up as. */
export type DecisionVerdict = "approved" | "rejected" | "expired";

const VERDICT_TONE: Readonly<
  Record<DecisionVerdict, "success" | "danger" | "warning">
> = {
  approved: "success",
  rejected: "danger",
  expired: "warning",
};

// A wire status this tier has not learned yet reads as `expired`. This branch
// is reachable because the status vocabulary grows on the server (see
// `DecisionDeadline`). A decided card with no badge says less than a slightly
// imprecise one. Whatever the contract adds to the decided set is still
// something nobody can act on any more.
function verdictOf(status: string): DecisionVerdict {
  if (status === "approved") {
    return "approved";
  }
  return status === "rejected" ? "rejected" : "expired";
}

/**
 * The status chip's words, on the same terms as `DecisionCardLabels`: this tier
 * knows when a deadline has passed, not what the reader's locale calls an hour.
 */
export type DecisionStatusLabels = Readonly<{
  /**
   * How long is left. It takes the span rather than an already-rendered
   * countdown because the units are copy too. "3h 0m" comes out of the message
   * catalogue like every other string. Also, only the chip knows whether a
   * countdown is drawn at all. A caller formatting one up front would format
   * spans nobody sees.
   */
  expiresIn: (msRemaining: number) => string;
  /** The verdict word, one per state a decided proposal can be in. */
  approved: string;
  rejected: string;
  expired: string;
}>;

/**
 * The decision's header chip: a verdict badge once it has been answered, else
 * the live countdown to its deadline.
 *
 * Every decision surface draws this one (the inbox's Decisions row and Brief's
 * deck both). A second countdown is a second answer to one deadline, and the
 * two drift the first time either moves.
 */
export function DecisionStatusChip({
  approval,
  decided,
  now,
  labels,
}: Readonly<{
  /** The deadline and the status; the chip reads nothing else off a proposal. */
  approval: DecisionDeadline;
  /** History rather than a question: the verdict, not the time it had left. */
  decided: boolean;
  now: number;
  labels: DecisionStatusLabels;
}>) {
  if (decided) {
    const verdict = verdictOf(approval.status);
    return <Badge tone={VERDICT_TONE[verdict]}>{labels[verdict]}</Badge>;
  }
  const expiresAtMs = decisionExpiryMs(approval);
  if (expiresAtMs == null) {
    return null;
  }
  // A lapsed pending decision carries no chip. The card it sits on already says
  // it ran out of time, where its verbs would have been. A badge repeating that
  // word one line above reads as two separate facts about one deadline.
  // Returning null here also keeps the countdown below off a negative span.
  if (decisionLapsed(approval, now)) {
    return null;
  }
  const remaining = expiresAtMs - now;
  return (
    <Badge tone={decisionUrgencyTone(remaining)}>
      {labels.expiresIn(remaining)}
    </Badge>
  );
}

/**
 * Names the tool that staged the proposal. The kind is meta on the line above.
 * This caption lets a reader tell "send_email" (the tool) from "advance_deal"
 * (the kind) without opening the detail.
 *
 * The verb arrives already looked up, with the words to wrap it in. The
 * kind→verb catalogue is the product's vocabulary, not this tier's. A primitive
 * holding a copy would be a second author of it. An unmapped kind gives no
 * verb, and the chip stays silent rather than naming a tool nobody can check.
 */
export function DecisionToolChip({
  verb,
  label,
}: Readonly<{
  verb: string | undefined;
  label: (verb: string) => string;
}>) {
  if (!verb) {
    return null;
  }
  return <span className="t-caption">{label(verb)}</span>;
}
