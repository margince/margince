// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package noticekeys

// OptOutReceiptMissedKind is the notice kind a missed required opt-out receipt
// raises. Shared so subject erasure can reach those notices.
const OptOutReceiptMissedKind = "optout_receipt_missed"

// OptOutReceiptPrefix starts every missed-receipt dedupe key.
const OptOutReceiptPrefix = "optout-receipt:"

// OptOutReceipt names the missed-receipt announcement for one stop act, by the
// receipt's message id.
func OptOutReceipt(messageID string) string { return OptOutReceiptPrefix + messageID }
