// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"testing"

	"github.com/margince/margince/backend/internal/modules/deals"
)

func TestAVerdictCloseDateKeepsItsOwnReceiptKind(t *testing.T) {
	t.Parallel()
	for correction, want := range map[string]string{
		deals.VerdictCloseDateCorrection: deals.VerdictCloseDateCorrection,
		string(deals.CloseDateOverdue):   deals.CloseDateCorrectionKind,
	} {
		receipt := correctionReceipt(deals.CorrectionReceipt{Correction: correction, Fields: []string{"expected_close_date"}})
		if receipt.Kind != want || receipt.Undo == nil {
			t.Errorf("receipt of a %s change = kind %q undo %v, want kind %q with an Undo", correction, receipt.Kind, receipt.Undo, want)
		}
		if !deals.IsCorrectionReceiptKind(receipt.Kind) {
			t.Errorf("kind %q is not a correction receipt kind, so the sign-in page offers it as an approval", receipt.Kind)
		}
	}
}
