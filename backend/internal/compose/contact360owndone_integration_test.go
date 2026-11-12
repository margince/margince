// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ourMessageSent is when the first message of these tests went out.
var ourMessageSent = time.Date(2026, 8, 3, 9, 0, 0, 0, time.UTC)

// ourLastMessageCard is the contact page's card for the reader, judged daysLater
// days after ourMessageSent.
func ourLastMessageCard(as context.Context, t *testing.T, e *integration.Env, contact ids.UUID, daysLater int) crmcontracts.ContactMoment {
	t.Helper()
	page := pageAsOf(e, ourMessageSent.AddDate(0, 0, daysLater)).WithMessageDone(
		activities.NewStore(InstallationDB(e.Pool)).WithOwnDomains(ownDomainReader{store: capture.NewOwnDomainStore(InstallationDB(e.Pool))}))
	return momentOf(as, t, page, ids.From[ids.ContactKind](contact))
}

func restsOnOurMessage(moment crmcontracts.ContactMoment) bool {
	return moment.Rule == crmcontracts.ContactMomentRuleAwaitingReply || moment.Rule == crmcontracts.ContactMomentRuleGoneQuiet
}

// Done on the waiting card hides it, and the gone-quiet card the same message
// turns into on day seven. It is the reader's own: a colleague still sees the
// card. Our next message brings it back, with a Done naming the new message.
func TestDoneOnTheWaitingCardHoldsUntilTheNextMessage(t *testing.T) {
	e := integration.Setup(t)
	const address = "kim@kunde.example"
	contact := seedLinkedContact(t, e, address)
	colleague := e.As(e.Rep3, []ids.UUID{e.Team2}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects:  map[string]principal.ObjectGrant{"contact": {Read: true}, "activity": {Read: true}},
		RowScope: principal.RowScopeAll,
	})
	first := logEmailFor(e.Admin(), t, e, contact, address, "outbound", true, ourMessageSent)
	card := ourLastMessageCard(e.Admin(), t, e, contact, 2)
	if card.Rule != crmcontracts.ContactMomentRuleAwaitingReply || card.Done == nil || ids.UUID(card.Done.ActivityId) != first {
		t.Fatalf("before Done the card is %q with done %+v, want the waiting card offering Done on our message", card.Rule, card.Done)
	}

	if err := e.Activities.MarkMessageHandled(e.Admin(), ids.From[ids.ActivityKind](first)); err != nil {
		t.Fatalf("pressing Done: %v", err)
	}

	for _, day := range []int{2, 9} {
		if got := ourLastMessageCard(e.Admin(), t, e, contact, day); restsOnOurMessage(got) {
			t.Errorf("day %d: the %q card still shows after Done on the message it rests on", day, got.Rule)
		}
	}
	if got := ourLastMessageCard(colleague, t, e, contact, 2); got.Rule != crmcontracts.ContactMomentRuleAwaitingReply {
		t.Errorf("the reader's Done took the waiting card off a colleague's page too: %q", got.Rule)
	}

	second := logEmailFor(e.Admin(), t, e, contact, address, "outbound", true, ourMessageSent.AddDate(0, 0, 3))
	back := ourLastMessageCard(e.Admin(), t, e, contact, 4)
	if back.Rule != crmcontracts.ContactMomentRuleAwaitingReply || back.Done == nil || ids.UUID(back.Done.ActivityId) != second {
		t.Errorf("after our next message the card is %q with done %+v, want the waiting card on the new message", back.Rule, back.Done)
	}
}

// The Worklist's Done on the awaiting-reply row is the same state, so it hides
// the contact card too.
func TestAWorklistDoneHidesTheContactsWaitingCard(t *testing.T) {
	e := integration.Setup(t)
	const address = "lee@kunde.example"
	contact := seedLinkedContact(t, e, address)
	sent := logEmailFor(e.Admin(), t, e, contact, address, "outbound", true, ourMessageSent)
	if got := ourLastMessageCard(e.Admin(), t, e, contact, 2); got.Rule != crmcontracts.ContactMomentRuleAwaitingReply {
		t.Fatalf("before Done the card is %q, so this proves nothing", got.Rule)
	}

	if err := e.Activities.MarkWorklistRowsDone(e.Admin(), []activities.WorklistRowRef{{
		Source: string(crmcontracts.WorklistItemSourceAwaitingReply), RowID: sent.String(),
	}}); err != nil {
		t.Fatalf("pressing Done on the Worklist row: %v", err)
	}

	if got := ourLastMessageCard(e.Admin(), t, e, contact, 2); restsOnOurMessage(got) {
		t.Errorf("the contact page still shows the %q card after the Worklist's Done on its message", got.Rule)
	}
}

// A colleague's message on the reader's page is theirs to judge. The card
// still shows it, with no Done, and the writer refuses one.
func TestNoDoneOnAColleaguesWaitingMessage(t *testing.T) {
	e := integration.Setup(t)
	const address = "sam@kunde.example"
	contact := seedLinkedContact(t, e, address)
	colleague := e.As(e.Rep1, []ids.UUID{e.Team1}, principal.Permissions{
		RoleKeys: []string{"rep"},
		Objects: map[string]principal.ObjectGrant{
			"contact":  {Read: true, Update: true},
			"activity": {Create: true, Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})
	theirs := logEmailFor(colleague, t, e, contact, address, "outbound", true, ourMessageSent)
	card := ourLastMessageCard(e.Admin(), t, e, contact, 2)
	if card.Rule != crmcontracts.ContactMomentRuleAwaitingReply {
		t.Fatalf("the reader's card is %q, want the waiting card on the colleague's message", card.Rule)
	}
	if card.Done != nil {
		t.Errorf("the reader is offered Done on a colleague's message: %+v", card.Done)
	}
	if err := e.Activities.MarkMessageHandled(e.Admin(), ids.From[ids.ActivityKind](theirs)); err == nil {
		t.Error("the writer took the reader's Done on a colleague's message")
	}
}
