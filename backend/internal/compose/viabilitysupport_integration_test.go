// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A deal the customer is talking about still needs a close date they back and
// a next step to be supported. A refusal is not the customer moving the deal.
// These run the real worker over deals written through the deals and
// activities stores, with the verdict written by the classifier's own writer.

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// customerMails logs a message the buyer sent on the deal and returns it.
func (v *viabilityEnv) customerMails(t *testing.T, deal ids.UUID, at time.Time) ids.UUID {
	t.Helper()
	subject, inbound := "Re: the proposal", "inbound"
	mail, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: &inbound, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "deal", EntityID: deal}, {EntityType: "contact", EntityID: v.buyer},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	v.WsExec(t, `UPDATE activity_participant SET role = 'from' WHERE activity_id = $1 AND contact_id = $2`,
		ids.UUID(mail.Id), v.buyer)
	return ids.UUID(mail.Id)
}

func (v *viabilityEnv) receiptOf(t *testing.T, deal ids.UUID) (outcome, reasons string) {
	t.Helper()
	return v.WsScalar(t, `SELECT outcome FROM assurance_deal_receipt WHERE deal_id = $1`, deal),
		v.WsScalar(t, `SELECT array_to_string(reasons, ',') FROM assurance_deal_receipt WHERE deal_id = $1`, deal)
}

func TestARefusalRequiresAReviewAndAnUnjudgedReplyCounts(t *testing.T) {
	v := setupViability(t)
	refusal := v.customerMails(t, v.supported, v.now.AddDate(0, 0, -2))
	if applied, err := activities.NewStore(v.DB()).SetReplyVerdict(v.Admin(), refusal,
		activities.ReplyVerdictNegative, "test"); err != nil || !applied {
		t.Fatalf("judging the reply: applied=%v err=%v", applied, err)
	}
	v.customerMails(t, v.silent, v.now.AddDate(0, 0, -1))

	v.check(t)
	if outcome, reasons := v.receiptOf(t, v.supported); outcome != "review_required" || !hasReason(reasons, "customer_declined") {
		t.Fatalf("after a refusal the receipt reads %s %q, want a review for customer_declined", outcome, reasons)
	}
	if _, reasons := v.receiptOf(t, v.silent); hasReason(reasons, "customer_silent") {
		t.Fatalf("an unjudged reply did not end the silence: %q", reasons)
	}

	v.customerMails(t, v.supported, v.now.Add(-time.Hour))
	v.check(t)
	if _, reasons := v.receiptOf(t, v.supported); hasReason(reasons, "customer_declined") {
		t.Fatalf("a later message did not outrank the refusal: %q", reasons)
	}
}

func TestADealWithNoNextStepIsUnsupported(t *testing.T) {
	v := setupViability(t)
	step, err := ids.Parse(v.WsScalar(t, `SELECT a.id::text FROM activity a
		JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'task'`, v.supported))
	if err != nil {
		t.Fatal(err)
	}
	if done, err := v.Activities.CompleteTask(v.Admin(), ids.From[ids.ActivityKind](step), clocktest.Now(t)); err != nil || !done {
		t.Fatalf("completing the next step: done=%v err=%v", done, err)
	}

	// Twice: the first pass files the "no next step" finding's own task, and
	// that task must not pass for the next step on the second.
	for range 2 {
		v.check(t)
		if outcome, reasons := v.receiptOf(t, v.supported); outcome != "unsupported" || reasons != "no_next_step" {
			t.Fatalf("receipt %s %q, want unsupported for the missing next step", outcome, reasons)
		}
	}
}

func TestARefusalNotEveryMemberMayReadMovesNothing(t *testing.T) {
	v := setupViability(t)
	refusal := v.customerMails(t, v.supported, v.now.AddDate(0, 0, -2))
	if applied, err := activities.NewStore(v.DB()).SetReplyVerdict(v.Admin(), refusal,
		activities.ReplyVerdictNegative, "test"); err != nil || !applied {
		t.Fatalf("judging the reply: applied=%v err=%v", applied, err)
	}
	if _, err := v.Activities.SetAudience(v.Admin(), ids.From[ids.ActivityKind](refusal),
		activities.SetAudienceInput{Audience: "participants"}); err != nil {
		t.Fatalf("limiting the reply: %v", err)
	}

	v.check(t)
	if _, reasons := v.receiptOf(t, v.supported); hasReason(reasons, "customer_declined") {
		t.Fatalf("a limited refusal raised %q; it must not reach a figure everybody reads", reasons)
	}
}

func TestAVerdictThatLandsAfterTheCheckMakesTheReceiptStale(t *testing.T) {
	v := setupViability(t)
	reply := v.customerMails(t, v.supported, v.now.AddDate(0, 0, -2))
	v.check(t)
	if v.receiptStale(t, v.supported) {
		t.Fatal("the receipt is stale before the verdict")
	}
	if applied, err := activities.NewStore(v.DB()).SetReplyVerdict(v.Admin(), reply,
		activities.ReplyVerdictNegative, "test"); err != nil || !applied {
		t.Fatalf("judging the reply: applied=%v err=%v", applied, err)
	}
	if !v.receiptStale(t, v.supported) {
		t.Fatal("a reply that turned out a refusal left the receipt current")
	}
}
