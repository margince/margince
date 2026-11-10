// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The changed deals Done for you draws, read from the server's summary and
// dressed with the receipts loaded so far.

import type { Receipt } from "../../lib/handled";
import {
  type MarginceChangeBundle,
  shownUndoable,
} from "../../lib/marginchanges";
import { isReviewableReceipt, receiptAuditId } from "../../lib/receiptreview";

/** One deal and the changes on it that wait for the reader, newest first. */
export type DealChanges = Readonly<{
  dealId: string;
  name: string | undefined;
  receipts: readonly Receipt[];
  /** Changes the server lists on this deal that no loaded page holds yet. */
  unloaded: number;
}>;

/**
 * The changed deals, in the server's order when its summary is read. That
 * summary decides which deals and changes wait; the loaded receipts only
 * dress them. Without it, the receipts read so far are grouped as they come.
 * A receipt the reader settled on this visit stays with its answer, so a row
 * never leaves under the pointer that pressed it.
 */
export function dealChanges(
  receipts: readonly Receipt[] | undefined,
  settled: ReadonlySet<string>,
  bundles?: readonly MarginceChangeBundle[],
): DealChanges[] {
  const byDeal = new Map<string, Receipt[]>();
  for (const receipt of receipts ?? []) {
    const subject = receipt.subject;
    if (subject?.type !== "deal") continue;
    const group = byDeal.get(subject.id) ?? [];
    group.push(receipt);
    byDeal.set(subject.id, group);
  }
  if (!bundles) {
    return [...byDeal]
      .map(([dealId, group]) => ({
        dealId,
        name: group[0]?.subject?.label ?? undefined,
        receipts: group.filter(
          (receipt) => isReviewableReceipt(receipt) || settled.has(receipt.id),
        ),
        unloaded: 0,
      }))
      .filter((group) => group.receipts.length > 0);
  }
  const listedDeals = new Set(bundles.map((bundle) => bundle.deal_id));
  const groups: DealChanges[] = bundles.map((bundle) => {
    const listed = new Set(bundle.audit_log_ids);
    const rows = (byDeal.get(bundle.deal_id) ?? []).filter((receipt) => {
      const audit = receiptAuditId(receipt);
      return (
        (audit !== undefined && listed.has(audit)) || settled.has(receipt.id)
      );
    });
    const loaded = rows.filter((receipt) => {
      const audit = receiptAuditId(receipt);
      return audit !== undefined && listed.has(audit);
    }).length;
    return {
      dealId: bundle.deal_id,
      name: bundle.deal_name,
      receipts: rows,
      unloaded: Math.max(0, listed.size - loaded),
    };
  });
  for (const [dealId, group] of byDeal) {
    if (listedDeals.has(dealId)) continue;
    const kept = group.filter((receipt) => settled.has(receipt.id));
    if (kept.length > 0) {
      groups.push({
        dealId,
        name: kept[0]?.subject?.label ?? undefined,
        receipts: kept,
        unloaded: 0,
      });
    }
  }
  return groups;
}

/**
 * The receipts the changed-deals list draws, by id, so the done lane below it
 * leaves out those and no others.
 */
export function shownReceiptIds(
  receipts: readonly Receipt[] | undefined,
  bundles: readonly MarginceChangeBundle[] | undefined,
): string[] {
  return dealChanges(receipts, new Set(), bundles).flatMap((group) =>
    group.receipts.map((receipt) => receipt.id),
  );
}

/**
 * The changes an Undo of this deal may send: those the server still allows,
 * and only those this list shows the reader. An older server names no
 * undoable subset, so the shown changes that still read as reviewable stand in.
 */
export function undoableShown(
  group: DealChanges,
  bundle: MarginceChangeBundle | undefined,
): string[] {
  if (!bundle) return [];
  return shownUndoable(
    bundle,
    new Set(
      group.receipts
        .filter(isReviewableReceipt)
        .map(receiptAuditId)
        .filter((id): id is string => id !== undefined),
    ),
  );
}
