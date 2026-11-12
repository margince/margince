// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

import type { Receipt } from "./handled";

// isOpenReceipt says the change still stands and the server offers its Undo.
// One a later change replaced, or one already undone, is settled.
export function isOpenReceipt(receipt: Receipt): boolean {
  return receipt.review
    ? !receipt.review.reversed && receipt.review.can_undo
    : Boolean(receipt.undo && !receipt.undo.reversed);
}

// receiptAuditId is the audit entry a correction receipt names. An approval
// has none, so its id never reaches an undo or a review request. Older
// servers send no audit_log_id; there a receipt with a review is a correction
// and its id is the audit entry.
export function receiptAuditId(receipt: Receipt): string | undefined {
  if (receipt.audit_log_id) return receipt.audit_log_id;
  if (receipt.source === "approval") return undefined;
  return (
    receipt.undo?.audit_log_id ?? (receipt.review ? receipt.id : undefined)
  );
}
