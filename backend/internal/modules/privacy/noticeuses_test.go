// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"slices"
	"testing"
	"time"
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
	want := []NoticeUse{{PurposeKey: businessCorrespondence, Basis: "subject_initiated_correspondence"}}
	if !slices.Equal(got, want) {
		t.Errorf("uses %v, want only the correspondence basis %v", got, want)
	}
}

// TestAConsentUseCarriesTheDateItWasGranted. The page names a consent by the
// day it was given.
func TestAConsentUseCarriesTheDateItWasGranted(t *testing.T) {
	granted := time.Date(2026, time.March, 2, 9, 0, 0, 0, time.UTC)
	got := recordedUses([]map[string]any{{"purpose": "marketing_email", "state": "granted", "captured_at": granted}}, nil)
	if len(got) != 1 || got[0].Since == nil || !got[0].Since.Equal(granted) {
		t.Errorf("uses %+v, want the consent dated %s", got, granted)
	}
}
