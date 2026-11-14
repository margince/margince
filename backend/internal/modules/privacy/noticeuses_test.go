// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"slices"
	"testing"
)

// TestTheNoticeItselfIsNotAUseOnRecord. Sending the privacy notice records a
// legal_obligation basis. Listed, it would be the only "use" a contact with
// nothing else on record is told about.
func TestTheNoticeItselfIsNotAUseOnRecord(t *testing.T) {
	bases := []map[string]any{
		{"kind": "legal_obligation"},
		{"kind": "subject_initiated_correspondence"},
	}
	got := recordedUses(nil, bases)
	want := []NoticeUse{{businessCorrespondence, "subject_initiated_correspondence"}}
	if !slices.Equal(got, want) {
		t.Errorf("uses %v, want only the correspondence basis %v", got, want)
	}
}
