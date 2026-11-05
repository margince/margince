// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestAMissingReasonIsAReasonRequiredFieldFault(t *testing.T) {
	field, code, message := (&ReasonRequiredError{}).FieldFault()
	if field != "reason" || code != "reason_required" || message == "" {
		t.Errorf("fault = %q %q %q, want reason / reason_required / a message", field, code, message)
	}
}

func TestEveryStockSentenceIsRecognisedAndTheMarkerIsNot(t *testing.T) {
	for _, sentence := range StockManualSignalReasons() {
		if !isStockManualSignalReason(sentence) {
			t.Errorf("%q not recognised", sentence)
		}
	}
	if isStockManualSignalReason(ManualSignalNoEvidence) {
		t.Error("the marker matched as a stock sentence, so a second run would rewrite it")
	}
}
