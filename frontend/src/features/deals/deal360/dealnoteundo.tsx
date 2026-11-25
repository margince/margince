// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { components } from "../../../api/schema";
import { BundleUndo } from "../../../components/marginchanges";
import { ReceiptVerbs } from "../../../components/receiptverbs";
import { useT } from "../../../i18n";
import type { Receipt } from "../../../lib/handled";
import { valueFor } from "../../../lib/labelfor";
import {
  type MarginceChangeBundle,
  noteReceipts,
} from "../../../lib/marginchanges";
import { isOpenReceipt, receiptAuditId } from "../../../lib/receiptstate";

type ReviewKind = components["schemas"]["AppliedDealChangeReview"]["kind"];
type BundleKind = MarginceChangeBundle["kinds"][number];

// The review names a change by its field, the bundle by its correction.
const BUNDLE_KIND: Readonly<Record<ReviewKind, BundleKind>> = {
  close_date: "deal_close_date",
  close_lost: "close_lost",
  amount: "deal_amount",
  probability: "deal_probability",
  stage: "stage_move",
  deal_open: "deal_open",
  deal_revive: "deal_revive",
};

/**
 * The Undo of the changes one History note reports, for as long as the server
 * offers it. One change presses the same Undo Home does. A night's several
 * changes press one bundle Undo, which the server takes in one transaction.
 */
export function DealNoteUndo({
  activityId,
  dealId,
  dealName,
  receipts,
}: Readonly<{
  activityId: string;
  dealId: string;
  dealName: string;
  receipts: readonly Receipt[];
}>) {
  const t = useT();
  const open = noteReceipts(receipts, activityId).filter(isOpenReceipt);
  const [first] = open;
  if (!first?.review) {
    return null;
  }
  if (open.length === 1) {
    return <ReceiptVerbs receipt={first} undoable={first.review.writable} />;
  }
  const audits = open.flatMap((receipt) => receiptAuditId(receipt) ?? []);
  return (
    <BundleUndo
      label={t("deal.note.undoAll")}
      bundles={[
        {
          deal_id: dealId,
          deal_name: dealName,
          version: first.review.version,
          audit_log_ids: audits,
          kinds: open.flatMap((receipt) =>
            receipt.review
              ? (valueFor(BUNDLE_KIND, receipt.review.kind, undefined) ?? [])
              : [],
          ),
          latest_at: first.occurred_at,
        },
      ]}
    />
  );
}
