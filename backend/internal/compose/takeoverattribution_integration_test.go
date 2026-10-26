// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A colleague takes over a promise nobody was asked to keep. A later reading
// of the same message names who made it, and keys the promise apart. The
// taker's task must stay the only one.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type takeoverReading struct {
	e        *integration.Env
	ctx      context.Context
	contact  ids.ContactID
	source   ids.UUID
	asOf     time.Time
	dispatch *CommitmentDispatcher
}

func newTakeoverReading(t *testing.T) takeoverReading {
	t.Helper()
	e := integration.Setup(t)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Priya Raman' WHERE id = $1`, e.Rep2)
	e.WsExec(t, `UPDATE app_user SET display_name = 'Robin Weber' WHERE id = $1`, e.Rep1)
	row, err := e.Contacts.CreateContact(e.Admin(), contacts.CreateContactInput{
		FullName: "Frau Adler", Source: "manual", Emails: []contacts.ContactEmailInput{{Email: "adler@kunde.test", EmailType: "work", IsPrimary: true}},
	})
	if err != nil {
		t.Fatal(err)
	}
	contact := ids.From[ids.ContactKind](ids.UUID(row.Id))
	asOf := clocktest.Now(t)
	sent := asOf.AddDate(0, 0, -1)
	body, direction := "I will send the signed contract by Friday. I will also send the price list.", "outbound"
	source, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Body: &body, Direction: &direction, OccurredAt: &sent, Source: "manual",
		CounterpartyEmail: "adler@kunde.test", CounterpartyOutboundAttested: true,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: contact.UUID}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return takeoverReading{
		e: e, contact: contact, source: ids.UUID(source.Id), asOf: asOf,
		ctx:      withTranscriptReader(principal.WithWorkspaceID(context.Background(), e.WS), "human:"+e.Rep1.String(), ids.NewV7()),
		dispatch: NewCommitmentDispatcher(e.Pool, approvals.NewService(e.DB())),
	}
}

// reading is one commitment the signal scan reads out of the message. A nil
// seat is a promiser the reading could not place.
func (r takeoverReading) reading(sentence string, seat *ids.UUID) Commitment {
	party := unnamedPromiserKey("")
	c := Commitment{
		Meaning: openCommitmentMeaning("", sentence), AsOf: r.asOf, ReadMessages: []ids.UUID{r.source}, Extractor: signalScanActor,
		SourceActivityID: r.source, Summary: sentence, Quote: sentence, Span: sentence, PromisedTo: &r.contact, Confidence: 0.95,
	}
	if seat != nil {
		party = "seat:" + seat.String()
		c.Seat, c.Party = seat, "Robin Weber"
		c.parties = []commitmentParty{{Ref: party, Role: commitmentSenderRole, Name: "Robin Weber", Seat: seat}}
	}
	c.Locator = commitmentLocator(r.source, claimKindOurs, party, sentence)
	return c
}

func (r takeoverReading) dispatchOnce(t *testing.T, c Commitment) CommitmentOutcome {
	t.Helper()
	var outcome CommitmentOutcome
	if err := database.WithWorkspaceTx(r.ctx, r.e.Pool, func(tx pgx.Tx) error {
		var err error
		outcome, _, err = r.dispatch.DispatchTx(r.ctx, tx, c, ids.NewV7())
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return outcome
}

func (r takeoverReading) tasks(t *testing.T) string {
	t.Helper()
	return r.e.WsScalar(t, `SELECT coalesce(string_agg(assignee_id::text, ',' ORDER BY created_at), '') FROM activity
		WHERE kind = 'task' AND source_system = $1`, activities.CommitmentTaskSource)
}

func TestATakenOverPromiseKeepsOneTaskWhenALaterReadingNamesItsPromiser(t *testing.T) {
	r := newTakeoverReading(t)
	const contract = "I will send the signed contract by Friday."
	if got := r.dispatchOnce(t, r.reading(contract, nil)); got != CommitmentPromiserUnassignable {
		t.Fatalf("the unplaced promise = %q, want it kept with no promiser", got)
	}
	claim := ids.MustParse(r.e.WsScalar(t, `SELECT id::text FROM conversation_claim
		WHERE contact_id = $1 AND source_activity_id = $2 AND kind = 'commitment_ours'`, r.contact.UUID, r.source))
	// A member corrected the claim, which keeps a promise nobody may be asked
	// about current, so it can be taken over.
	r.e.WsExec(t, `UPDATE conversation_claim SET corrected_at = now(), corrected_by_user_id = $2 WHERE id = $1`, claim, r.e.Rep2)
	priya := r.e.As(r.e.Rep2, []ids.UUID{r.e.Team1}, integration.AdminPerms)
	if _, err := newPromiseTakeOverHandlers(r.e.Pool).TakeOver(priya, claim); err != nil {
		t.Fatalf("taking the promise over: %v", err)
	}

	// The later reading names Robin and quotes only part of the sentence, and
	// a replay reads it again.
	named := r.reading(contract, &r.e.Rep1)
	named.Quote = "send the signed contract"
	for range 2 {
		r.dispatchOnce(t, named)
		if got := r.tasks(t); got != r.e.Rep2.String() {
			t.Fatalf("tasks held by %q, want only Priya's", got)
		}
	}
	if n := r.e.WsCount(t, `SELECT count(*) FROM conversation_claim WHERE contact_id = $1 AND kind = 'commitment_ours'`, r.contact.UUID); n != 1 {
		t.Errorf("the contact carries %d claims for the one promise, want 1", n)
	}
	if n := r.e.WsCount(t, `SELECT count(*) FROM approval WHERE kind = $1`, CommitmentTaskKind); n != 0 {
		t.Errorf("%d proposals staged beside the taken task, want none", n)
	}

	// Another sentence of the same message is different work.
	r.dispatchOnce(t, r.reading("I will also send the price list.", &r.e.Rep1))
	if got := r.tasks(t); got != r.e.Rep2.String()+","+r.e.Rep1.String() {
		t.Errorf("tasks held by %q, want Priya's and a new one for Robin", got)
	}
}
