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

/** What the worklist's receipt read says about one change. */
export type LineReceipt = Readonly<{
  receipt?: Receipt;
  // The read did not answer this time, or stopped before this change. A
  // cached or missing answer is not shown as current: its actions may no
  // longer be allowed.
  unavailable: boolean;
}>;

/** The receipt a one-record done line stands for. */
export function useLineReceipt(line: MagicLine): LineReceipt {
  const oneRecord = line.lane === "done" && (line.count ?? 1) === 1;
  return useReceiptFor(
    oneRecord ? line.receipt_ids : undefined,
    line.entity?.type,
  );
}

/**
 * The receipt with its own way back among these canonical ids, from the
 * worklist's receipt read. Only a deal change carries a review, so only a
 * deal can have lost one.
 */
export function useReceiptFor(
  receiptIds: readonly string[] | undefined,
  entityType: string | undefined,
): LineReceipt {
  const handled = useHandledForYou();
  if (!receiptIds?.length) {
    return { unavailable: false };
  }
  const reviewable = entityType === "deal";
  if (handled.isError) {
    return { unavailable: reviewable };
  }
  const receipt = handled.data?.receipts?.find(
    (candidate) =>
      receiptIds.includes(candidate.id) &&
      Boolean(candidate.review || candidate.undo),
  );
  return {
    receipt,
    unavailable: !receipt && reviewable && handled.data?.truncated === true,
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
