// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What holds the lead half of communication_override unreachable, and what must
// land the day it stops being.
//
// The table, the merge carry, the erasure and retention sweeps and the SAR
// section all carry a lead arm — the shape communication_suppression has — but
// no door writes one, and liveOverride reads contact_id alone: Allow takes a
// contact and nothing else, and the carry's lead target is reached only from a
// lead-to-lead merge that never calls it. Two things are therefore missing on purpose rather than by oversight, and
// both would be silent defects the moment a lead vouch became writable:
//
//   - decideLead (authorizelead.go) never consults liveOverride, and
//     liveOverride itself matches contact_id alone, so a lead override would
//     be recorded and then ignored by every send about that lead.
//   - promote.go carries a lead's STOPS onto the new contact and would have to
//     carry the vouch with them, or a promotion would drop it exactly as the
//     contact merge used to.
//
// The test below is what makes that a failure instead of a comment: it fails on
// the change that makes a lead override writable, and names the rest.

import (
	"reflect"
	"strings"
	"testing"
)

// TestTheAllowDoorTakesNoLeadSubject pins the one fact the two gaps above rest
// on. It reads AllowInput's own fields rather than a list of names kept beside
// it, so a lead subject spelled any way at all — LeadID, Lead, SubjectLeadID —
// trips it.
func TestTheAllowDoorTakesNoLeadSubject(t *testing.T) {
	t.Parallel()

	in := reflect.TypeFor[AllowInput]()
	if in.NumField() == 0 {
		t.Fatal("AllowInput has no fields at all — this gate is reading the wrong type")
	}
	for field := range in.Fields() {
		name := field.Name
		if !strings.Contains(strings.ToLower(name), "lead") {
			continue
		}
		t.Errorf("AllowInput.%s makes a lead override writable. Four things now owe a change in "+
			"the same diff: decideLead must consult liveOverride or every send about that lead "+
			"ignores the vouch; liveOverride must grow a lead_id arm; promote.go must carry it onto the new contact beside the stops it "+
			"already carries; and contacts.carryOverridesTx must reach the lead-to-lead merge the "+
			"way carryStopsTx does", name)
	}
}
