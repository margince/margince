// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func holdAnswerRepair(t *testing.T, o *owedEnv, answer ids.UUID) {
	t.Helper()
	key := o.threadKey(t, answer)
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		return capture.QueueThreadRepairTx(o.e.Admin(), tx, key, true)
	}); err != nil {
		t.Fatal(err)
	}
	if o.e.WsCount(t, `SELECT count(*) FROM activity WHERE id=$1 AND NOT activity_thread_available(thread_key)`, answer) != 1 {
		t.Fatal("the answer did not become temporarily unavailable")
	}
}

func settlementRepairItem(t *testing.T, o *owedEnv, request ids.UUID) activities.SettlementRepairItem {
	t.Helper()
	var found activities.SettlementRepairItem
	if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
		items, err := activities.SettlementRepairPageTx(o.e.Admin(), tx, ids.Nil, 100, o.now)
		for _, item := range items {
			if item.RequestID == request {
				found = item
			}
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	return found
}

func TestAnUnavailableOffThreadAnswerCannotWithdrawOrReclassifySettlement(t *testing.T) {
	for _, legacy := range []bool{false, true} {
		name := "recorded evidence"
		if legacy {
			name = "legacy review"
		}
		t.Run(name, func(t *testing.T) {
			o := setupOwed(t)
			o.now = clocktest.Now(t)
			o.contact(t, "Pat Buyer", "pat@customer.example")
			request := o.askOf(t, "Signed NDA", o.now.Add(-3*time.Hour))
			answer := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Signed NDA", at: o.now.Add(-2 * time.Hour)})
			finishThreadRepairs(t, o.e)
			candidate, ok := o.offered(t, request)
			if !ok || candidate.NewestAnswerID != answer {
				t.Fatalf("missing native off-thread answer: %+v", candidate)
			}
			actor := o.classifier()
			actor.ID = activities.RequestSettlementActor
			ctx := principal.WithActor(o.e.Admin(), actor)
			if err := o.e.Activities.SettleRequest(ctx, activities.RequestSettlementInput{Request: candidate, Verdict: activities.RequestSettled, Confidence: 0.95, DecidedBy: "test-model"}); err != nil {
				t.Fatal(err)
			}
			var reviewed activities.SettlementRepairItem
			if legacy {
				o.e.WsExec(t, `UPDATE activity_request_settlement SET judged_evidence=NULL WHERE request_activity_id=$1`, request)
				reviewed = settlementRepairItem(t, o, request)
				if reviewed.Category != activities.SettlementRevalidate {
					t.Fatalf("initial review=%+v", reviewed)
				}
			}
			holdAnswerRepair(t, o, answer)
			if n, err := o.e.Activities.WithdrawLostSettlements(ctx, o.now, 100); err != nil || n != 0 {
				t.Fatalf("repair withdrew settlement: count=%d err=%v", n, err)
			}
			if got := o.e.WsScalar(t, `SELECT verdict FROM activity_request_settlement WHERE request_activity_id=$1`, request); got != activities.RequestSettled {
				t.Fatalf("verdict changed during repair: %s", got)
			}
			if legacy {
				item := settlementRepairItem(t, o, request)
				if item.Category != activities.SettlementUnavailable || item.Snapshot != "" {
					t.Fatalf("deferred proof review=%+v", item)
				}
				if err := database.WithWorkspaceTx(o.e.Admin(), o.e.Pool, func(tx pgx.Tx) error {
					wrote, err := activities.RequestSettlementRevalidationTx(o.e.Admin(), tx, reviewed, o.now)
					if wrote {
						t.Error("a stale legacy review applied during answer repair")
					}
					return err
				}); err != nil {
					t.Fatal(err)
				}
			}
			finishThreadRepairs(t, o.e)
			if n, err := o.e.Activities.WithdrawLostSettlements(ctx, o.now, 100); err != nil || n != 0 {
				t.Fatalf("restored proof withdrew settlement: count=%d err=%v", n, err)
			}
			if legacy && settlementRepairItem(t, o, request).Category != activities.SettlementRevalidate {
				t.Fatal("restored answer was not eligible for revalidation")
			}
			if _, err := o.e.Activities.ArchiveActivity(o.reader(), ids.From[ids.ActivityKind](answer), nil); err != nil {
				t.Fatal(err)
			}
			finishThreadRepairs(t, o.e)
			if legacy {
				if settlementRepairItem(t, o, request).Category != activities.SettlementNoAnswers {
					t.Fatal("an actually removed answer was not classified as missing")
				}
			} else if n, err := o.e.Activities.WithdrawLostSettlements(ctx, o.now, 100); err != nil || n != 1 {
				t.Fatalf("actual removal did not withdraw: count=%d err=%v", n, err)
			}
		})
	}
}

func TestSettlementDoesNotJudgeAPartialSetOfOffThreadAnswers(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	o.contact(t, "Pat Buyer", "pat@customer.example")
	request := o.askOf(t, "Signed NDA", o.now.Add(-4*time.Hour))
	o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Signed NDA", at: o.now.Add(-3 * time.Hour)})
	answer := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Re: Signed NDA", at: o.now.Add(-2 * time.Hour)})
	finishThreadRepairs(t, o.e)
	candidate, ok := o.offered(t, request)
	if !ok {
		t.Fatal("complete answer set was not offered")
	}
	holdAnswerRepair(t, o, answer)
	if _, ok := o.offered(t, request); ok {
		t.Fatal("a partial answer set was offered for judgement")
	}
	if err := o.e.Activities.SettleRequest(o.judge(), activities.RequestSettlementInput{Request: candidate, Verdict: activities.RequestSettled, Confidence: 0.95, DecidedBy: "test-model"}); err != nil {
		t.Fatal(err)
	}
	if o.e.WsCount(t, `SELECT count(*) FROM activity_request_settlement WHERE request_activity_id=$1`, request) != 0 {
		t.Fatal("a stale candidate published over unavailable proof")
	}
	finishThreadRepairs(t, o.e)
	if _, ok := o.offered(t, request); !ok {
		t.Fatal("the complete restored answer set was not offered")
	}
}

func assertMeetingFollowUps(t *testing.T, o *owedEnv, meeting, deal ids.UUID, want bool) {
	t.Helper()
	ctx := o.reader()
	check := func(name string, rows []activities.AwaitingReply, err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, row := range rows {
			if row.ActivityID == meeting {
				found = true
			}
		}
		if found != want {
			t.Fatalf("%s offered meeting=%v, want %v", name, found, want)
		}
	}
	rows, err := o.e.Activities.MeetingsAwaitingFollowUp(ctx, o.now)
	check("all meetings", rows, err)
	rows, err = o.e.Activities.MeetingsAwaitingFollowUpOnDeal(ctx, o.now, deal)
	check("deal meetings", rows, err)
	rows, err = o.e.Activities.NextMeetingFollowUpOnDeal(ctx, o.now, deal)
	check("next deal meeting", rows, err)
	still, err := o.e.Activities.MeetingsStillAwaitingFollowUp(ctx, o.now, []ids.UUID{meeting})
	if err != nil || still[meeting] != want {
		t.Fatalf("recheck offered=%v want=%v err=%v", still[meeting], want, err)
	}
	if err := database.WithWorkspaceTx(ctx, o.e.Pool, func(tx pgx.Tx) error {
		byDeal, err := o.e.Activities.NextMeetingFollowUpsByDealTx(ctx, tx, o.now, []ids.UUID{deal})
		check("batched next meeting", byDeal[deal], err)
		return err
	}); err != nil {
		t.Fatal(err)
	}
}

func TestAnUnavailableLaterTouchCannotCreateAMeetingFollowUp(t *testing.T) {
	o := setupOwed(t)
	o.now = clocktest.Now(t)
	pat := o.contact(t, "Pat Buyer", "pat@customer.example")
	pipeline, open, _ := integration.DealFixture(t, o.e)
	deal, err := o.e.Deals.CreateDeal(o.e.Admin(), deals.CreateDealInput{Name: "Follow-up deal", PipelineID: pipeline, StageID: open, Source: "manual"})
	if err != nil {
		t.Fatal(err)
	}
	subject, held, at := "Discovery workshop", "held", o.now.Add(-4*24*time.Hour)
	host := ids.From[ids.UserKind](o.owner)
	meeting, _, err := o.e.Activities.LogActivity(o.reader(), activities.LogActivityInput{
		Kind: "meeting", Subject: &subject, OccurredAt: &at, MeetingStatus: &held, HostUserID: &host, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: pat}, {EntityType: "deal", EntityID: ids.UUID(deal.Id)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	assertMeetingFollowUps(t, o, ids.UUID(meeting.Id), ids.UUID(deal.Id), true)
	answer := o.weWrite(t, owedMail{to: "pat@customer.example", subject: "Meeting recap", at: o.now.Add(-24 * time.Hour)})
	finishThreadRepairs(t, o.e)
	assertMeetingFollowUps(t, o, ids.UUID(meeting.Id), ids.UUID(deal.Id), false)
	holdAnswerRepair(t, o, answer)
	assertMeetingFollowUps(t, o, ids.UUID(meeting.Id), ids.UUID(deal.Id), false)
	finishThreadRepairs(t, o.e)
	assertMeetingFollowUps(t, o, ids.UUID(meeting.Id), ids.UUID(deal.Id), false)
	if _, err := o.e.Activities.ArchiveActivity(o.reader(), ids.From[ids.ActivityKind](answer), nil); err != nil {
		t.Fatal(err)
	}
	finishThreadRepairs(t, o.e)
	assertMeetingFollowUps(t, o, ids.UUID(meeting.Id), ids.UUID(deal.Id), true)
}
