// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// One person's feed, read from the projection against a real database.
//
// Every row here arrives the way production writes one — a real reading moved
// by activities.Store, announced on the bus, projected by the real consumer.
// The one exception is ageing a lease, which no writer can do because
// stale_after is derived from timestamps the database stamped.

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/aiactivity"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// feed reads one person's own view.
//
// The person comes from the BOUND PRINCIPAL, because that is the only way the
// read can be asked at all — another person's feed is not expressible. "Today"
// comes from the database clock, so the boundary is the one the rows were
// stamped against rather than the test host's idea of the date.
func (f *readingFixture) feed(t *testing.T, user ids.UUID) aiactivity.Feed {
	t.Helper()
	feed, err := aiactivity.NewStore(f.env.DB()).
		Mine(f.env.As(user, nil, principal.Permissions{}), f.midnight(t), nil)
	if err != nil {
		t.Fatalf("Mine: %v", err)
	}
	return feed
}

// midnight is the start of the database's today, read from the database.
func (f *readingFixture) midnight(t *testing.T) time.Time {
	t.Helper()
	var midnight time.Time
	if err := f.env.Pool.QueryRow(context.Background(),
		`SELECT date_trunc('day', now())`).Scan(&midnight); err != nil {
		t.Fatalf("reading the database's idea of today: %v", err)
	}
	return midnight
}

// A queued reading is LIVE for the person who asked for it — queued is work in
// progress to them, not an absence.
func TestAQueuedReadingIsLiveInItsOwnPersonsFeed(t *testing.T) {
	f := newReadingFixture(t)
	f.drain(t)

	feed := f.feed(t, f.env.AdminUser)
	live, settled := feed.Live, feed.Settled
	if len(live) != 1 || len(settled) != 0 {
		t.Fatalf("live/settled = %d/%d, want 1/0", len(live), len(settled))
	}
	if live[0].State != "queued" || live[0].Kind != "document_extract" {
		t.Fatalf("state/kind = %s/%s, want queued/document_extract", live[0].State, live[0].Kind)
	}
}

// The feed is PERSONAL. Another seat sees none of it, and the separation is the
// row's own actor rather than anything the caller passes.
func TestOnePersonsWorkIsNotInAnothersFeed(t *testing.T) {
	f := newReadingFixture(t)
	f.drain(t)

	if feed := f.feed(t, f.env.Rep2); len(feed.Live) != 0 || len(feed.Settled) != 0 {
		t.Fatalf("a different seat sees %d live and %d settled occurrences, want none", len(feed.Live), len(feed.Settled))
	}
}

// A live occurrence past the lease its own source declared reads as `stalled`,
// and it is DERIVED — nothing stores the word, so no writer can forget it.
func TestALiveOccurrencePastItsLeaseReadsAsStalled(t *testing.T) {
	f := newReadingFixture(t)
	if _, err := f.store.BeginExtractionRead(f.ctx, f.readID, activities.ExtractionReadLease); err != nil {
		t.Fatalf("BeginExtractionRead: %v", err)
	}
	f.drain(t)
	if live := f.feed(t, f.env.AdminUser).Live; live[0].State != "running" {
		t.Fatalf("state = %s, want running before the lease elapses", live[0].State)
	}

	// The projection's own column, aged: stale_after is computed from database
	// timestamps, so there is no writer that takes it as a parameter.
	if _, err := f.env.Pool.Exec(context.Background(),
		`UPDATE ai_task_run SET stale_after = now() - interval '1 minute'
		  WHERE source = $1 AND occurrence_key = $2`,
		"attachment_extraction", f.readID.String()); err != nil {
		t.Fatalf("ageing the lease: %v", err)
	}

	live := f.feed(t, f.env.AdminUser).Live
	if len(live) != 1 || live[0].State != aiactivity.StateStalled {
		t.Fatalf("state = %v, want %q — a worker that died without saying so must not read as working",
			live, aiactivity.StateStalled)
	}
	if got := f.projection(t).State; got != "running" {
		t.Fatalf("the STORED state is %q; stalled is derived at read time and must never be written", got)
	}
}

// A settled occurrence never goes stale, whatever its lease said: it is not
// claiming to be working, so there is nothing to be past.
func TestASettledOccurrenceIsNeverStalled(t *testing.T) {
	f := newReadingFixture(t)
	claim, err := f.store.BeginExtractionRead(f.ctx, f.readID, activities.ExtractionReadLease)
	if err != nil {
		t.Fatalf("BeginExtractionRead: %v", err)
	}
	if err := f.store.FinishExtractionRead(f.ctx, f.readID, activities.ExtractionReadOutcome{
		Status: activities.ExtractionReadDone, ClaimedAt: *claim.StartedAt,
		Detail: "the document states none of the four fields",
	}); err != nil {
		t.Fatalf("FinishExtractionRead: %v", err)
	}
	f.drain(t)

	feed := f.feed(t, f.env.AdminUser)
	live, settled := feed.Live, feed.Settled
	if len(live) != 0 || len(settled) != 1 {
		t.Fatalf("live/settled = %d/%d, want 0/1", len(live), len(settled))
	}
	if settled[0].State != "done" {
		t.Fatalf("state = %s, want done", settled[0].State)
	}
	if settled[0].FinishedAt == nil {
		t.Fatal("a settled occurrence with no finish would break the feed's keyset ordering")
	}
}

// A read with nobody bound is refused rather than answered with everybody's
// work. There is no id to pass, so this is the ONLY way to ask wrongly.
func TestAFeedWithNoPersonIsRefused(t *testing.T) {
	f := newReadingFixture(t)
	if _, err := aiactivity.NewStore(f.env.DB()).
		Mine(context.Background(), f.midnight(t), nil); err == nil {
		t.Fatal("a personal read with nobody bound must be refused, not answered")
	}
}

