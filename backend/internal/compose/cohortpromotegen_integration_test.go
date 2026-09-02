// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The cg:cohort-promote consumer, driven the way the bus drives it.
//
// The repair's own SQL is covered in the people module. What is covered HERE is
// the thing that only breaks on the consumer path: the CONTEXT a subscriber
// hands the store. A subscriber carries no correlation id, and the repair's
// last act is to publish an activity.updated for each message that moved — so a
// pass that assembles the context wrong writes its links, is refused at the
// publish, rolls back, and is redelivered forever. Every assertion below runs
// through HandleEvent for that reason; calling PromotePersonCohortTx directly
// would pass with the context bug in place, which is exactly how it shipped.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// seedStrandedMeeting writes the ordering this consumer exists for: a captured
// meeting whose attendee is an ADDRESS, and the person for that address created
// afterwards. Nothing links the two, which is the state a real calendar sync
// leaves behind whenever the invitation arrives before the contact does.
//
// Written as capture writes it — connector captured_by, an address-only
// participant row, no activity_link — because a fixture that links the meeting
// itself would prove nothing about the repair.
func seedStrandedMeeting(t *testing.T, e *integration.Env, address string) (person, meeting ids.UUID) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			INSERT INTO activity (kind, subject, occurred_at, source, captured_by)
			VALUES ('meeting', 'Quarterly review', now() + interval '2 days',
			        'connector', 'connector:gcal')
			RETURNING id`).Scan(&meeting); err != nil {
			return err
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO activity_participant (activity_id, address, role)
			VALUES ($1, $2, 'to')`, meeting, address); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, `
			INSERT INTO person (full_name, owner_id, source, captured_by, visibility)
			VALUES ('Stranded Attendee', $1, 'manual', 'human:test', 'workspace')
			RETURNING id`, e.Rep1).Scan(&person); err != nil {
			return err
		}
		_, err := tx.Exec(ctx, `
			INSERT INTO person_email (person_id, email, email_type, is_primary, position, source, captured_by)
			VALUES ($1, $2, 'work', true, 0, 'manual', 'human:test')`, person, address)
		return err
	}); err != nil {
		t.Fatalf("seeding a stranded meeting: %v", err)
	}
	return person, meeting
}

// meetingIsFiledUnder reports the two halves of a repaired cohort separately:
// the participant row now naming the person, and the meeting filed under them.
// Separately because they fail for different reasons — the promotion is an
// address match, the link is attendance — and one number could not say which.
func meetingIsFiledUnder(t *testing.T, e *integration.Env, person, meeting ids.UUID) (named, linked bool) {
	t.Helper()
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		ctx := context.Background()
		if err := tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM activity_participant
			                WHERE activity_id = $1 AND person_id = $2)`,
			meeting, person).Scan(&named); err != nil {
			return err
		}
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM activity_link
			                WHERE activity_id = $1 AND person_id = $2)`,
			meeting, person).Scan(&linked)
	}); err != nil {
		t.Fatalf("reading the repaired cohort: %v", err)
	}
	return named, linked
}

func cohortEnvelope(eventType string, person ids.UUID) kevents.Envelope {
	return kevents.Envelope{
		EventID:    ids.NewV7(),
		Type:       eventType,
		Version:    1,
		OccurredAt: time.Now().UTC(),
		Entity:     kevents.EntityRef{Type: "person", ID: person},
		Trace:      kevents.Trace{CorrelationID: ids.NewV7()},
	}
}

func cohortGen(e *integration.Env) *CohortPromoteGen {
	return NewCohortPromoteGen(e.Pool, people.NewStore(e.DB()), slog.Default())
}

// The regression this file exists for.
//
// Reverting the WithCorrelationID line in HandleEvent fails this at the
// HandleEvent call, not at the assertions: the repair publishes, the publish is
// refused for want of a correlation id, and the whole transaction rolls back.
// That is the shape the defect had in production — links written, then undone,
// forever — so the test reproduces the mechanism rather than only the symptom.
func TestAPersonEventFilesTheMeetingTheirAddressAttended(t *testing.T) {
	e := integration.Setup(t)
	person, meeting := seedStrandedMeeting(t, e, "attendee@example.test")

	if named, linked := meetingIsFiledUnder(t, e, person, meeting); named || linked {
		t.Fatal("the meeting was already filed before any event was delivered — the fixture links it itself and proves nothing")
	}

	if err := cohortGen(e).HandleEvent(context.Background(), cohortEnvelope("person.created", person)); err != nil {
		t.Fatalf("person.created reached the repair and failed: %v", err)
	}

	named, linked := meetingIsFiledUnder(t, e, person, meeting)
	if !named {
		t.Error("the attendee row still names only an address, so nothing downstream can tell it is this person")
	}
	if !linked {
		t.Error("the meeting is not filed under the person who attended it, so their record shows no meeting and offers no brief")
	}
}

// Redelivery is free. The bus is at-least-once and the repair runs on somebody
// else's retry as readily as its own, so a second pass must add nothing rather
// than a second link.
func TestARedeliveredPersonEventRepairsTheCohortOnce(t *testing.T) {
	e := integration.Setup(t)
	person, meeting := seedStrandedMeeting(t, e, "repeat@example.test")

	gen := cohortGen(e)
	ctx := context.Background()
	for i := range 3 {
		if err := gen.HandleEvent(ctx, cohortEnvelope("person.updated", person)); err != nil {
			t.Fatalf("delivery %d: %v", i, err)
		}
	}

	var links int
	if err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		return tx.QueryRow(context.Background(),
			`SELECT count(*) FROM activity_link WHERE activity_id = $1 AND person_id = $2`,
			meeting, person).Scan(&links)
	}); err != nil {
		t.Fatalf("counting links: %v", err)
	}
	if links != 1 {
		t.Errorf("three deliveries left %d links, want 1", links)
	}
}

// An event this consumer does not act on must answer nil. The group carries the
// whole person stream, so erroring on somebody else's traffic would wedge the
// repair for every contact behind it — which is the failure this consumer spent
// its first months in.
func TestAnUnrelatedEventLeavesTheCohortAlone(t *testing.T) {
	e := integration.Setup(t)
	person, meeting := seedStrandedMeeting(t, e, "untouched@example.test")

	gen := cohortGen(e)
	ctx := context.Background()
	for _, unrelated := range []kevents.Envelope{
		cohortEnvelope("person.archived", person),
		{
			EventID: ids.NewV7(), Type: "deal.created", Version: 1, OccurredAt: time.Now().UTC(),
			Entity: kevents.EntityRef{Type: "deal", ID: ids.NewV7()},
			Trace:  kevents.Trace{CorrelationID: ids.NewV7()},
		},
	} {
		if err := gen.HandleEvent(ctx, unrelated); err != nil {
			t.Errorf("%s errored and would wedge the stream: %v", unrelated.Type, err)
		}
	}

	if _, linked := meetingIsFiledUnder(t, e, person, meeting); linked {
		t.Error("an event that gains the person no address filed their meeting anyway")
	}
}
