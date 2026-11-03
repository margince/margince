// The deadline of a staged proposal, as plain functions. The card, its chip and
// the MCP App views read the bands from here; a view carries no React.

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
 * The deadline half of a staged proposal. `status` is a plain string: the
 * server's vocabulary grows, and a build renders a status it has no word for.
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
