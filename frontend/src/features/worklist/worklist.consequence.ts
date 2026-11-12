// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// What waiting costs on a row, in one sentence.

import type { MessageKey } from "../../i18n/en";
import { valueFor } from "../../lib/labelfor";
import { partyName, type T } from "../../lib/worklistcopy";
import type { WorklistItem } from "../../lib/worklisttypes";

const CONSEQUENCE: Readonly<
  Record<
    WorklistItem["consequence"],
    Readonly<{ named?: MessageKey; bare: MessageKey }> | null
  >
> = {
  none: null,
  buyer_waits: {
    named: "worklist.consequence.buyer_waits.named",
    bare: "worklist.consequence.buyer_waits",
  },
  promise_breaks: {
    named: "worklist.consequence.promise_breaks.named",
    bare: "worklist.consequence.promise_breaks",
  },
  deal_drifts: {
    named: "worklist.consequence.deal_drifts.named",
    bare: "worklist.consequence.deal_drifts",
  },
  deal_slips_past_close: {
    named: "worklist.consequence.deal_slips_past_close.named",
    bare: "worklist.consequence.deal_slips_past_close",
  },
  meeting_unprepared: { bare: "worklist.consequence.meeting_unprepared" },
  task_slips: {
    named: "worklist.consequence.task_slips.named",
    bare: "worklist.consequence.task_slips",
  },
  work_blocked: { bare: "worklist.consequence.work_blocked" },
  customer_never_received: {
    named: "worklist.consequence.customer_never_received.named",
    bare: "worklist.consequence.customer_never_received",
  },
  you_believe_it_happened: {
    bare: "worklist.consequence.you_believe_it_happened",
  },
  legal_deadline_missed: { bare: "worklist.consequence.legal_deadline_missed" },
  mailbox_blind: { bare: "worklist.consequence.mailbox_blind" },
  data_drifts: { bare: "worklist.consequence.data_drifts" },
};

// The deal consequences name the deal; every other one names the contact.
const DEAL_CONSEQUENCES = new Set<string>([
  "deal_drifts",
  "deal_slips_past_close",
]);

/**
 * What happens if nobody acts, or null when nothing does. A consequence this
 * build has no sentence for is left out, as an unknown reason is.
 */
export function consequenceLine(item: WorklistItem, t: T): string | null {
  const consequence = item.consequence;
  const keys = valueFor(CONSEQUENCE, consequence, null);
  if (!keys) return null;
  const name = DEAL_CONSEQUENCES.has(consequence)
    ? item.subject?.type === "deal"
      ? item.subject.label
      : undefined
    : partyName(item);
  return keys.named && name ? t(keys.named, { name }) : t(keys.bare);
}
