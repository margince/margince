// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

func TestLiveMeetingsUseTheirOwnDealEvidenceAndNeverRaiseTheMailQuestion(t *testing.T) {
	f := newDealRefileFixture(t, 0)
	e := f.o.e
	integration.ApplyRiverSchema(t)
	f.run(t, f.enqueue(t, "contact.created", f.contact))
	var thread string
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT thread_key FROM activity WHERE id=@activity`, pgx.NamedArgs{"activity": f.mail}).Scan(&thread)
	}); err != nil {
		t.Fatal(err)
	}
	if thread == "" {
		t.Fatal("mail fixture has no thread to challenge meeting attribution")
	}
	owner := seatAddress(t, e, e.AdminUser)
	actor, ok := principal.Actor(e.Admin())
	if !ok {
		t.Fatal("missing fixture actor")
	}
	actor.Type = principal.PrincipalConnector
	actor.ID = "connector:gcal"
	actor.OnBehalfOf = e.AdminUser
	actor.SeatType = principal.SeatFull
	ctx := principal.WithActor(e.Admin(), actor)
	sink := newCaptureSink(e.Pool, CaptureConfig{Now: func() time.Time { return f.o.now }})
	captureMeeting := func() ids.UUID {
		t.Helper()
		event := ids.NewV7().String()
		ref, err := sink.Upsert(ctx, connector.NormalizedRecord{EntityType: "activity", NaturalKey: connector.NaturalKey{SourceSystem: "gcal", SourceID: event}, Fields: capture.ActivityFields{Kind: "meeting", Subject: "Customer requirements", OccurredAt: f.o.now.Add(24 * time.Hour)}, ThreadKey: thread, Participants: []connector.MessageParticipant{{Email: owner, Role: "organizer"}, {Email: "pat@refile.example", Role: "attendee"}}, Addresses: []string{owner, "pat@refile.example"}, Source: "gcal:" + event, CapturedBy: "connector:gcal"}.WithProviderAttestedParticipants(true))
		if err != nil {
			t.Fatal(err)
		}
		return ref.ID
	}
	first := captureMeeting()
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND deal_id=$2`, first, f.deal) != 1 {
		t.Fatal("the real live-capture deal path did not file its unique candidate")
	}
	company := ids.From[ids.CompanyKind](f.company)
	if _, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{Name: "Another rollout", PipelineID: f.pipeline, StageID: f.stage, CompanyID: &company, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	questions := e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`)
	second := captureMeeting()
	if e.WsCount(t, `SELECT count(*) FROM activity_link WHERE activity_id=$1 AND entity_type='deal'`, second) != 0 {
		t.Fatal("meeting inherited the mail thread's deal instead of remaining ambiguous")
	}
	if e.WsCount(t, `SELECT count(*) FROM activity WHERE kind='task'`) != questions {
		t.Fatal("ambiguous meeting raised the mail-only question")
	}
}
