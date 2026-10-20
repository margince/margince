// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (e *scoutEnv) buyingEmail(t *testing.T, company ids.UUID, at time.Time, threads ...string) (ids.UUID, ids.UUID) {
	t.Helper()
	buyer := e.employee(t, "Mira Buyer", company)
	thread := ""
	if len(threads) > 0 {
		thread = threads[0]
	}
	subject, body, direction := "Platform request", "We need a platform migration. Please send a proposal.", "inbound"
	logged, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		ThreadKey: thread, Kind: "email", Subject: &subject, Body: &body, Direction: &direction, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return ids.UUID(logged.Id), buyer
}

func (e *scoutEnv) buyerPair(t *testing.T, company ids.UUID, at time.Time) {
	t.Helper()
	source, buyer := e.buyingEmail(t, company, at)
	e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
}

func TestMeetingsAndProposalsDoNotEstablishBuyerIntent(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	contact := e.employee(t, "Mira Buyer", company)
	e.meeting(e.Admin(), t, "Networking meeting", &contact, e.daysAgo(2))
	e.document(t, e.email(t, "Proposal copy", "outbound", company, e.daysAgo(1)), "Proposal.pdf", "application/pdf")
	if pass := e.pass(t); pass.Raised != 0 {
		t.Fatalf("meeting and copied proposal raised %d suggestions", pass.Raised)
	}
	e.buyerPair(t, company, e.daysAgo(1))
	if pass := e.pass(t); pass.Raised != 1 {
		t.Fatalf("verified buyer request raised %d suggestions, want 1", pass.Raised)
	}
}

func TestAmbiguousBuyerEmploymentRetiresANewDealProposal(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	source, buyer := e.buyingEmail(t, company, e.daysAgo(2))
	e.signal(t, "new_opportunity", company, source, e.now, ids.Nil, buyer)
	e.signal(t, "commitment_made", company, source, e.now, ids.Nil)
	e.pass(t)
	suggestion := e.onlySuggestion(e.signalReader(), t, company)
	other := e.SeedCompany(t, "Cedar Partner", nil)
	contactID, employer := ids.From[ids.ContactKind](buyer), ids.From[ids.CompanyKind](other)
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{Kind: "employment", ContactID: &contactID, CompanyID: &employer, Source: "manual"}); err != nil {
		t.Fatal(err)
	}
	_, err := e.decider().AcceptSuggestion(e.signalReader(), suggestion.ID, deals.AcceptSuggestionInput{})
	var superseded *deals.SuggestionSupersededError
	if !errors.As(err, &superseded) || !superseded.EvidenceChanged {
		t.Fatalf("acceptance = %v, want changed-evidence conflict", err)
	}
	if got := e.WsCount(t, `SELECT count(*) FROM deal WHERE company_id = $1`, company); got != 0 {
		t.Fatalf("ambiguous buyer created %d deals", got)
	}
}

func TestCopiedDocumentsCountOnceAndRevisionsRemainCited(t *testing.T) {
	e := setupScout(t)
	e.now = time.Date(2026, 6, 15, 12, 0, 0, 0, time.UTC)
	company := e.SeedCompany(t, "Maple Studio", nil)
	e.buyerPair(t, company, e.daysAgo(5))
	for _, age := range []int{4, 3} {
		e.document(t, e.email(t, "Proposal copy", "outbound", company, e.daysAgo(age)), "Proposal.pdf", "application/pdf")
	}
	e.document(t, e.email(t, "Revised proposal", "outbound", company, e.daysAgo(2)), "Proposal-revised.pdf", "application/pdf")
	e.pass(t)
	suggestion := e.onlySuggestion(e.signalReader(), t, company)
	documents := 0
	for _, item := range suggestion.Evidence {
		if item.Kind == deals.EvidenceAttachment {
			documents++
		}
	}
	if documents != 2 {
		t.Fatalf("cited %d documents, want one original and one revision", documents)
	}
}
