// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

// Margince's own deal question in the link suggestion queue, written and
// answered through the real store on a real database.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// seedDeal adds one more deal in the given status.
func (f suggestionFixture) seedDeal(t *testing.T, name, status string) ids.UUID {
	t.Helper()
	deal, pipeline, stage := ids.NewV7(), ids.NewV7(), ids.NewV7()
	f.e.exec(t, `INSERT INTO pipeline (id, name) VALUES ($1, $2)`, pipeline, "Pipeline "+pipeline.String())
	f.e.exec(t, `INSERT INTO stage (id, pipeline_id, name, "position") VALUES ($1, $2, 'Qualified', 1)`, stage, pipeline)
	f.e.exec(t, `INSERT INTO deal (id, name, status, owner_id, pipeline_id, stage_id, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, $6, 'manual', 'system')`, deal, name, status, f.e.rep, pipeline, stage)
	return deal
}

// askAbout asks the rep which of deals the message is about.
func (f suggestionFixture) askAbout(t *testing.T, mail ids.UUID, deals ...ids.UUID) {
	t.Helper()
	err := f.store.tx(f.ctx, func(tx pgx.Tx) error {
		return AskWhichDealTx(f.ctx, tx, WhichDealQuestion{
			Mail: ids.From[ids.ActivityKind](mail), Seat: f.e.rep, Deals: deals, Reason: "Which deal is this about?",
		})
	})
	if err != nil {
		t.Fatalf("asking the deal question: %v", err)
	}
}

// statusOf reads the status of the core question row about deal.
func (f suggestionFixture) statusOf(t *testing.T, deal ids.UUID) string {
	t.Helper()
	var status string
	if err := f.e.owner.QueryRow(context.Background(), `SELECT status FROM activity_link_suggestion
		 WHERE activity_id = $1 AND entity_id = $2 AND for_user IS NOT NULL`, f.message, deal).Scan(&status); err != nil {
		t.Fatalf("reading the question about %s: %v", deal, err)
	}
	return status
}

// A unit's suggestion and Margince's question about the same message and deal
// are two rows, whichever came first.
func TestAUnitSuggestionAndACoreQuestionKeepTheirOwnRows(t *testing.T) {
	for name, unitFirst := range map[string]bool{"the unit first": true, "the question first": false} {
		t.Run(name, func(t *testing.T) {
			f := newSuggestionFixture(t)
			if unitFirst {
				f.propose(t, "deal", f.deal)
				f.askAbout(t, f.message, f.deal)
			} else {
				f.askAbout(t, f.message, f.deal)
				f.propose(t, "deal", f.deal)
			}
			units := f.count(t, `SELECT count(*) FROM activity_link_suggestion WHERE activity_id = $1 AND unit IS NOT NULL`)
			questions := f.count(t, `SELECT count(*) FROM activity_link_suggestion WHERE activity_id = $1 AND for_user IS NOT NULL`)
			if units != 1 || questions != 1 {
				t.Fatalf("%d unit rows and %d question rows, want one of each", units, questions)
			}
		})
	}
}

// A deal that closed after Margince asked about it is gone from the question:
// the queue leaves it out and accepting it files nothing.
func TestADealClosedSinceTheQuestionIsGone(t *testing.T) {
	f := newSuggestionFixture(t)
	f.askAbout(t, f.message, f.deal)
	queue, err := f.queue()
	if err != nil || len(queue) != 1 {
		t.Fatalf("queue = %+v, %v; want the question", queue, err)
	}
	f.e.exec(t, `UPDATE deal SET status = 'lost' WHERE id = $1`, f.deal)

	if after, err := f.queue(); err != nil || len(after) != 0 {
		t.Fatalf("queue after the deal closed = %+v, %v; want it empty", after, err)
	}
	if _, err := f.store.AcceptLinkSuggestion(f.ctx, ids.UUID(queue[0].Id)); !errors.Is(err, apperrors.ErrNotFound) {
		t.Fatalf("accepting a closed deal answered %v, want not found", err)
	}
	if n := f.count(t, `SELECT count(*) FROM activity_link WHERE activity_id = $1 AND entity_type = 'deal'`); n != 0 {
		t.Fatalf("accepting a closed deal filed the message on %d deals", n)
	}
}

// An automatic filing of the mail on a deal answers the question as a
// colleague's filing does: that deal's row is accepted, the rest rejected.
func TestAnAutomaticFilingSettlesTheQuestion(t *testing.T) {
	f := newSuggestionFixture(t)
	other := f.seedDeal(t, "Pilot", "open")
	f.askAbout(t, f.message, f.deal, other)

	system := principal.SystemActing(f.ctx, "system:deal-rule")
	err := f.store.tx(system, func(tx pgx.Tx) error {
		filed, err := FileOnDealIfUnfiledTx(system, tx, ids.From[ids.ActivityKind](f.message), f.deal, map[string]any{"rule": "test"}, "")
		if err == nil && filed != DealFilingLanded {
			t.Errorf("the automatic filing answered %s, want landed", filed)
		}
		return err
	})
	if err != nil {
		t.Fatalf("filing automatically: %v", err)
	}
	if got, want := [2]string{f.statusOf(t, f.deal), f.statusOf(t, other)}, [2]string{"accepted", "rejected"}; got != want {
		t.Fatalf("the question rows read %v after the filing, want %v", got, want)
	}
}

