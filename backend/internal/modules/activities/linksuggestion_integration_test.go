// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Suggested links, written and read through the real store on a real database.

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type suggestionFixture struct {
	e        *loadEnv
	store    *Store
	ctx      context.Context
	message  ids.UUID
	deal     ids.UUID
	author   ids.UUID
	stranger ids.UUID
}

// newSuggestionFixture seeds one inbound message sent by a known contact, a deal
// to suggest and a second contact who is not the author.
func newSuggestionFixture(t *testing.T) suggestionFixture {
	t.Helper()
	e := setupLoad(t)
	f := suggestionFixture{
		e: e, store: NewStore(database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws))),
		ctx: e.asSeat(e.rep), deal: ids.NewV7(), author: ids.NewV7(), stranger: ids.NewV7(),
	}
	for _, c := range []ids.UUID{f.author, f.stranger} {
		e.exec(t, `INSERT INTO contact (id, full_name, owner_id, source, captured_by)
			VALUES ($1, 'Buyer Contact', $2, 'seed', 'system')`, c, e.rep)
	}
	f.message = e.seedWait(t, "Which project is this?", "contact_id", f.author)
	e.exec(t, `UPDATE activity_participant SET contact_id = $2 WHERE activity_id = $1 AND role = 'from'`, f.message, f.author)
	pipeline, stage := ids.NewV7(), ids.NewV7()
	e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, 'Renewal', 'open', $2, $3, $4, 'seed', 'system')`, f.deal, e.rep, pipeline, stage)
	return f
}

// ownRowsSeat may update activities but reads only the deals it owns.
func ownRowsSeat(e *loadEnv, user ids.UUID) context.Context {
	ctx := e.asSeat(user)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.RowScope = principal.RowScopeOwn
	return principal.WithActor(ctx, actor)
}

// readOnlySeat reads activities and may not change them.
func readOnlySeat(e *loadEnv, user ids.UUID) context.Context {
	ctx := e.asSeat(user)
	actor, _ := principal.Actor(ctx)
	actor.Permissions.Objects = map[string]principal.ObjectGrant{
		"activity": {Read: true}, "contact": {Read: true}, "deal": {Read: true}, "company": {Read: true},
	}
	return principal.WithActor(ctx, actor)
}

func (f suggestionFixture) propose(t *testing.T, entityType string, entity ids.UUID) LinkSuggestionOutcome {
	t.Helper()
	var outcome LinkSuggestionOutcome
	err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		var err error
		outcome, err = ProposeLinkSuggestionTx(f.ctx, tx, LinkSuggestionInput{
			ActivityID: ids.From[ids.ActivityKind](f.message), EntityType: entityType, EntityID: entity,
			Unit: "finance-demo", Reason: "The subject names this deal.", Score: 0.8,
		})
		return err
	})
	if err != nil {
		t.Fatalf("proposing %s: %v", entityType, err)
	}
	return outcome
}

// queue is the pending suggestions for this fixture's message. The clone is
// shared by the package's tests, so the whole queue holds other tests' rows.
func (f suggestionFixture) queue() ([]crmcontracts.LinkSuggestion, error) {
	all, err := f.store.ListLinkSuggestions(f.ctx)
	return slices.DeleteFunc(all, func(s crmcontracts.LinkSuggestion) bool {
		return ids.UUID(s.ActivityId) != f.message
	}), err
}

func (f suggestionFixture) count(t *testing.T, sql string) int {
	t.Helper()
	var n int
	if err := f.e.owner.QueryRow(context.Background(), sql, f.message).Scan(&n); err != nil {
		t.Fatalf("counting: %v", err)
	}
	return n
}

func TestAProposalWaitsInTheQueueAndAttachesNothing(t *testing.T) {
	f := newSuggestionFixture(t)
	linksBefore := f.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`)

	if got := f.propose(t, "deal", f.deal); got != SuggestionCreated {
		t.Fatalf("first proposal answered %q, want created", got)
	}

	if after := f.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`); after != linksBefore {
		t.Errorf("a proposal changed the message's links from %d to %d; only an accept may attach", linksBefore, after)
	}
	queue, err := f.queue()
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	if len(queue) != 1 || queue[0].Status != "pending" || queue[0].EntityName == nil || *queue[0].EntityName != "Renewal" {
		t.Fatalf("queue = %+v, want one pending suggestion naming the deal", queue)
	}
	if audited := f.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after ? 'link_suggestion'`); audited != 1 {
		t.Errorf("a proposal left %d audit rows, want 1", audited)
	}
	if announced := f.count(t, `SELECT count(*) FROM event_outbox
		WHERE envelope->>'type' = 'activity.link_suggestion_changed' AND envelope->'entity'->>'id' = $1::text`); announced != 1 {
		t.Errorf("a proposal left %d events, want 1", announced)
	}
}

