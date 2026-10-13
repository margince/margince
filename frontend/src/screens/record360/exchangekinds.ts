import type { MessageKey } from "../../i18n/en";

// The kinds that count as contact with the record, each with the word for one
// of it. The timeline also carries tasks and notes, which we wrote for
// ourselves, and the newest stop drawn from this list is titled "Last contact".
//
// A map keyed by kind, because `t` takes a declared MessageKey. Building the
// key from the kind would let a new kind ship printing its own id.
export const EXCHANGE_KINDS = {
  email: "co.spine.kind.email",
  call: "co.spine.kind.call",
  meeting: "co.spine.kind.meeting",
  message: "co.spine.kind.message",
} as const satisfies Record<string, MessageKey>;

// A meeting nobody held is not contact either. The server's
// relstrength.countingMeetingStatuses is the same rule from the other side.
export const CALLED_OFF_MEETING: ReadonlySet<string> = new Set([
  "canceled",
  "no_show",
]);

export type ExchangeKind = keyof typeof EXCHANGE_KINDS;

export function isExchange(kind: string): kind is ExchangeKind {
  return kind in EXCHANGE_KINDS;
}
