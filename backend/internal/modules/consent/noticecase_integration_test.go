// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package consent

// The notice case against a real database: the duty is recorded once per
// acquisition, and the queue orders by the clock.

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedAcquisition writes one acquisition row through SQL and returns its id.
// The people module owns the writer, and consent may not import a sibling — so
// the fixture states the row the writer produces rather than reaching for it.
func seedAcquisition(t *testing.T, e *channelConsentEnv, kind string) ids.UUID {
	t.Helper()
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		INSERT INTO person_acquisition_evidence (person_id, kind, captured_by)
		VALUES ($1, $2, 'test') RETURNING id`, e.person, kind).Scan(&id); err != nil {
		t.Fatalf("seeding the acquisition: %v", err)
	}
	return id
}

// TestADutyIsRecordedOncePerAcquisition holds the idempotence the at-least-once
// consumer depends on.
//
// A redelivered person.created reaches the writer a second time. Were the
// duplicate accepted, one acquisition would owe two duties and discharging one
// would leave the other standing forever on the queue.
func TestADutyIsRecordedOncePerAcquisition(t *testing.T) {
	e := setupChannelConsent(t)
	acq := seedAcquisition(t, e, "purchased_or_imported")
	due := time.Now().Add(30 * 24 * time.Hour)

	for i := range 2 {
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
				PersonID: e.person, AcquisitionID: acq, Rule: RuleArt14, DueAt: due,
			})
		}); err != nil {
			t.Fatalf("recording the duty (attempt %d): %v", i+1, err)
		}
	}

	var cases int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM privacy_notice_case WHERE acquisition_id = $1`, acq).Scan(&cases); err != nil {
		t.Fatal(err)
	}
	if cases != 1 {
		t.Errorf("one acquisition owes %d notice cases, want 1: a redelivered person.created "+
			"must not mint a second duty for the same acquisition", cases)
	}
}

// TestTwoAcquisitionsOweTwoDuties is the other half, and the reason the case is
// keyed on the acquisition rather than the person.
//
// A contact acquired twice — a form today, an import next month — incurs the
// duty twice, and the second is a fresh disclosure with its own deadline. Keyed
// on the person, the second would collapse into the first and read as
// discharged by it.
func TestTwoAcquisitionsOweTwoDuties(t *testing.T) {
	e := setupChannelConsent(t)
	first := seedAcquisition(t, e, "event_or_form")
	second := seedAcquisition(t, e, "purchased_or_imported")
	due := time.Now().Add(30 * 24 * time.Hour)

	for _, acq := range []ids.UUID{first, second} {
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
				PersonID: e.person, AcquisitionID: acq, Rule: RuleArt14, DueAt: due,
			})
		}); err != nil {
			t.Fatalf("recording the duty: %v", err)
		}
	}

	var cases int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM privacy_notice_case WHERE person_id = $1`, e.person).Scan(&cases); err != nil {
		t.Fatal(err)
	}
	if cases != 2 {
		t.Errorf("two acquisitions owe %d duties, want 2: the second acquisition is a fresh "+
			"disclosure with its own deadline, not one already discharged", cases)
	}
}

// TestTheQueueOrdersByTheClock — soonest deadline first, and a completed case
// is off it entirely.
func TestTheQueueOrdersByTheClock(t *testing.T) {
	e := setupChannelConsent(t)
	now := time.Now()
	late := seedAcquisition(t, e, "purchased_or_imported")
	soon := seedAcquisition(t, e, "referral")
	done := seedAcquisition(t, e, "event_or_form")
	completedAt := now

	write := func(acq ids.UUID, due time.Time, state NoticeState, at *time.Time) {
		t.Helper()
		if err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
			return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
				PersonID: e.person, AcquisitionID: acq, Rule: RuleArt14,
				DueAt: due, State: state, CompletedAt: at,
			})
		}); err != nil {
			t.Fatalf("recording the duty: %v", err)
		}
	}
	write(late, now.Add(48*time.Hour), NoticeOpen, nil)
	write(soon, now.Add(1*time.Hour), NoticeOpen, nil)
	write(done, now.Add(-99*time.Hour), NoticeCompleted, &completedAt)

	// The queue is gated on privacy_request, the object the subject-request
	// queue moved onto: reading it says how named people were obtained and
	// whether we have told them. The shared fixture's actor holds people
	// grants, not the privacy inbox, so the grant is added HERE rather than
	// widened for every test that shares the fixture.
	got, err := e.store.OpenNoticeCasesDueSoonest(privacyOperator(e), 10)
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("the queue holds %d cases, want 2: a completed duty is discharged and off it", len(got))
	}
	if !got[0].DueAt.Before(got[1].DueAt) {
		t.Errorf("the queue is not ordered by the clock: %v then %v", got[0].DueAt, got[1].DueAt)
	}
}

// TestATerminalCaseSaysWhenItBecameOne — a completed case carrying no timestamp
// cannot be told from one somebody closed and forgot to date.
func TestATerminalCaseSaysWhenItBecameOne(t *testing.T) {
	e := setupChannelConsent(t)
	acq := seedAcquisition(t, e, "referral")

	err := e.store.db.Tx(e.ctx, func(tx pgx.Tx) error {
		return OpenNoticeCaseTx(e.ctx, tx, NoticeCaseInput{
			PersonID: e.person, AcquisitionID: acq, Rule: RuleArt14,
			DueAt: time.Now(), State: NoticeCompleted,
		})
	})
	var invalid *ValidationError
	if !errors.As(err, &invalid) {
		t.Fatalf("a completed case with no timestamp returned %v, want a validation error", err)
	}
}

// privacyOperator returns the fixture's context with the privacy-inbox grant
// the notice queue is gated on, and nothing else added.
func privacyOperator(e *channelConsentEnv) context.Context {
	actor, _ := principal.Actor(e.ctx)
	objects := map[string]principal.ObjectGrant{}
	for k, v := range actor.Permissions.Objects {
		objects[k] = v
	}
	objects["privacy_request"] = principal.ObjectGrant{Read: true, Update: true}
	actor.Permissions.Objects = objects
	return principal.WithActor(e.ctx, actor)
}