// Accepting the deal the mail already carries answers the question too.
func TestAcceptingTheDealTheMailIsOnSettlesTheRest(t *testing.T) {
	f := newSuggestionFixture(t)
	other := f.seedDeal(t, "Pilot", "open")
	f.askAbout(t, f.message, f.deal, other)
	f.e.exec(t, `INSERT INTO activity_link (id, activity_id, entity_type, deal_id) VALUES ($1, $2, 'deal', $3)`,
		ids.NewV7(), f.message, f.deal)
	queue, err := f.queue()
	if err != nil {
		t.Fatalf("reading the queue: %v", err)
	}
	for _, suggestion := range queue {
		if ids.UUID(suggestion.EntityId) != f.deal {
			continue
		}
		got, err := f.store.AcceptLinkSuggestion(f.ctx, ids.UUID(suggestion.Id))
		if err != nil || got.Status != "accepted" {
			t.Fatalf("accepting the filed deal answered %+v, %v", got, err)
		}
	}
	if status := f.statusOf(t, other); status != "rejected" {
		t.Fatalf("the other deal's row is %s after the answer, want rejected", status)
	}
	if n := f.count(t, `SELECT count(*) FROM audit_log WHERE entity_id = $1 AND after->'link_suggestion'->>'status' = 'accepted'`); n != 1 {
		t.Fatalf("the accept left %d accepted records, want one", n)
	}
}

// Two mails of one conversation asked about at once ask the colleague once.
func TestTwoAsksInOneConversationAtOnceAskOnce(t *testing.T) {
	f := newSuggestionFixture(t)
	second := f.e.seedWait(t, "Re: Which project is this?", "contact_id", f.author)
	f.e.exec(t, `UPDATE activity SET thread_key = 'one-conversation' WHERE id = ANY($1)`, []ids.UUID{f.message, second})

	firstAsked, release := make(chan struct{}), make(chan struct{})
	firstDone, secondDone := make(chan error, 1), make(chan error, 1)
	ask := func(mail ids.UUID, hold bool) error {
		return f.store.tx(f.ctx, func(tx pgx.Tx) error {
			err := AskWhichDealTx(f.ctx, tx, WhichDealQuestion{
				Mail: ids.From[ids.ActivityKind](mail), Seat: f.e.rep, Deals: []ids.UUID{f.deal}, Reason: "Which deal is this about?",
			})
			if hold {
				close(firstAsked)
				<-release
			}
			return err
		})
	}
	go func() { firstDone <- ask(f.message, true) }()
	<-firstAsked
	go func() { secondDone <- ask(second, false) }()
	f.waitForAnAdvisoryLockWaiter(t)
	close(release)
	if err := <-firstDone; err != nil {
		t.Fatalf("the first ask: %v", err)
	}
	if err := <-secondDone; err != nil {
		t.Fatalf("the second ask: %v", err)
	}
	var rows int
	if err := f.e.owner.QueryRow(context.Background(), `SELECT count(*) FROM activity_link_suggestion
		 WHERE activity_id = ANY($1) AND for_user IS NOT NULL`, []ids.UUID{f.message, second}).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Fatalf("the conversation holds %d questions, want one", rows)
	}
}

// waitForAnAdvisoryLockWaiter returns once some transaction waits on an
// advisory lock, which here is the second ask queued behind the first.
func (f suggestionFixture) waitForAnAdvisoryLockWaiter(t *testing.T) {
	t.Helper()
	var waiting bool
	err := f.e.owner.QueryRow(context.Background(), `
		WITH RECURSIVE poll(n, seen) AS (
		    SELECT 0, false
		    UNION ALL
		    SELECT n + 1, EXISTS (SELECT 1 FROM pg_locks WHERE locktype = 'advisory' AND NOT granted)
		      FROM poll, LATERAL (SELECT pg_sleep(0.01)) pause
		     WHERE NOT seen AND n < 1000)
		SELECT bool_or(seen) FROM poll`).Scan(&waiting)
	if err != nil || !waiting {
		t.Fatalf("the second ask never waited for the first: %v", err)
	}
}

// A suggested deal's value reaches the queue only through the reader's field
// masks.
func TestTheQueueShowsADealsValueOnlyWhenTheReaderMaySeeIt(t *testing.T) {
	f := newSuggestionFixture(t)
	f.e.exec(t, `UPDATE deal SET amount_minor = 4200000, currency = 'EUR' WHERE id = $1`, f.deal)
	f.askAbout(t, f.message, f.deal)
	open, err := f.queue()
	if err != nil || len(open) != 1 || open[0].DealAmountMinor == nil || *open[0].DealAmountMinor != 4200000 {
		t.Fatalf("an unmasked reader's queue = %+v, %v; want the deal's value", open, err)
	}
	masked := f
	actor, _ := principal.Actor(f.ctx)
	actor.Permissions.FieldMasks = []principal.FieldMask{{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways}}
	masked.ctx = principal.WithActor(f.ctx, actor)
	hidden, err := masked.queue()
	if err != nil || len(hidden) != 1 || hidden[0].DealAmountMinor != nil || hidden[0].DealCurrency != nil {
		t.Fatalf("a masked reader's queue = %+v, %v; want the value withheld", hidden, err)
	}
}
