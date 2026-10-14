// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import "testing"

// A legal review request and its withdrawal read as sentences in the record's
// history, the way every other verb does.
func TestALegalReviewRequestAndWithdrawalReadAsSentences(t *testing.T) {
	for verb, want := range map[string]string{
		"request_legal_review":  "Rosa asked legal to review the record",
		"withdraw_legal_review": "Rosa withdrew the legal review request for the record",
	} {
		if got := composeRecordSummary("human", "Rosa", nil, verb, false, nil); got != want {
			t.Errorf("%s reads %q, want %q", verb, got, want)
		}
	}
}