func TestARepeatProposalWritesNothingAndARejectedOneStaysRejected(t *testing.T) {
	f := newSuggestionFixture(t)
	f.propose(t, "deal", f.deal)
	queue, _ := f.queue()
	if _, err := f.store.RejectLinkSuggestion(f.ctx, ids.UUID(queue[0].Id)); err != nil {
		t.Fatalf("rejecting: %v", err)
	}

	if got := f.propose(t, "deal", f.deal); got != SuggestionExisting {
		t.Errorf("a repeat answered %q, want existing", got)
	}
	if n := f.count(t, `SELECT count(*) FROM activity_link_suggestion WHERE activity_id = $1`); n != 1 {
		t.Errorf("%d suggestion rows after a repeat, want 1", n)
	}
	if queue, _ := f.queue(); len(queue) != 0 {
		t.Errorf("a rejected suggestion came back to the queue: %+v", queue)
	}
}

func TestTheMessageAuthorIsNeverSuggestedAsItsTarget(t *testing.T) {
	f := newSuggestionFixture(t)

	if got := f.propose(t, "contact", f.author); got != SuggestionExcluded {
		t.Fatalf("proposing the sender answered %q, want excluded", got)
	}
	if n := f.count(t, `SELECT count(*) FROM activity_link_suggestion WHERE activity_id = $1`); n != 0 {
		t.Errorf("the sender was written as a suggestion (%d rows)", n)
	}
	if got := f.propose(t, "contact", f.stranger); got != SuggestionCreated {
		t.Errorf("another contact answered %q, want created", got)
	}
}

func TestAcceptingFilesTheMessageAndClosesTheSuggestionTogether(t *testing.T) {
	f := newSuggestionFixture(t)
	f.propose(t, "deal", f.deal)
	queue, _ := f.queue()

	got, err := f.store.AcceptLinkSuggestion(f.ctx, ids.UUID(queue[0].Id))
	if err != nil {
		t.Fatalf("accepting: %v", err)
	}

	if got.Status != "accepted" {
		t.Errorf("status = %q, want accepted", got.Status)
	}
	if n := f.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`); n != 1 {
		t.Errorf("the message has %d deal links after an accept, want 1", n)
	}
	if again, err := f.store.AcceptLinkSuggestion(f.ctx, ids.UUID(queue[0].Id)); err != nil || again.Status != "accepted" {
		t.Errorf("accepting twice = %+v, %v; want the same success", again, err)
	}
	if _, err := f.store.RejectLinkSuggestion(f.ctx, ids.UUID(queue[0].Id)); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("rejecting an accepted suggestion = %v, want a conflict", err)
	}
}

func TestASuggestionForARecordTheReaderCannotSeeCannotBeDecided(t *testing.T) {
	f := newSuggestionFixture(t)
	f.propose(t, "deal", f.deal)
	queue, _ := f.queue()
	f.ctx = ownRowsSeat(f.e, f.e.other)

	_, err := f.store.RejectLinkSuggestion(f.ctx, ids.UUID(queue[0].Id))

	if !errors.Is(err, apperrors.ErrNotFound) && !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("deciding a suggestion for a deal the reader cannot see = %v, want a refusal", err)
	}
	if n := f.count(t, `SELECT count(*) FROM activity_link_suggestion WHERE activity_id = $1 AND status = 'pending'`); n != 1 {
		t.Errorf("%d pending suggestions after the refused decision, want 1", n)
	}
}

func TestRejectingAttachesNothing(t *testing.T) {
	f := newSuggestionFixture(t)
	f.propose(t, "deal", f.deal)
	queue, _ := f.queue()
	linksBefore := f.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`)

	if _, err := f.store.RejectLinkSuggestion(f.ctx, ids.UUID(queue[0].Id)); err != nil {
		t.Fatalf("rejecting: %v", err)
	}

	if after := f.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1`); after != linksBefore {
		t.Errorf("a rejection changed the links from %d to %d", linksBefore, after)
	}
}

func TestAProposalNeedsTheRightToUpdateTheMessage(t *testing.T) {
	f := newSuggestionFixture(t)
	f.ctx = readOnlySeat(f.e, f.e.rep)

	err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		_, err := ProposeLinkSuggestionTx(f.ctx, tx, LinkSuggestionInput{
			ActivityID: ids.From[ids.ActivityKind](f.message), EntityType: "deal", EntityID: f.deal,
			Unit: "finance-demo", Reason: "r", Score: 0.5,
		})
		return err
	})

	if !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a seat that cannot update activities proposed: %v, want permission denied", err)
	}
}

func TestAProposalIsRefusedForABadShape(t *testing.T) {
	f := newSuggestionFixture(t)
	for name, in := range map[string]LinkSuggestionInput{
		"a lead":       {EntityType: "lead", Reason: "r", Score: 0.5},
		"no reason":    {EntityType: "deal", Reason: "  ", Score: 0.5},
		"score over 1": {EntityType: "deal", Reason: "r", Score: 1.5},
	} {
		in.ActivityID, in.EntityID, in.Unit = ids.From[ids.ActivityKind](f.message), f.deal, "finance-demo"
		err := f.store.tx(f.ctx, func(tx pgx.Tx) error { _, err := ProposeLinkSuggestionTx(f.ctx, tx, in); return err })
		if err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
}