// drainEvery delivers every staged ai_task.state_changed to the projection,
// whichever occurrence it belongs to.
//
// The fixture's own `drain` is deliberately narrow — it follows one reading and
// remembers how far it has got, so a replay cannot paper over a later event. A
// case that settles ELEVEN readings needs the wide one, and it is separate
// rather than a widening of that: the two answer different questions, and a
// drain that followed everything would let a replay of the first event hide
// what a repair path did to the row.
//
// Replay-safe because the projection is idempotent on the occurrence key, which
// is what lets this deliver the whole outbox rather than track a cursor per
// reading.
func (f *readingFixture) drainEvery(t *testing.T) {
	t.Helper()
	var raws [][]byte
	if err := f.env.DB().Tx(context.Background(), func(tx pgx.Tx) error {
		rows, err := tx.Query(context.Background(), `
			SELECT envelope FROM event_outbox
			 WHERE envelope->>'type' = 'ai_task.state_changed'
			 ORDER BY seq`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var raw []byte
			if scanErr := rows.Scan(&raw); scanErr != nil {
				return scanErr
			}
			raws = append(raws, raw)
		}
		return rows.Err()
	}); err != nil {
		t.Fatalf("reading the staged envelopes: %v", err)
	}
	if len(raws) == 0 {
		t.Fatal("nothing staged an ai_task.state_changed at all — no occurrence could reach the feed")
	}
	for _, raw := range raws {
		var env kevents.Envelope
		if err := json.Unmarshal(raw, &env); err != nil {
			t.Fatalf("decoding a staged envelope: %v", err)
		}
		if err := f.consumer.HandleEvent(context.Background(), env); err != nil {
			t.Fatalf("the projection refused envelope %s: %v", env.EventID, err)
		}
	}
}

// settleOne runs a reading of its own to completion with the outcome given.
//
// Its own attachment each time: the in-flight index admits one live reading per
// attachment, so a second reading on the same document cannot start.
//
// It returns nothing on purpose. The reading's id is its occurrence KEY and the
// projection mints an id of its own for the row, so handing the first back would
// offer a caller something that never matches what the feed answers with.
func (f *readingFixture) settleOne(t *testing.T, name, status, detail string) {
	t.Helper()
	att := uploadDealAttachment(f.ctx, t, f.handlers, f.deal, name, []byte("bytes for "+name))
	read, _, err := f.store.StartExtractionReadQueued(f.ctx, ids.UUID(att.Id), "human:"+f.env.AdminUser.String(), nil)
	if err != nil {
		t.Fatalf("starting the reading for %s: %v", name, err)
	}
	claim, err := f.store.BeginExtractionRead(f.ctx, read.ID, activities.ExtractionReadLease)
	if err != nil {
		t.Fatalf("claiming the reading for %s: %v", name, err)
	}
	if err := f.store.FinishExtractionRead(f.ctx, read.ID, activities.ExtractionReadOutcome{
		Status: status, ClaimedAt: *claim.StartedAt, Detail: detail,
	}); err != nil {
		t.Fatalf("finishing the reading for %s: %v", name, err)
	}
}

// failuresIn counts the occurrences of an arm that record a break.
//
// Counted by STATE rather than matched by id: the reading's own id is its
// occurrence KEY, and the projection mints an id of its own for the row, so a
// caller holding the first has nothing to compare against the second.
func failuresIn(items []aiactivity.Item) int {
	failures := 0
	for _, item := range items {
		if item.State == "failed" {
			failures++
		}
	}
	return failures
}

// THE DEFECT THIS ARM EXISTS FOR: a fault that later successes have pushed out
// of `recent`.
//
// The rail holds an unacknowledged fault until somebody opens the panel, and it
// used to read faults out of `recent` — the newest ten settled occurrences of
// any outcome. So an overnight failure was released by the day's ordinary work,
// with nobody having looked: the product knew its scheduled run failed and
// stopped saying so.
//
// Eleven readings, all through the real writer and the real projection. The
// fault settles FIRST, so it is the oldest and the one `recent` drops.
func TestAFaultSurvivesTheSettledBoundLaterSuccessesFill(t *testing.T) {
	f := newReadingFixture(t)
	f.settleOne(t, "overnight.pdf", activities.ExtractionReadFailed,
		"the provider refused the document")
	// One more than the settled bound, so the fault is certainly displaced.
	for i := range 11 {
		f.settleOne(t, fmt.Sprintf("later-%02d.pdf", i), activities.ExtractionReadDone,
			"the document states none of the four fields")
	}
	f.drainEvery(t)

	feed := f.feed(t, f.env.AdminUser)

	// The premise: `recent` really has dropped it. Without this the case below
	// would pass on a day the bound never bit, which is every day with fewer
	// than ten runs — most of them.
	if failuresIn(feed.Settled) != 0 {
		t.Fatalf("the failure is still in `recent` (%d rows) — the bound did not bite, so this "+
			"test proves nothing about the arm", len(feed.Settled))
	}
	if failuresIn(feed.Faults) != 1 {
		t.Errorf("the faults arm holds %d failures among %d rows, want the one that `recent` dropped. "+
			"An unacknowledged failure the reader never saw was released by the day's own successes",
			failuresIn(feed.Faults), len(feed.Faults))
	}
	// And the arm carries ONLY what went wrong: eleven successes settled beside
	// it, and an arm that simply mirrored `recent` would hold them too.
	if len(feed.Faults) != 1 {
		t.Errorf("the faults arm carries %d rows, want the one failure alone", len(feed.Faults))
	}
}
