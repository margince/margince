// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The review controls of a receipt, on the one Done for you line that stands
// for it. The worklist's receipt read is only a detail source here, joined by
// the canonical receipt id. No line is hidden because a second list shows it.

import { useLocale, useT } from "../../i18n";
import { useHandledForYou } from "../worklist/worklist.queries";
import { receiptBasis } from "../worklist/worklist.receiptcopy";
import { ReceiptReview } from "../worklist/worklist.receiptreview";
import type { MagicLine } from "./magic.queries";

type Receipt = NonNullable<
  ReturnType<typeof useHandledForYou>["data"]
>["receipts"][number];

/** What the worklist's receipt read says about this one-record line. */
export type LineReceipt = Readonly<{
  receipt?: Receipt;
  // The detail read did not answer this time. A cached answer is not shown
  // as current: its actions may no longer be allowed.
  failed: boolean;
}>;

export function useLineReceipt(line: MagicLine): LineReceipt {
  const handled = useHandledForYou();
  if (
    line.lane !== "done" ||
    (line.count ?? 1) > 1 ||
    !line.receipt_ids?.length
  ) {
    return { failed: false };
  }
  // Only a deal change carries a receipt review, so only its line can have
  // lost one.
  if (handled.isError) {
    return { failed: line.entity?.type === "deal" };
  }
  return {
    failed: false,
    receipt: handled.data?.receipts?.find(
      (receipt) =>
        line.receipt_ids?.includes(receipt.id) &&
        Boolean(receipt.review || receipt.undo),
    ),
  };
}

export function LineReview({ receipt }: Readonly<{ receipt: Receipt }>) {
  const t = useT();
  const { locale } = useLocale();
  const basis = receiptBasis(receipt, t, locale);
  return (
    <div className="magic-line-controls">
      {basis && <p className="t-caption">{basis}</p>}
      <ReceiptReview receipt={receipt} />
    </div>
  );
}
