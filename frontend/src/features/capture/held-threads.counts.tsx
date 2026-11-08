// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../api/schema";
import { formatNumber } from "../../format/format";
import { useLocale, useT } from "../../i18n";
import type { MessageKey } from "../../i18n/en";

type HeldThread = components["schemas"]["HeldThread"];
type HeldThreadCounts = components["schemas"]["HeldThreadCounts"];

/** A ledger status the held list holds. */
export type HeldStatus = "pending" | "unsure" | "held" | "held_by_owner";

export const HELD_STATUSES: readonly HeldStatus[] = [
  "pending",
  "unsure",
  "held",
  "held_by_owner",
];

export const heldStatusLabel: Record<HeldStatus, MessageKey> = {
  pending: "heldThreads.count.pending",
  unsure: "heldThreads.count.unsure",
  held: "heldThreads.count.held",
  held_by_owner: "heldThreads.count.heldByOwner",
};

// The whole list by status, so a stuck classifier backlog reads apart from
// real holds at a glance.
export function HeldCounts({ counts }: Readonly<{ counts: HeldThreadCounts }>) {
  const t = useT();
  const { locale } = useLocale();
  const parts = HELD_STATUSES.filter((status) => counts[status] > 0).map(
    (status) =>
      `${formatNumber(counts[status], locale)} ${t(heldStatusLabel[status])}`,
  );
  if (parts.length === 0) {
    return null;
  }
  return <p className="t-caption">{parts.join(" · ")}</p>;
}

/**
 * Every held thread the list holds, whatever the page narrows to. A server
 * older than the counts sent the whole list, so its rows are the total.
 */
export function heldTotal(
  page: Readonly<{ data: readonly HeldThread[]; counts?: HeldThreadCounts }>,
): number {
  const counts = page.counts;
  if (!counts) {
    return page.data.length;
  }
  return counts.pending + counts.unsure + counts.held + counts.held_by_owner;
}
