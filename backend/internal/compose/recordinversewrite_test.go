// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"fmt"
	"testing"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// A module that refuses at the write answers with the reason the button would
// have shown up front, so a refusal found late reads the same as one found
// early. Anything that is not a refusal travels on unchanged.
func TestAModuleRefusalReadsAsTheReasonTheButtonShows(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want Reason
	}{
		{"erased since", &storekit.RestoreRefusal{Reason: storekit.RestoreErased}, ReasonBehindErasureBoundary},
		{"brought back since", &storekit.RestoreRefusal{Reason: storekit.RestoreNotArchived}, ReasonSuperseded},
		{"merged away", &storekit.RestoreRefusal{Reason: storekit.RestoreMerged}, ReasonNotRestorableByThisPath},
		{"address taken", &storekit.RestoreRefusal{Reason: storekit.RestoreValueTaken, Detail: "email"}, ReasonNotRestorableByThisPath},
		{"field changed since", &contacts.FillRetractionRefusal{Moved: []string{"title"}}, ReasonSuperseded},
		{"value replaced", &contacts.FillRetractionRefusal{Replaced: []string{"phone"}}, ReasonNotRestorableByThisPath},
		{"no longer promoted", &contacts.NotPromotedError{}, ReasonSuperseded},
		{"contact on a live deal", &contacts.ContactHasDealError{}, ReasonNotRestorableByThisPath},
		{"colleague worked on it", &contacts.HumanTouchedError{EntityType: "contact"}, ReasonSuperseded},
	}
	for _, c := range cases {
		var refusal RefusedRestore
		if !errors.As(inverseWriteRefusal(fmt.Errorf("wrapped: %w", c.err)), &refusal) {
			t.Errorf("%s: not a refusal", c.name)
			continue
		}
		if refusal.Reason != c.want {
			t.Errorf("%s: reason = %q, want %q", c.name, refusal.Reason, c.want)
		}
	}
	boom := errors.New("the database went away")
	if got := inverseWriteRefusal(boom); !errors.Is(got, boom) {
		t.Errorf("a fault came back as %v, want it unchanged", got)
	}
}

// An un-archive is recognised by the archive it names, so a field restore is
// never mistaken for one and archived again.
func TestOnlyAnUnarchiveIsRedoneByArchiving(t *testing.T) {
	unarchive := AuditRow{EntityType: "company", Action: actionRestore, Evidence: []byte(`{"restores_archive":"x"}`)}
	field := AuditRow{EntityType: "company", Action: actionRestore, Evidence: []byte(`{"undid_audit_log_id":"x"}`)}
	if inverseOf(unarchive) != inverseRearchive {
		t.Error("an un-archive is not redone by archiving")
	}
	if inverseOf(field) != inverseNone {
		t.Error("a field restore is read as an un-archive")
	}
	if undoGrantFor(unarchive) != "delete" || undoGrantFor(field) != "update" {
		t.Errorf("grants = %q / %q, want delete for the archive and update for the field", undoGrantFor(unarchive), undoGrantFor(field))
	}
}
