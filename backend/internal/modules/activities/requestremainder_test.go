// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import "testing"

func TestRemainingAskExcludesSignatureHistoryAndTrivialFragments(t *testing.T) {
	t.Parallel()
	body := "Please send the report.\n\n-- \nKind regards\nA sender"
	for _, quote := range []string{"Please", "Please .", "Kind regards", "A sender"} {
		if RequestRemainderGrounded(body, quote) {
			t.Errorf("non-duty fragment admitted: %q", quote)
		}
	}
	if !RequestRemainderGrounded(body, "send the report") {
		t.Fatal("original request was lost")
	}
	if RequestRemainderGrounded("-- \nSend the report", "Send the report") {
		t.Fatal("signature-only message supplied an ask")
	}
}
