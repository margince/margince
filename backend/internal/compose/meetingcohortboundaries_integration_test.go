// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cohort repair files a synced meeting under an attendee who became a
// contact later. These are its boundaries. A member's re-filing stands, and
// the meeting keeps its time. An archived meeting and one at the link ceiling
// are left alone. The meeting reads as its audience says afterwards.

import (
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var cohortMeetingAt = time.Date(2026, 9, 3, 14, 30, 0, 0, time.UTC)

// syncedMeeting writes a meeting the way capture leaves it before the attendee
// is a contact. It is connector-captured, the attendee is an address on a
// participant row, and nothing links it. With no counterparty, capture holds
// it for its participants until a link files it.
func syncedMeeting(t *testing.T, e *integration.Env, address string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	e.WsExec(t, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by, source_system, source_id, audience, audience_reason)
		VALUES ($1, 'meeting', 'Quarterly review', $2, 'gcal:seed', 'connector:gcal', 'gcal', $3, 'participants', $4)`,
		id, cohortMeetingAt, "cohort-"+id.String(), activities.ReasonNoCounterparty)
	e.WsExec(t, `INSERT INTO activity_participant (activity_id, address, role) VALUES ($1, $2, 'attendee')`, id, address)
	e.WsExec(t, `INSERT INTO capture_import (activity_id, user_id, posture_at_import) VALUES ($1, $2, 'shared')`, id, e.Rep1)
	return id
}

// cohortContact is a contact created after the meeting synced, at the address
// the meeting names.
func cohortContact(t *testing.T, e *integration.Env, name, address string) ids.ContactID {
	t.Helper()
	row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: name, Source: "manual", OwnerID: new(ids.From[ids.UserKind](e.Rep1)),
		Emails: []contacts.ContactEmailInput{{Email: address, EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.From[ids.ContactKind](ids.UUID(row.Id))
}

func repairCohort(t *testing.T, e *integration.Env, contact ids.ContactID) {
	t.Helper()
	// The store as the reconciliation worker builds it, re-deriving the
	// audience of what it files (cohortpromotegen.go).
	store := contacts.NewStore(e.DB()).WithAudienceRecompute(activities.RecomputeAudienceTx)
	if _, err := store.RepairContactCohort(e.Admin(), contact); err != nil {
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
	return slices.Contains(owed, contact)
}

func TestTheCohortRepairLeavesAMeetingAMemberRefiledElsewhere(t *testing.T) {
	e := integration.Setup(t)
	meeting := syncedMeeting(t, e, "guest@cohort.test")
	guest := cohortContact(t, e, "Guest Attendee", "guest@cohort.test")
	other := cohortContact(t, e, "Other Contact", "other@cohort.test")
	repairCohort(t, e, guest)
	if !meetingFiledUnder(t, e, meeting, guest) {
		t.Fatal("the repair did not file the meeting under its attendee")
	}
	if got := e.WsScalar(t, `SELECT to_char(occurred_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI')
		FROM activity WHERE id = $1`, meeting); got != "2026-09-03T14:30" {
		t.Errorf("the repair moved the meeting to %s", got)
	}

	// A member files the meeting under another contact instead. The re-filing
	// also repoints the attendee row to the contact they chose, so the sweep
	// and the repair no longer name the attendee.
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
	archived := syncedMeeting(t, e, "guest@cohort.test")
	e.WsExec(t, `UPDATE activity SET archived_at = now() WHERE id = $1`, archived)
	full := syncedMeeting(t, e, "guest@cohort.test")
	guest := cohortContact(t, e, "Guest Attendee", "guest@cohort.test")
	for i := range 25 {
		filler := ids.From[ids.ContactKind](e.SeedContact(t, "Filler "+string(rune('A'+i)), &e.Rep1))
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
	meeting := syncedMeeting(t, e, "guest@cohort.test")
	colleague := e.As(e.Rep2, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}, "activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	readsContent := func() bool {
		t.Helper()
		row, err := e.Activities.GetActivity(colleague, ids.From[ids.ActivityKind](meeting), storekit.LiveOnly)
		if err != nil {
			return false
		}
		return row.ContentState == nil || *row.ContentState == crmcontracts.ActivityContentStateAvailable
	}
	if readsContent() {
		t.Fatal("a colleague reads a held meeting's content before the repair")
	}
	guest := cohortContact(t, e, "Guest Attendee", "guest@cohort.test")
	repairCohort(t, e, guest)
	if !meetingFiledUnder(t, e, meeting, guest) {
		t.Fatal("the repair did not file the meeting under its attendee")
	}
	// Filed under a contact, the meeting is no longer the link-less record the
	// capture hold was for, so the repair lifts it to the workspace.
	if audience := e.WsScalar(t, `SELECT audience FROM activity WHERE id = $1`, meeting); audience != "workspace" {
		t.Errorf("after the repair the audience is %s, want workspace", audience)
	}
	if !readsContent() {
		t.Error("a colleague cannot read the repaired workspace meeting's content")
	}
	if _, err := e.Activities.GetActivity(e.Admin(), ids.From[ids.ActivityKind](meeting), storekit.LiveOnly); err != nil {
		t.Fatalf("the workspace admin cannot read the repaired meeting: %v", err)
	}
}
