// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package httperr

import "testing"

// A client holding only an answer's code reads the same verdict the Fault gives.
func TestAnAnswersCodeTellsWhetherItClearsOnItsOwn(t *testing.T) {
	t.Parallel()
	for code, want := range map[string]bool{
		writeContentionCode: true, WebsiteFormBusyCode: true, requestTimedOutCode: true, schemaChangedCode: true,
		"not_found": false, "service_unavailable": false, "": false,
	} {
		if got := TransientCode(code); got != want || (Fault{Code: code}).Transient() != want {
			t.Errorf("TransientCode(%q) = %v, want %v, the same as the Fault's", code, got, want)
		}
	}
}
