// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package commsauthz

import (
	"slices"
	"testing"
)

// TestEveryReasonIsARefusalOrAnAllow reads every Reason constant off the
// package's own files. A code added under any name must be placed: in
// RefusalCodes, or named here as one of the two codes an allow carries.
func TestEveryReasonIsARefusalOrAnAllow(t *testing.T) {
	t.Parallel()
	allows := []string{ReasonAllowed, ReasonAllowedByOverride}
	refusals := RefusalCodes()
	declared := reasonConstants(t)
	for _, code := range declared {
		inRefusals, inAllows := slices.Contains(refusals, code), slices.Contains(allows, code)
		switch {
		case inRefusals && inAllows:
			t.Errorf("%q is listed as a refusal and is an allow code; take it out of RefusalCodes", code)
		case !inRefusals && !inAllows:
			t.Errorf("%q is a reason code that is neither in RefusalCodes nor an allow; add it to RefusalCodes in refusalcodes.go", code)
		}
	}
	for _, code := range refusals {
		if !slices.Contains(declared, code) {
			t.Errorf("RefusalCodes lists %q, which no Reason constant in this package declares", code)
		}
	}
	unique := slices.Clone(refusals)
	slices.Sort(unique)
	if len(slices.Compact(unique)) != len(refusals) {
		t.Errorf("RefusalCodes lists a code twice: %v", refusals)
	}
}
