// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/meetingoutcome"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

// heldAsks records which lead and meeting the store asked to qualify after.
type heldAsks struct{ asked map[ids.UUID]ids.UUID }

func (*heldAsks) FileLeadBookingTx(context.Context, pgx.Tx, ids.LeadID, ids.UUID) error { return nil }

func (h *heldAsks) FileLeadMeetingHeldTx(_ context.Context, _ pgx.Tx, lead ids.LeadID, meeting ids.UUID) error {
	h.asked[lead.UUID] = meeting
	return nil
}

// Marking a meeting with a lead held asks to qualify the lead in the same
// write. Booking it asks nothing more.
func TestMarkingAMeetingHeldAsksItsLeadToBeQualified(t *testing.T) {
	e := setupLoad(t)
	lead := ids.NewV7()
	e.exec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
		VALUES ($1, 'Mira Prospect', 'engaged', 'manual', $2, $3)`, lead, "human:"+e.rep.String(), e.rep)
	asks := &heldAsks{asked: map[ids.UUID]ids.UUID{}}
	store := storeKnowing(e).WithLeadQualifyTasks(asks)
	actor, ok := principal.Actor(e.as())
	if !ok {
		t.Fatal("missing reader")
	}
	actor.Permissions.Objects["activity"] = principal.ObjectGrant{Read: true, Create: true, Update: true, Delete: true}
	writer := principal.WithActor(e.as(), actor)

	at, subject := requestInstant.Add(-time.Hour), "Demo"
	meeting, _, err := store.LogActivity(writer, LogActivityInput{
		Kind: "meeting", Subject: &subject, Source: "manual",
		OccurredAt: &at, Links: []ActivityLinkInput{{EntityType: "lead", EntityID: lead}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(asks.asked) != 0 {
		t.Fatalf("logging the meeting asked to qualify %d leads, want none until it is held", len(asks.asked))
	}
	held := meetingoutcome.Held
	if _, err := store.UpdateActivity(writer, ids.From[ids.ActivityKind](ids.UUID(meeting.Id)), UpdateActivityInput{MeetingStatus: &held}); err != nil {
		t.Fatal(err)
	}
	if asks.asked[lead] != ids.UUID(meeting.Id) {
		t.Errorf("marking the meeting held asked %v, want the lead asked after this meeting", asks.asked)
	}
	// A meeting logged already held asks too.
	logged, _, err := store.LogActivity(writer, LogActivityInput{
		Kind: "meeting", Subject: &subject, Source: "manual", OccurredAt: &at,
		MeetingStatus: &held, Links: []ActivityLinkInput{{EntityType: "lead", EntityID: lead}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if asks.asked[lead] != ids.UUID(logged.Id) {
		t.Errorf("logging a held meeting asked %v, want the lead asked after it", asks.asked)
	}
}

// The repair keeps the newest open qualify task on a lead and archives the
// older ones nobody worked.
func TestTheRepairKeepsOneOpenQualifyTaskPerLead(t *testing.T) {
	e := setupLoad(t)
	lead := ids.NewV7()
	e.exec(t, `INSERT INTO lead (id, full_name, status, source, captured_by, owner_id)
		VALUES ($1, 'Mira Prospect', 'engaged', 'manual', $2, $3)`, lead, "human:"+e.rep.String(), e.rep)
	source := provenance.LeadRepliedSource
	var tasks []ids.UUID
	for _, subject := range []string{"Review qualification", "Review qualification again", "Review qualification once more"} {
		task, _, err := storeKnowing(e).LogActivity(asClassifier(e), LogActivityInput{
			Kind: "task", Subject: &subject, Source: "system",
			SourceSystem: &source, Links: []ActivityLinkInput{{EntityType: "lead", EntityID: lead}},
		})
		if err != nil {
			t.Fatal(err)
		}
		tasks = append(tasks, ids.UUID(task.Id))
	}
	var scanned []ids.UUID
	var changed int
	if err := database.WithWorkspaceTx(asClassifier(e), e.pool, func(tx pgx.Tx) error {
		var err error
		scanned, changed, err = RetireDuplicateLeadTasksTx(asClassifier(e), tx, source, ids.Nil, 10, requestInstant)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// The data task rejects a page that changed more than it scanned.
	if len(scanned) != 1 || changed != 1 {
		t.Errorf("the repair scanned %d leads and changed %d, want the one lead counted once", len(scanned), changed)
	}
	for i, task := range tasks {
		var archived bool
		if err := e.owner.QueryRow(context.Background(), `SELECT archived_at IS NOT NULL FROM activity WHERE id = $1`, task).Scan(&archived); err != nil {
			t.Fatal(err)
		}
		if newest := i == len(tasks)-1; archived == newest {
			t.Errorf("task %d archived %v, want only the older ones archived", i, archived)
		}
	}
}
