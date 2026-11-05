// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Receipt } from "./handled";

export function isReviewableReceipt(receipt: Receipt): boolean {
  return receipt.review
    ? !receipt.review.accepted &&
        !receipt.review.reversed &&
        (receipt.review.can_accept || receipt.review.can_undo)
    : Boolean(receipt.undo && !receipt.undo.reversed);
}
