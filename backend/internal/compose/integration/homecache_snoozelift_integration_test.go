// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A reply or meeting snooze lifts without a write to the brief. These hold that
// a kept Home answer which hid the item ends, and only for the snooze's owner.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/briefs"
	"github.com/margince/margince/backend/internal/compose/homecache"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// snoozedDeal is Rep1's brief item on a deal of theirs. Once snoozed, both
// reps' Home answers are kept after Rep1 set it down.
type snoozedDeal struct {
	e      *Env
	cache  *homecache.Cache
	engine *briefs.BriefEngine
	rep1   context.Context
	rep2   context.Context
	deal   ids.UUID
	item   ids.UUID
	stage  ids.StageID
	// pipeline holds every deal the fixture seeds; a second one would clash by name.
	pipeline ids.PipelineID
	// now is the database clock, which the cache judges validity by.
	now time.Time
}

func briefItemOnADeal(t *testing.T) snoozedDeal {
	t.Helper()
	e := Setup(t)
	s := snoozedDeal{
		e: e, cache: homecache.New(e.Pool), engine: briefs.NewBriefEngine(e.Pool, e.Contacts),
		rep1: e.As(e.Rep1, []ids.UUID{e.Team1}, AdminPerms), rep2: e.As(e.Rep2, []ids.UUID{e.Team1}, RepPerms),
	}
	s.pipeline, s.stage, _ = DealFixture(t, e)
	s.deal, s.now = s.seedDeal(t)
	run, err := s.engine.SnapshotRun(s.rep1, s.now)
	if err != nil || len(run.Items) == 0 {
		t.Fatalf("a run to snooze from: items=%d err=%v", len(run.Items), err)
	}
	s.item = run.Items[0].ID
	return s
}

// seedDeal adds an open deal of Rep1's and answers it with the database clock.
func (s snoozedDeal) seedDeal(t *testing.T) (deal ids.UUID, now time.Time) {
	t.Helper()
	if err := OwnerConn(t).QueryRow(context.Background(), `
		INSERT INTO deal (pipeline_id, stage_id, name, owner_id, status, source, captured_by, amount_minor, currency, expected_close_date)
		VALUES ($1, $2, 'Waiting deal', $3, 'open', 'manual', 'test', 6000000, 'EUR', now() + interval '5 days')
		RETURNING id, now()`, s.pipeline, s.stage, s.e.Rep1).Scan(&deal, &now); err != nil {
		t.Fatal(err)
	}
	return deal, now
}

// snooze sets the item down ten minutes ago and keeps both reps' answers now.
func (s snoozedDeal) snooze(t *testing.T, on values.ReopenCondition, meeting *ids.UUID) {
	t.Helper()
	if _, err := s.engine.MarkSnoozed(s.rep1, s.item, on, nil, meeting, s.now.Add(-10*time.Minute), nil); err != nil {
		t.Fatal(err)
	}
	keep(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist, nil, "today")
	keep(s.rep2, t, s.cache, s.e.Rep2, homecache.Worklist, nil, "today")
	if !served(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist) {
		t.Fatal("the answer kept while the snooze holds was not served")
	}
}

// captureMail lands a mail on the deal as a mailbox sync does. It is dated
// before the answer was kept, as most captured mail is: it was sent earlier.
func (s snoozedDeal) captureMail(t *testing.T, id, direction string, deal ids.UUID) {
	t.Helper()
	address := "buyer@waiting-customer.test"
	_, err := capture.NewSink(s.e.DB()).Upsert(s.e.ConnectorOwnerCtx(s.e.Rep1, "gmail"), connector.NormalizedRecord{
		EntityType: datasource.EntityActivity,
		NaturalKey: connector.NaturalKey{SourceSystem: connector.EmailSourceSystem, SourceID: "<" + id + "@waiting-customer.test>"},
		Fields: capture.ActivityFields{
			Kind: "email", Subject: "About the rollout", Body: "Here is our answer.",
			Direction: direction, OccurredAt: s.now.Add(-5 * time.Minute),
		},
		Links:  []datasource.EntityRef{{Type: datasource.EntityDeal, ID: deal}},
		Source: "gmail:" + id, CapturedBy: "connector:gmail",
		Raw:          []byte("From: " + address + "\r\nSubject: About the rollout\r\n\r\nHere is our answer."),
		Counterparty: connector.Counterparty{Email: address, Domain: "waiting-customer.test", Direction: direction},
		Addresses:    []string{address},
	})
	if err != nil {
		t.Fatalf("capturing mail %s: %v", id, err)
	}
}

