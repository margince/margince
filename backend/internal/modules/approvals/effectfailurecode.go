// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

// effectFailureCodes names each failure sentence this module records, so a
// client can word it in the reader's language and fall back to the sentence.
var effectFailureCodes = map[string]string{
	lapsedRedemptionSentence:  "approval_lapsed",
	effectNotRunSentence:      "effect_not_run",
	effectUnconfirmedSentence: "effect_unconfirmed",
	followUpFailedSentence:    "follow_up_failed",
	windowNotWidenedSentence:  "window_not_widened",
}

// effectFailureCode is the code of a recorded failure sentence. A sentence
// recorded in older words has none.
func effectFailureCode(sentence *string) *string {
	if sentence == nil {
		return nil
	}
	code, ok := effectFailureCodes[*sentence]
	if !ok {
		return nil
	}
	return &code
}
