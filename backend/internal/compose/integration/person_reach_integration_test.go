// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A contact's page shows the messages they were IN, not only the ones filed
// under them.
//
// Capture files a message under its counterparty — one address, one link. So a
// thread a contact was CC'd on is filed under whoever wrote it, and a meeting is
// filed under whoever the attendee resolution reached first. Reading the person
// page through the link table alone therefore answered a narrower question than
// the page asks: it showed what was FILED here rather than what this human was
// part of, and a contact copied on the whole negotiation had an empty timeline.
//
// The second half of this is what the page may then CLAIM. Links describe how a
// message is filed; a participant row does not make one, so the payload must not
// report a link that activity_link does not hold — a client acting on it, to
// unfile the message from this contact, would act on nothing.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/person360"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestThePersonPageShowsAThreadTheContactWasOnlyCopiedOn(t *testing.T) {
	e := Setup(t)
	author := e.As(e.Rep1, []ids.UUID{e.Team1}, activityLifecyclePerms)
	filedUnder := e.SeedPerson(t, "Primary Counterparty", &e.Rep1)
	copied := e.SeedPerson(t, "Copied Colleague", &e.Rep1)

	subject, body := "Renewal terms", "the numbers we discussed"
	logged, _, err := e.Activities.LogActivity(author, activities.LogActivityInput{
		Kind: "email", Subject: &subject, Body: &body, Direction: strPtr("inbound"),
		// Filed under the counterparty alone, which is what capture does.
		Links: []activities.ActivityLinkInput{{EntityType: "person", EntityID: filedUnder}},
	})
	if err != nil {
		t.Fatalf("log: %v", err)
	}
	// The copied contact is on the message as a participant and nowhere else —
	// the shape capture leaves for a Cc line and for a meeting attendee.
	e.WsExec(t, `INSERT INTO activity_participant (id, activity_id, person_id, role)
	             VALUES ($1, $2, $3, 'cc')`, ids.NewV7(), ids.UUID(logged.Id), copied)

	svc := person360.NewService(e.Pool, e.People, e.Deals, e.Projects, consent.NewStore(e.DB()),
		comms.NewStore(e.DB(), time.Now, activities.NewStore(e.DB())), ai.NewFeedbackStore(e.DB()),
		func() time.Time { return roomFixedNow })

	page, err := svc.Assemble(author, ids.From[ids.PersonKind](copied))
	if err != nil {
		t.Fatalf("assembling the copied contact's page: %v", err)
	}
	row := onlyTimelineRow(t, page)
	if ids.UUID(row.Id) != ids.UUID(logged.Id) {
		t.Fatalf("timeline row = %s, want the thread %s they were copied on", row.Id, logged.Id)
	}
	// And it does NOT claim a filing that does not exist.
	if row.Links != nil && len(*row.Links) > 0 {
		t.Errorf("the page reports links %+v for a message filed under somebody else — "+
			"a participant row is not a link, and a client acting on that claim would act on nothing", *row.Links)
	}

	// The contact the message IS filed under still gets the link reported, so
	// the honesty above did not cost the ordinary case.
	filedPage, err := svc.Assemble(author, ids.From[ids.PersonKind](filedUnder))
	if err != nil {
		t.Fatalf("assembling the filed-under contact's page: %v", err)
	}
	filedRow := onlyTimelineRow(t, filedPage)
	if filedRow.Links == nil || len(*filedRow.Links) != 1 {
		t.Fatalf("a message filed under this contact reports links %v, want the one link it actually has", filedRow.Links)
	}
	if ids.UUID((*filedRow.Links)[0].EntityId) != filedUnder {
		t.Errorf("reported link names %s, want the contact it is filed under %s",
			(*filedRow.Links)[0].EntityId, filedUnder)
	}
}
