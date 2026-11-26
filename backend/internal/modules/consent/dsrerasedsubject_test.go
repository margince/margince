// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A request whose subject an erasure retired names no one on the wire. The
// placeholder over its answer is flagged, and an answer written later is not.
func TestARetiredRequestSaysItsSubjectAndAnswerWereRemoved(t *testing.T) {
	contact := ids.NewV7()
	resolvedContact := map[ids.UUID]dsrSubject{contact: {kind: crmcontracts.DataSubjectRequestSubjectKindContact, label: "Anna Roth"}}
	retired := ids.NewV7()
	cases := []struct {
		name            string
		row             dsrRow
		erased, removed bool
	}{
		{
			name:   "a closed access request the erasure retired",
			row:    dsrRow{ID: retired, Kind: "access", Status: "fulfilled", SubjectRef: "erased+" + retired.String(), Resolution: new(erasedResolution)},
			erased: true, removed: true,
		},
		{
			name:   "an open request the erasure retired, keeping its link",
			row:    dsrRow{ID: retired, Kind: "access", Status: "open", SubjectRef: "erased+" + retired.String(), ContactID: &contact},
			erased: true,
		},
		{
			name:   "a retired request answered afterwards in the officer's words",
			row:    dsrRow{ID: retired, Kind: "access", Status: "fulfilled", SubjectRef: "erased+" + retired.String(), Resolution: new("Nothing left to send.")},
			erased: true,
		},
		{
			name: "the erasure that removed the contact, answered Erased",
			row:  dsrRow{ID: ids.NewV7(), Kind: "erasure", Status: "fulfilled", SubjectRef: contact.String(), Resolution: new(erasedResolution)},
		},
		{
			name: "a placeholder that names another case",
			row:  dsrRow{ID: ids.NewV7(), Kind: "access", Status: "fulfilled", SubjectRef: "erased+" + retired.String(), Resolution: new(erasedResolution)},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := wireDSR(tc.row, resolvedContact)
			if got.SubjectErased == nil || *got.SubjectErased != tc.erased {
				t.Errorf("subject_erased = %v, want %v", got.SubjectErased, tc.erased)
			}
			if got.ResolutionRemoved == nil || *got.ResolutionRemoved != tc.removed {
				t.Errorf("resolution_removed = %v, want %v", got.ResolutionRemoved, tc.removed)
			}
			if tc.erased && (got.SubjectLabel != nil || got.SubjectKind != nil) {
				t.Errorf("a retired subject was named %v (%v), want no record named", got.SubjectLabel, got.SubjectKind)
			}
		})
	}
}
