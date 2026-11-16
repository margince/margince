// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capturerepairs

import (
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadMetAt files the meeting on a new lead the admin owns.
func leadMetAt(t *testing.T, e *integration.Env, email string, meeting ids.UUID) ids.UUID {
	t.Helper()
	lead, _, err := e.Contacts.CreateLead(e.Admin(), contacts.CreateLeadInput{FullName: new("Lena Prospect"), Email: &email, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `UPDATE lead SET owner_id = $1 WHERE id = $2`, e.AdminUser, lead.Id)
	e.WsExec(t, `INSERT INTO activity_link (activity_id, entity_type, lead_id) VALUES ($1, 'lead', $2)`, meeting, lead.Id)
	return ids.UUID(lead.Id)
}

// The release's notes repair, run through the catalog the worker runs, marks a
// booked lead meeting held and asks the lead's owner to qualify the lead. Notes
// on a meeting already canceled keep that outcome and ask nobody.
func TestTheNotesRepairMarkingALeadMeetingHeldAsksTheOwnerToQualify(t *testing.T) {
	e := integration.Setup(t)
	booked, _ := storedAutomaticNotes(t, e, "booked")
	canceled, _ := storedAutomaticNotes(t, e, "canceled")
	met := leadMetAt(t, e, "lena@prospect.example", booked)
	missed := leadMetAt(t, e, "milo@prospect.example", canceled)

	if _, err := compose.DataTaskExecute(e.Admin(), e.Pool, 1000); err != nil {
		t.Fatalf("running the release's data tasks: %v", err)
	}

	if got := e.WsScalar(t, `SELECT meeting_status FROM activity WHERE id=$1`, booked); got != "held" {
		t.Fatalf("the repair left the booked meeting %q, want held", got)
	}
	integration.AssertOwnerAskedToQualify(e.Admin(), t, e.DB(), met, booked)
	if tasks := integration.OpenQualifyTasks(e.Admin(), t, e.DB(), missed); len(tasks) != 0 {
		t.Errorf("notes on a canceled meeting asked to qualify its lead: %+v", tasks)
	}
}