// assertOwnerReadsTheItemAgain holds that Rep1's kept answer ended, Rep1's next
// read is assembled fresh with the deal back in the queue, and Rep2's answer stands.
func (s snoozedDeal) assertOwnerReadsTheItemAgain(t *testing.T) {
	t.Helper()
	if served(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist) {
		t.Error("the snooze lifted and its owner's kept answer, which hid the item, was still served")
	}
	held, err := s.engine.SnoozedDeals(s.rep1, s.now)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(held, s.deal) {
		t.Error("the snooze lifted and the deal is still held out of the owner's queue")
	}
	fresh := crmcontracts.WorklistView("decisions")
	answer, stale, err := homecache.Serve(s.rep1, s.cache, s.e.Rep1, homecache.Worklist, "home",
		func(context.Context) (crmcontracts.Worklist, error) { return crmcontracts.Worklist{View: &fresh}, nil },
		homecache.NoWalk[crmcontracts.Worklist]())
	s.cache.Settle()
	if err != nil || stale || answer.View == nil || *answer.View != fresh {
		t.Errorf("the owner's next Home read: stale %v (err %v), want the fresh %q view", stale, err, fresh)
	}
	if !served(s.rep2, t, s.cache, s.e.Rep2, homecache.Worklist) {
		t.Error("another reader's kept answer ended on a snooze that was not theirs")
	}
}

func TestACapturedReplyEndsTheSnoozeOwnersKeptHomeAnswer(t *testing.T) {
	s := briefItemOnADeal(t)
	s.snooze(t, values.ReopenOnReply, nil)
	s.captureMail(t, "reply", connector.DirectionInbound, s.deal)
	s.assertOwnerReadsTheItemAgain(t)
}

// A meeting that ends by the clock alone is not tested: the database clock cannot move between keep and lookup.
func TestACalendarCancellationEndsTheMeetingSnoozeOwnersKeptHomeAnswer(t *testing.T) {
	s := briefItemOnADeal(t)
	meeting := CalendarMeeting{Event: "demo", At: s.now.AddDate(0, 0, 2)}
	id := meeting.Capture(t, s.e)
	s.snooze(t, values.ReopenOnMeeting, &id)
	meeting.Cancel(t, s.e)
	s.assertOwnerReadsTheItemAgain(t)
}

// Our own chaser does not lift a reply snooze, so it leaves the kept answer.
func TestAMailThatLiftsNoSnoozeLeavesTheKeptHomeAnswer(t *testing.T) {
	s := briefItemOnADeal(t)
	s.snooze(t, values.ReopenOnReply, nil)
	s.captureMail(t, "chaser", connector.DirectionOutbound, s.deal)
	if !served(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist) {
		t.Error("a mail that lifted no snooze ended the kept answer")
	}
}

// A lifted snooze stays on its row, so an answer read after the lift already
// shows the item and is kept as any other.
func TestAnAnswerReadAfterTheLiftIsKept(t *testing.T) {
	s := briefItemOnADeal(t)
	s.snooze(t, values.ReopenOnReply, nil)
	s.captureMail(t, "reply", connector.DirectionInbound, s.deal)
	keep(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist, nil, "today")
	if !served(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist) {
		t.Error("an answer read after the snooze lifted was never served")
	}
}

// A reply on another deal of the owner's lifts nothing on this one.
func TestAReplyOnAnotherDealLeavesTheKeptHomeAnswer(t *testing.T) {
	s := briefItemOnADeal(t)
	other, _ := s.seedDeal(t)
	s.snooze(t, values.ReopenOnReply, nil)
	s.captureMail(t, "elsewhere", connector.DirectionInbound, other)
	if !served(s.rep1, t, s.cache, s.e.Rep1, homecache.Worklist) {
		t.Error("a reply on another deal ended the kept answer")
	}
}
