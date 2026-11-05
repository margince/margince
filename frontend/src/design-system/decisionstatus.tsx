import type { components } from "../api/schema";
import { Badge } from "./atoms";
import {
  type DecisionDeadline,
  decisionExpiryMs,
  decisionLapsed,
  decisionUrgencyTone,
} from "./decisiondeadline";

export type DecisionApproval = components["schemas"]["Approval"];

export {
  type DecisionDeadline,
  type DecisionUrgency,
  decisionExpiryMs,
  decisionLapsed,
  decisionUrgency,
  decisionUrgencyTone,
} from "./decisiondeadline";

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
