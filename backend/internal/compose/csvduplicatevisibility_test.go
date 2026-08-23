// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// A skipped row's REASON must not name a company the caller may not see.
//
// Closing this in the preview alone moved the leak rather than fixing it: a
// finished run's report is readable on the `import_run` grant, so a commit that
// wrote "a company of this name is already in the CRM" into it made the report
// the oracle instead of the preview. The two reasons exist so the decision and
// the disclosure can differ, which is the whole shape of the fix.

import (
	"strings"
	"testing"
)

func TestASkipReasonNamesNoCompanyTheCallerCannotSee(t *testing.T) {
	// The visible reason is allowed to say what happened, because the caller
	// could have read the record anyway.
	if !strings.Contains(duplicateSkipReason, "already in the CRM") {
		t.Errorf("the visible skip reason no longer says why the row was left alone: %q — a caller "+
			"who may read the incumbent should be told plainly", duplicateSkipReason)
	}

	// The opaque one must not. Any of these words turns it back into an answer
	// to "is this company in your CRM".
	for _, leak := range []string{"already", "in the CRM", "duplicate", "exists"} {
		if strings.Contains(strings.ToLower(opaqueSkipReason), strings.ToLower(leak)) {
			t.Errorf("the opaque skip reason contains %q: %q — it is shown for a collision with a "+
				"record the caller may not read, so it must not confirm that record exists",
				leak, opaqueSkipReason)
		}
	}
	if opaqueSkipReason == "" {
		t.Error("the opaque skip reason is empty; a row left alone with no reason reads as a bug")
	}
	if opaqueSkipReason == duplicateSkipReason {
		t.Error("the two skip reasons are identical, so the disclosure distinction does not exist")
	}
}
