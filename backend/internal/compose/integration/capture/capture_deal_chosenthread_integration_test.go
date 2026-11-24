// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

// A deal a colleague chose for one mail of a conversation is followed by the
// rest of it, whatever company its people work at. The common case is a
// partner: the deal sits on the end customer, and the mail goes back and forth
// with the partner.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const partnerAddress = "pat@partner.example"

// partnerWorld is a customer with one open deal, and a partner company with a
// contact who is no stakeholder on it.
func partnerWorld(t *testing.T, env captureEnv) (w, partner dealWorld, deal ids.UUID) {
	t.Helper()
	w = newDealWorld(t, env.e)
	w.employee(t, "alice@acme.example", w.customer)
	deal = w.deal(t, "Acme rollout")
	partner = w.at(t, "Partner")
	partner.employee(t, partnerAddress, partner.customer)
	return w, partner, deal
}

// partnerReply is the partner's authenticated answer in the thread that
// started at root.
func partnerReply(msgID, root string) []byte {
	return authenticated(mailFrom(partnerAddress, msgID, root, "Re: project"), "partner.example")
}

func TestCaptureFilesAReplyOnTheDealAColleagueChoseAtAnotherCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w, _, deal := partnerWorld(t, env)

	env.syncSent(t, map[string]bool{"cho1@myco.example": true},
		email(captureOwner, "", partnerAddress, "cho1@myco.example", ""))
	wantDeals(t, e, "cho1@myco.example")
	relinkAsColleague(t, e, w, activityOf(t, e, "cho1@myco.example"), "deal", deal)

	env.sync(t, partnerReply("cho2@partner.example", "cho1@myco.example"))
	wantDeals(t, e, "cho2@partner.example", deal)
	if changed := newRepairer(e, w).run(t); changed != 0 {
		t.Fatalf("the repair changed %d mails, want the followed deal confirmed", changed)
	}
	wantDeals(t, e, "cho2@partner.example", deal)
}

// Only a colleague's choice crosses companies. A deal link the connector wrote
// on the partner's mail is the machine's, and the reply does not follow it.
func TestCaptureFollowsNoAutomaticDealAtAnotherCompany(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	_, _, deal := partnerWorld(t, env)

	env.syncFiledUnderDeal(t, map[string][]ids.UUID{"auto1@partner.example": {deal}},
		authenticated(mailFrom(partnerAddress, "auto1@partner.example", "", "project"), "partner.example"))
	wantDeals(t, e, "auto1@partner.example", deal)

	env.syncSent(t, map[string]bool{"auto2@myco.example": true},
		email(captureOwner, "", partnerAddress, "auto2@myco.example", "auto1@partner.example"))
	wantDeals(t, e, "auto2@myco.example")
}

// A colleague's "not about this deal" on the reply holds against the choice
// made for the conversation, for live capture and for the repair.
func TestADeclineOnTheReplyHoldsAgainstTheConversationsChosenDeal(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w, _, deal := partnerWorld(t, env)

	env.syncSent(t, map[string]bool{"dcl1@myco.example": true},
		email(captureOwner, "", partnerAddress, "dcl1@myco.example", ""))
	relinkAsColleague(t, e, w, activityOf(t, e, "dcl1@myco.example"), "deal", deal)
	reply := partnerReply("dcl2@partner.example", "dcl1@myco.example")
	env.sync(t, reply)
	wantDeals(t, e, "dcl2@partner.example", deal)

	mustDecline(w.ctx, t, e, activityOf(t, e, "dcl2@partner.example"), deal)
	env.sync(t, reply)
	if n := newRepairer(e, w).run(t); n != 0 {
		t.Fatalf("the repair changed %d mails after the decline, want none", n)
	}
	query := `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id AND l.entity_type = 'deal'
		WHERE a.source_id = 'dcl2@partner.example' AND l.deal_relevance IS DISTINCT FROM 'unrelated'`
	if n := countRows(t, e, query); n != 0 {
		t.Fatalf("the declined reply stands on %d deals, want none", n)
	}
}

// A choice a colleague took back no longer leads the conversation.
func TestADeclinedChoiceIsNotFollowed(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w, _, deal := partnerWorld(t, env)

	env.syncSent(t, map[string]bool{"back1@myco.example": true},
		email(captureOwner, "", partnerAddress, "back1@myco.example", ""))
	first := activityOf(t, e, "back1@myco.example")
	relinkAsColleague(t, e, w, first, "deal", deal)
	mustDecline(w.ctx, t, e, first, deal)

	env.sync(t, partnerReply("back2@partner.example", "back1@myco.example"))
	wantDeals(t, e, "back2@partner.example")
}

// Two colleagues chose two deals for one conversation. They disagree, so the
// reply goes on neither, and the partner's own sole open deal does not decide.
func TestTwoDealsChosenForOneConversationFileTheReplyOnNeither(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w, partner, rollout := partnerWorld(t, env)

	env.syncSent(t, map[string]bool{"two1@myco.example": true, "two2@myco.example": true},
		email(captureOwner, "", partnerAddress, "two1@myco.example", ""),
		email(captureOwner, "", partnerAddress, "two2@myco.example", "two1@myco.example"))
	reporting := partner.deal(t, "Partner reporting")
	relinkAsColleague(t, e, w, activityOf(t, e, "two1@myco.example"), "deal", rollout)
	relinkAsColleague(t, e, w, activityOf(t, e, "two2@myco.example"), "deal", reporting)

	env.sync(t, partnerReply("two3@partner.example", "two1@myco.example"))
	wantDeals(t, e, "two3@partner.example")
	if n, _ := whichDealQuestions(t, e); n != 0 {
		t.Fatalf("%d deal questions for two choices, want none", n)
	}
}

// The choice counts only on mail the capturing mailbox may read. A colleague's
// private sent mail, filed on the deal by hand, leads none of our mail there.
func TestAChoiceOnMailTheCapturingSeatCannotReadIsNotFollowed(t *testing.T) {
	env := newCaptureEnv(t)
	e := env.e
	w, _, deal := partnerWorld(t, env)
	colleague := secondSentMailbox(t, e, e.Rep3)

	colleague(t, map[string]bool{"hid1@myco.example": true}, nil,
		email(secondSeatAddress, "", partnerAddress, "hid1@myco.example", ""))
	owner, _ := principal.Actor(w.ctx)
	owner.ID, owner.UserID = "human:"+e.Rep3.String(), e.Rep3
	w.ctx = principal.WithActor(w.ctx, owner)
	relinkAsColleague(t, e, w, activityOf(t, e, "hid1@myco.example"), "deal", deal)
	wantDeals(t, e, "hid1@myco.example", deal)

	env.sync(t, partnerReply("hid2@partner.example", "hid1@myco.example"))
	wantDeals(t, e, "hid2@partner.example")
}
