// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package magic

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// cutLanes names the lanes read to their bound, which may hold more than the
// page draws. A lane holding limit lines was stopped by the bound, not by
// running out. The done lane is also cut when its count is a floor. Watching
// is never named, because limit does not bound it.
func cutLanes(receipt crmcontracts.MagicReceipt, limit int) []crmcontracts.MagicReceiptCutAtLimit {
	cut := []crmcontracts.MagicReceiptCutAtLimit{}
	floor := receipt.DoneIsFloor != nil && *receipt.DoneIsFloor
	if floor || len(receipt.Done) >= limit {
		cut = append(cut, crmcontracts.MagicReceiptCutAtLimitDone)
	}
	if len(receipt.NeedsYou) >= limit {
		cut = append(cut, crmcontracts.MagicReceiptCutAtLimitNeedsYou)
	}
	if len(receipt.CouldNotComplete) >= limit {
		cut = append(cut, crmcontracts.MagicReceiptCutAtLimitCouldNotComplete)
	}
	return cut
}
