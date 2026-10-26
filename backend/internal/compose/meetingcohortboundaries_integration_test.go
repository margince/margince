// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cohort repair files a synced meeting under an attendee who became a
// contact later. These are its boundaries: a member's re-filing stands, the
// meeting keeps its time, an archived meeting and one at the link ceiling are
// left alone, and the meeting reads as its audience says afterwards.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var cohortMeetingAt = time.Date(2026, 9, 3, 14, 30, 0, 0, time.UTC)

// syncedMeeting writes a meeting the way a calendar sync left it before the
// attendee was filed: connector-captured, the attendee on a participant row,
// no link to them.
func syncedMeeting(t *testing.T, e *integration.Env, attendee ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, source_system, source_id)
		VALUES ($1, 'meeting', 'Quarterly review', $2, 'gcal:seed', 'connector:gcal', 'gcal', $3)`, id, cohortMeetingAt, "cohort-"+id.String())
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, contact_id, address, role) VALUES ($1, $2, 'guest@cohort.test', 'attendee')`, id, attendee)
	return id
}

func cohortContact(t *testing.T, e *integration.Env, name string) ids.ContactID {
	t.Helper()
	return ids.From[ids.ContactKind](e.SeedContact(t, name, &e.Rep1))
}

func repairCohort(t *testing.T, e *integration.Env, contact ids.ContactID) {
	t.Helper()
	if _, err := e.Contacts.RepairContactCohort(e.Admin(), contact); err != nil {
		t.Fatalf("repairing the cohort: %v", err)
	}
}

func meetingFiledUnder(t *testing.T, e *integration.Env, meeting ids.UUID, contact ids.ContactID) bool {
	t.Helper()
	return e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND contact_id = $2`, meeting, contact.UUID) == 1
}

func owedRepair(t *testing.T, e *integration.Env, contact ids.ContactID) bool {
	t.Helper()
	owed, err := e.Contacts.ContactsOwedACohortRepair(e.Admin(), 200)
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range owed {
		if id == contact {
			return true
		}
	}
	return false
}

func TestTheCohortRepairLeavesAMeetingAMemberRefiledElsewhere(t *testing.T) {
	e := integration.Setup(t)
	guest, other := cohortContact(t, e, "Guest Attendee"), cohortContact(t, e, "Other Contact")
	meeting := syncedMeeting(t, e, guest.UUID)
	repairCohort(t, e, guest)
	if !meetingFiledUnder(t, e, meeting, guest) {
		t.Fatal("the repair did not file the meeting under its attendee")
	}
	if got := e.WsScalar(t, `SELECT to_char(occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI')
		FROM activity WHERE id = $1`, meeting); got != "2026-09-03T14:30" {
		t.Errorf("the repair moved the meeting to %s", got)
	}

	// A member files the meeting under another contact instead.
	if _, err := e.Activities.RelinkActivity(e.Admin(), ids.From[ids.ActivityKind](meeting), activities.RelinkActivityInput{
		EntityType: "contact", EntityID: other.UUID, ReplaceExistingOfType: true,
	}); err != nil {
		t.Fatalf("re-filing the meeting: %v", err)
	}
	if meetingFiledUnder(t, e, meeting, guest) {
		t.Fatal("the re-filing left the meeting under the attendee")
	}
	if owedRepair(t, e, guest) {
		t.Error("the sweep offers the attendee again after a member re-filed their meeting")
	}
	repairCohort(t, e, guest)
	if meetingFiledUnder(t, e, meeting, guest) {
		t.Error("the repair undid the member's re-filing")
	}
	if !meetingFiledUnder(t, e, meeting, other) {
		t.Error("the repair took the meeting off the contact the member chose")
	}
}

func TestTheCohortRepairLeavesArchivedAndFullMeetingsAlone(t *testing.T) {
	e := integration.Setup(t)
	guest := cohortContact(t, e, "Guest Attendee")
	archived := syncedMeeting(t, e, guest.UUID)
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	full := syncedMeeting(t, e, guest.UUID)
	for i := range 25 {
		filler := cohortContact(t, e, "Filler "+string(rune('A'+i)))
		e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, contact_id) VALUES ($1, 'contact', $2)`, full, filler.UUID)
	}
	repairCohort(t, e, guest)
	if meetingFiledUnder(t, e, archived, guest) {
		t.Error("the repair filed an archived meeting")
	}
	if meetingFiledUnder(t, e, full, guest) {
		t.Error("the repair filed a meeting past the link ceiling")
	}
	if owedRepair(t, e, guest) {
		t.Error("the sweep keeps offering a contact whose only meetings the write refuses")
	}
}

func TestARepairedMeetingReadsAsItsAudienceSays(t *testing.T) {
	e := integration.Setup(t)
	guest := cohortContact(t, e, "Guest Attendee")
	meeting := syncedMeeting(t, e, guest.UUID)
	repairCohort(t, e, guest)
	audience := e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`, meeting)
	if _, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](meeting), storekit.LiveOnly); err != nil {
		t.Fatalf("the workspace admin cannot read the repaired meeting: %v", err)
	}
	colleague := e.As(e.Rep2, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}, "activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	_, err := e.Activities.GetActivity(colleague, ids.From[ids.ActivityKind](meeting), storekit.LiveOnly)
	if audience == "workspace" && err != nil {
		t.Errorf("a workspace meeting is refused to a colleague: %v", err)
	}
	if audience != "workspace" && err == nil {
		t.Errorf("a %s meeting reads for a colleague who is not in it", audience)
	}
}
