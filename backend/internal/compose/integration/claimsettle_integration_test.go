// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration_test

// Settling a promise, against real Postgres.
//
// The status column has carried open/done/dismissed since the table existed and
// every reader filters on `open`. Nothing wrote the other two: a promise could
// be extracted from a conversation and shown to the rep who made it every
// morning, with no way to say they had kept it. The sibling test in
// commitmentlane_integration_test.go says so in its own words — it sets the
// column directly because "these are the extractor's own lifecycle columns and
// have no writer on this path".

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestASettledPromiseLeavesTheLaneItWasOn(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Vogt", &e.Rep1)
	kept := seedPromise(t, e, person, "Angebot nachreichen", &due)
	seedPromise(t, e, person, "Bleibt offen", &due)

	store := people.NewStore(e.DB())
	if err := store.SettleConversationClaim(e.Admin(), kept, "done"); err != nil {
		t.Fatalf("settling the promise: %v", err)
	}

	// Read through the SAME lane the row came from, so this proves the write
	// reaches the reader rather than that one column changed.
	rows, err := store.OpenCommitmentsDue(e.Admin(),
		ids.From[ids.UserKind](e.Rep1), laneClock.Add(24*time.Hour), 20)
	if err != nil {
		t.Fatalf("reading the lane: %v", err)
	}
	if got := bodiesOf(rows); len(got) != 1 || got[0] != "Bleibt offen" {
		t.Fatalf("the lane = %v, want only the promise nobody settled", got)
	}
}

func TestSettlingTwiceTheSameWayIsOneSettlement(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Frau Adler", &e.Rep1)
	claim := seedPromise(t, e, person, "Termin bestätigen", &due)

	store := people.NewStore(e.DB())
	before := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, person)
	for range 2 {
		if err := store.SettleConversationClaim(e.Admin(), claim, "done"); err != nil {
			t.Fatalf("settling: %v", err)
		}
	}
	// The second settle writes NOTHING. A second audit row would say a person's
	// record changed when it did not, and the trail is read to answer "what
	// happened to this person" — an entry for a call that changed nothing is a
	// false answer to that.
	after := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1`, person)
	if after-before != 1 {
		t.Fatalf("settling twice wrote %d audit rows, want 1", after-before)
	}
}

func TestSettlingAPromiseTheOtherWayIsRefused(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Herr Baum", &e.Rep1)
	claim := seedPromise(t, e, person, "Preisliste schicken", &due)

	store := people.NewStore(e.DB())
	if err := store.SettleConversationClaim(e.Admin(), claim, "done"); err != nil {
		t.Fatalf("settling as done: %v", err)
	}
	// A promise recorded as KEPT is not re-decidable as never-real without
	// somebody saying which is true. Silently overwriting would let the second
	// caller erase the first one's answer with no trace of the disagreement.
	err := store.SettleConversationClaim(e.Admin(), claim, "dismissed")
	if !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("re-settling the other way returned %v, want a conflict", err)
	}
}

func TestSettlingWritesTheWholeShape(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Frau Kern", &e.Rep1)
	claim := seedPromise(t, e, person, "Vertrag prüfen", &due)

	store := people.NewStore(e.DB())
	outbox := e.WsCount(t, `SELECT count(*) FROM event_outbox`)
	if err := store.SettleConversationClaim(e.Admin(), claim, "done"); err != nil {
		t.Fatalf("settling: %v", err)
	}

	// Domain row, audit row and event, all three. A settlement that moved the
	// column and emitted nothing would leave a downstream reader holding a
	// promise this workspace considers finished.
	settled := e.WsCount(t,
		`SELECT count(*) FROM conversation_claim WHERE id = $1 AND status = 'done'`, claim)
	if settled != 1 {
		t.Error("the claim did not reach status done")
	}
	audits := e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_id = $1 AND action = 'update'`, person)
	if audits != 1 {
		t.Errorf("the settlement wrote %d update audit rows, want 1", audits)
	}
	if after := e.WsCount(t, `SELECT count(*) FROM event_outbox`); after != outbox+1 {
		t.Errorf("the settlement staged %d events, want 1", after-outbox)
	}
}

func TestAReaderWhoMayNotWriteThePersonCannotSettleTheirPromise(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	// Rep2's person, settled by Rep3. A person outside the caller's scope reads
	// as absent, which is what keeps the claim's existence from leaking to
	// somebody who may not know the person exists.
	person := e.SeedPerson(t, "Herr Fremd", &e.Rep2)
	claim := seedPromise(t, e, person, "Nicht deine Zusage", &due)

	store := people.NewStore(e.DB())
	// RepPerms, not AdminPerms. An admin legitimately may write any person in
	// the workspace, so an admin fixture here would test nothing: the refusal
	// this asserts is row scope, and admin has none to fail. Rep3 sits in Team2
	// while the person is Rep2's, so a team-scoped reader cannot reach them.
	stranger := e.As(e.Rep3, []ids.UUID{e.Team2}, integration.RepPerms)
	err := store.SettleConversationClaim(stranger, claim, "done")
	if err == nil {
		t.Fatal("a reader settled a promise on a person they may not write")
	}
	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the refusal was %v, want not-found or permission-denied", err)
	}
}

// A claim quotes words, and the evidence can be narrowed or archived after the
// claim was written. Settling reads the same gate the recording path holds:
// somebody who may no longer read the conversation must not go on settling
// promises read out of it, and the 204-vs-409 answer would report the claim's
// current status to them either way.
func TestAPromiseWhoseEvidenceIsGoneCannotBeSettled(t *testing.T) {
	e := integration.Setup(t)
	due := laneClock.Add(2 * time.Hour)
	person := e.SeedPerson(t, "Frau Winter", &e.Rep1)
	claim := seedPromise(t, e, person, "Muster zusenden", &due)

	// The message the promise was read from, archived after the fact.
	e.WsExec(t, `
		UPDATE activity SET archived_at = now()
		WHERE id = (SELECT source_activity_id FROM conversation_claim WHERE id = $1)`, claim)

	err := people.NewStore(e.DB()).SettleConversationClaim(e.Admin(), claim, "done")
	if err == nil {
		t.Fatal("a promise was settled from evidence that is no longer readable")
	}
	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("the refusal was %v, want not-found or permission-denied", err)
	}
}
