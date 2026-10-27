// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// An interaction that has not happened yet is nobody's last touch. That holds
// on the graph edges, and on the contact page, which reads by its own clock.

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/contact360"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/comms"
	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestAMeetingAheadIsNoGraphEdgesLastInteraction(t *testing.T) {
	e := integration.Setup(t)
	now := time.Now().UTC().Truncate(time.Second)
	dana := contactWithEmail(t, e, "Dana Ahead", "dana@ahead.example")
	eli := contactWithEmail(t, e, "Eli Ahead", "eli@ahead.example")
	parties := []connector.MessageParticipant{
		{Email: "dana@ahead.example", Role: connector.ParticipantRoleAttendee},
		{Email: "eli@ahead.example", Role: connector.ParticipantRoleAttendee},
	}
	held := integration.CalendarMeeting{Event: "evt-ahead-held", At: now.AddDate(0, 0, -10), Links: contactLink(dana), Parties: parties}
	ahead := integration.CalendarMeeting{Event: "evt-ahead-booked", At: now.AddDate(0, 0, 5), Links: contactLink(dana), Parties: parties}
	touched := []ids.UUID{held.Capture(t, e), ahead.Capture(t, e)}

	assertLastAtHeld := func(path string) {
		t.Helper()
		if got := e.WsCount(t, `SELECT count(*) FROM graph_interaction_edge
		                         WHERE user_id = $1 AND contact_id = $2 AND last_at = $3`,
			e.AdminUser, dana, held.At); got != 1 {
			t.Errorf("%s: the colleague edge's last_at is not the held meeting; one five days ahead is no interaction yet", path)
		}
		if got := e.WsCount(t, `SELECT count(*) FROM graph_contact_edge
		                         WHERE contact_a = least($1::uuid, $2::uuid) AND contact_b = greatest($1::uuid, $2::uuid)
		                           AND last_at = $3`,
			dana, eli, held.At); got != 1 {
			t.Errorf("%s: the contact edge's last_at is not the held meeting; one five days ahead is no interaction yet", path)
		}
	}
	inWorkspaceTx(t, e, func(tx pgx.Tx) error { return search.RecomputeEdgesForActivities(e.Admin(), tx, touched) })
	assertLastAtHeld("incremental fold")
	inWorkspaceTx(t, e, func(tx pgx.Tx) error { return search.RebuildEdges(e.Admin(), tx) })
	assertLastAtHeld("rebuild")
}

func TestTheContactPageReadsLastTouchByItsOwnClock(t *testing.T) {
	e := integration.Setup(t)
	contact := seedLinkedContact(t, e, "uhr@kanzlei.example")
	pageNow := time.Now().Add(-72 * time.Hour).Truncate(time.Microsecond)
	before := pageNow.Add(-24 * time.Hour)
	seedLinkedActivityAt(t, e, contact, "workspace", before)
	// Past for the database, ahead of the page's clock.
	seedLinkedActivityAt(t, e, contact, "workspace", pageNow.Add(24*time.Hour))

	svc := contact360.NewService(e.Pool, e.Contacts, e.Deals, e.Projects,
		consent.NewStore(InstallationDB(e.Pool)),
		comms.NewStore(InstallationDB(e.Pool), time.Now, activities.NewStore(InstallationDB(e.Pool))),
		ai.NewFeedbackStore(InstallationDB(e.Pool)), func() time.Time { return pageNow })
	page, err := svc.Assemble(e.Admin(), ids.From[ids.ContactKind](contact))
	if err != nil {
		t.Fatalf("assembling contact360: %v", err)
	}
	if page.LastInboundAt == nil || !page.LastInboundAt.Equal(before) {
		t.Errorf("last-inbound = %v at a page clock of %v, want %v: the message after the page's clock "+
			"has not happened for this page", page.LastInboundAt, pageNow, before)
	}
}
