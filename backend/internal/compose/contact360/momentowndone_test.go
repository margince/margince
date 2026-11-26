// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contact360

import (
	"testing"

	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// weWroteLast is a page whose newest message is ours, sent daysAgo, unanswered.
func weWroteLast(daysAgo int) *crmcontracts.Contact360 {
	sent := openapi_types.UUID(ids.NewV7())
	return &crmcontracts.Contact360{LastOutboundAt: new(at(daysAgo)), LastOutboundActivityId: &sent}
}

// A Done on our last message hides the waiting card, and the gone-quiet card the
// same message turns into on day seven.
func TestDoneOnOurLastMessageHidesEveryCardRestingOnIt(t *testing.T) {
	for daysAgo, card := range map[int]crmcontracts.ContactMomentRule{
		2: crmcontracts.ContactMomentRuleAwaitingReply,
		9: crmcontracts.ContactMomentRuleGoneQuiet,
	} {
		page := weWroteLast(daysAgo)
		said := momentCopyFor("en")
		open := said.deriveMomentPast(readerCtx(), now, page, cardsPutAway(nil, activities.OwnMessageDone{}), asAssembled)
		if open.Rule != card {
			t.Fatalf("day %d: rule = %q before Done, want %q, so this proves nothing", daysAgo, open.Rule, card)
		}
		done := said.deriveMomentPast(readerCtx(), now, page,
			cardsPutAway(nil, activities.OwnMessageDone{SetAside: true}), asAssembled)
		if restsOnOurLastMessage(done) {
			t.Errorf("day %d: the %q card still shows after Done on the message it rests on", daysAgo, done.Rule)
		}
	}
}

// A dismissal of another card still holds next to a Done.
func TestDoneLeavesOtherDismissalsAsTheyWere(t *testing.T) {
	putAway := cardsPutAway(map[string]string{"moment:open_promise": "fp"}, activities.OwnMessageDone{SetAside: true})
	if !putAway(crmcontracts.ContactMoment{ClaimKey: "moment:open_promise", EvidenceFingerprint: "fp"}) {
		t.Error("a dismissed promise card came back once the reader marked our last message done")
	}
	if putAway(crmcontracts.ContactMoment{ClaimKey: "moment:open_promise", EvidenceFingerprint: "moved"}) {
		t.Error("a dismissal held after its evidence moved")
	}
}

// Done is offered only where pressing it changes something.
func TestDoneIsOfferedOnlyWhereItChangesSomething(t *testing.T) {
	markable := activities.OwnMessageDone{CanMark: true}
	cases := map[string]struct {
		rule     crmcontracts.ContactMomentRule
		standing activities.OwnMessageDone
		noSent   bool
		want     bool
	}{
		"waiting card the reader may mark":    {rule: crmcontracts.ContactMomentRuleAwaitingReply, standing: markable, want: true},
		"gone-quiet card the reader may mark": {rule: crmcontracts.ContactMomentRuleGoneQuiet, standing: markable, want: true},
		"a reader who may not mark it":        {rule: crmcontracts.ContactMomentRuleAwaitingReply},
		"a message already set aside": {
			rule:     crmcontracts.ContactMomentRuleAwaitingReply,
			standing: activities.OwnMessageDone{CanMark: true, SetAside: true},
		},
		"a card that rests on something else": {rule: crmcontracts.ContactMomentRuleOpenPromise, standing: markable},
		"no message of ours on the page":      {rule: crmcontracts.ContactMomentRuleAwaitingReply, standing: markable, noSent: true},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			page := weWroteLast(2)
			if tc.noSent {
				page.LastOutboundActivityId = nil
			}
			moment := crmcontracts.ContactMoment{Rule: tc.rule}
			offerDone(&moment, tc.standing, page)
			if got := moment.Done != nil; got != tc.want {
				t.Fatalf("done offered = %v, want %v", got, tc.want)
			}
			if tc.want && moment.Done.ActivityId != *page.LastOutboundActivityId {
				t.Errorf("done names %v, want our last message %v", moment.Done.ActivityId, *page.LastOutboundActivityId)
			}
		})
	}
}
