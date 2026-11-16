// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package activities

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// colleagueDomainBound binds a domain of our own for the badge's reads and
// returns it, unbinding at cleanup. The domain is unique to the test, so a
// parallel test's mail never comes from it.
func colleagueDomainBound(t *testing.T) string {
	t.Helper()
	domain := "colleagues-" + ids.NewV7().String() + ".test"
	UseColleagueDomains(func(context.Context, pgx.Tx) ([]string, error) { return []string{domain}, nil })
	t.Cleanup(func() { UseColleagueDomains(nil) })
	return domain
}

// mailFrom seeds an inbound mail the classifier judged asks_us, sent from
// address, as an invitation when invitation is set. It carries no seat stamp.
func mailFrom(t *testing.T, e *loadEnv, address string, invitation bool) ids.UUID {
	t.Helper()
	links := []ActivityLinkInput{{EntityType: "contact", EntityID: e.buyer(t)}}
	label := "commitment"
	if invitation {
		label = "meeting"
	}
	id := seedEmailRequestFiledUnder(t, e, "Invitation: Sync", label, OwedVerdictAsksUs,
		address, links, mailParticipants{From: address})
	if invitation {
		setCalendarMethod(t, e, id, "REQUEST")
	}
	return id
}

// badgeAndWorklist reads the History badge and the Worklist for one message,
// both knowing domain as our own.
func badgeAndWorklist(t *testing.T, e *loadEnv, id ids.UUID, domain string) (crmcontracts.EmailSummaryMove, bool) {
	t.Helper()
	at := requestInstant.Add(time.Hour)
	store := storeKnowing(e, domain)
	summaries, err := readingAt(store, at).EmailSummariesByID(e.as(), []ids.UUID{id})
	if err != nil {
		t.Fatal(err)
	}
	waiting, err := store.WaitingReplies(e.as(), at)
	if err != nil {
		t.Fatal(err)
	}
	onWorklist := false
	for _, row := range waiting {
		onWorklist = onWorklist || row.ActivityID == id
	}
	return summaries[id].Move, onWorklist
}

// A colleague's invitation without the seat stamp is owed nothing on the History
// badge, as on the Worklist. Both test the sender against our own domains.
func TestTheBadgeAndTheWorklistAgreeAColleaguesUnstampedInvitationIsNotOwed(t *testing.T) {
	e := setupLoad(t)
	domain := colleagueDomainBound(t)
	invitation := mailFrom(t, e, "colleague@"+domain, true)
	move, onWorklist := badgeAndWorklist(t, e, invitation, domain)
	if move != crmcontracts.EmailSummaryMoveNone || onWorklist {
		t.Errorf("a colleague's unstamped invitation: badge %q, Worklist %v; want none and off the Worklist", move, onWorklist)
	}
	if cited, settlement := openOnRequestReads(t, e, invitation, "colleague@"+domain, domain); cited || settlement {
		t.Errorf("a colleague's unstamped invitation: cited read open %v, settlement offers it %v; want neither", cited, settlement)
	}
}

// openOnRequestReads reports whether a cited-thread read finds the message
// unsettled, and whether the settlement pass offers it once answered.
func openOnRequestReads(t *testing.T, e *loadEnv, id ids.UUID, sender, domain string) (cited, settlement bool) {
	t.Helper()
	var settled map[ids.UUID]bool
	reader := e.asSeat(e.rep)
	if err := database.BindTo(e.pool, ids.From[ids.WorkspaceKind](e.ws)).Tx(reader, func(tx pgx.Tx) error {
		var err error
		settled, err = SettledCitations(reader, tx, []ids.UUID{id}, nil, requestInstant.Add(time.Hour))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	replyToCounterparty(t, e, id, sender, "Thanks, see you there.", requestInstant.Add(time.Hour))
	rows, err := storeKnowing(e, domain).RepliedRequests(asClassifier(e), requestInstant.Add(2*time.Hour), 64)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range rows {
		settlement = settlement || row.RequestID == id
	}
	return !settled[id], settlement
}

// The colleague test reads the sender, so a customer's invitation stays owed on
// both surfaces.
func TestAnOutsideInvitationStaysOwedOnTheBadgeAndTheWorklist(t *testing.T) {
	e := setupLoad(t)
	domain := colleagueDomainBound(t)
	invitation := mailFrom(t, e, "buyer@customer.test", true)
	move, onWorklist := badgeAndWorklist(t, e, invitation, domain)
	if move != crmcontracts.EmailSummaryMoveNeedsReply || !onWorklist {
		t.Errorf("a customer's invitation: badge %q, Worklist %v; want needs_reply and on the Worklist", move, onWorklist)
	}
	if cited, settlement := openOnRequestReads(t, e, invitation, "buyer@customer.test", domain); !cited || !settlement {
		t.Errorf("a customer's invitation: cited read open %v, settlement offers it %v; want both", cited, settlement)
	}
}

// The invitation rule is about invitations. A colleague's plain mail keeps the
// badge it had: the badge applies no sender test to it.
func TestAColleaguesPlainMailKeepsItsBadge(t *testing.T) {
	e := setupLoad(t)
	domain := colleagueDomainBound(t)
	plain := mailFrom(t, e, "colleague@"+domain, false)
	move, onWorklist := badgeAndWorklist(t, e, plain, domain)
	if move != crmcontracts.EmailSummaryMoveNeedsReply {
		t.Errorf("a colleague's plain mail asking us reads %q on the badge, want needs_reply", move)
	}
	if onWorklist {
		t.Error("a colleague's plain mail is on the Worklist, which excludes our own domains")
	}
}
