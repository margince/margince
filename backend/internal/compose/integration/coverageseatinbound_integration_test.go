// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A seat on a deal says when the contact last wrote to us. It is read through
// the contact page's strength fold, and only for a caller who may read activities.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestADealSeatCarriesWhenTheContactLastWroteAsADay(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)
	// A message they sent, logged by its date alone: the server keeps it at noon UTC.
	logInboundDay(t, e, buyer, time.Date(2026, 5, 20, 16, 30, 0, 0, time.UTC))

	granted := readDealCoverage(t, e, coverageReaderPerms(true), dealID)
	if len(granted.Stakeholders) != 1 {
		t.Fatalf("the coverage page seated %d contacts, want the champion", len(granted.Stakeholders))
	}
	seat := granted.Stakeholders[0]
	want := time.Date(2026, 5, 20, 12, 0, 0, 0, time.UTC)
	if seat.LastInboundAt == nil || !seat.LastInboundAt.Equal(want) ||
		seat.LastInboundPrecision == nil || *seat.LastInboundPrecision != "day" {
		t.Errorf("the seat went out as %+v, want it last wrote on %v as a day", seat, want)
	}

	blind := coverageReaderPerms(true)
	delete(blind.Objects, "activity")
	refused := readDealCoverage(t, e, blind, dealID)
	if len(refused.Stakeholders) != 1 {
		t.Fatalf("a caller without the activity grant lost the seat itself: %d seats", len(refused.Stakeholders))
	}
	if seat := refused.Stakeholders[0]; seat.LastInboundAt != nil || seat.LastInboundPrecision != nil {
		t.Errorf("a caller without the activity grant was told when the contact wrote: %+v", seat)
	}
}

// The status card, the assistant and the intro paths read CoverageFor and show
// no seat dates. So it leaves them unread, and none of them pays for that read.
func TestCoverageForLeavesTheSeatDatesToTheCoveragePage(t *testing.T) {
	e := Setup(t)
	buyer := e.SeedContact(t, "Mai Tran", nil)
	dealID := seatedDeal(t, e, buyer)
	logInboundDay(t, e, buyer, time.Date(2026, 5, 20, 16, 30, 0, 0, time.UTC))

	coverage := coverageFor(t, e, coverageReaderPerms(true), dealID, time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC))
	if len(coverage.Stakeholders) != 1 {
		t.Fatalf("CoverageFor seated %d contacts, want the champion", len(coverage.Stakeholders))
	}
	if coverage.SeatsInbound != nil {
		t.Errorf("CoverageFor read the seat dates only the coverage page shows: %+v", coverage.SeatsInbound)
	}
}

// readDealCoverage drives the real coverage handler as a reader holding perms.
func readDealCoverage(
	t *testing.T, e *Env, perms principal.Permissions, dealID ids.DealID,
) crmcontracts.DealCoverage {
	t.Helper()
	ctx := e.As(e.Rep1, []ids.UUID{e.Team1}, perms)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/v1/deals/"+dealID.String()+"/coverage", nil).WithContext(ctx)
	compose.NewContactGraphReads(e.Pool, e.DB()).GetDealCoverage(rec, req, crmcontracts.Id(dealID.UUID))
	if rec.Code != http.StatusOK {
		t.Fatalf("the coverage page answered %d: %s", rec.Code, rec.Body.String())
	}
	var out crmcontracts.DealCoverage
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("decoding the coverage page: %v", err)
	}
	return out
}

// seatedDeal opens a deal with the contact seated on it as its champion.
func seatedDeal(t *testing.T, e *Env, buyer ids.UUID) ids.DealID {
	t.Helper()
	pipeline, open := pipelineFixtureFor(e.Admin(), t, e.Deals)
	deal, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
		Name: "Harbour renewal", PipelineID: pipeline, StageID: open, Source: "manual",
	})
	if err != nil {
		t.Fatalf("creating the deal: %v", err)
	}
	dealID := ids.From[ids.DealKind](ids.UUID(deal.Id))
	contactID, role := ids.From[ids.ContactKind](buyer), "champion"
	if _, err := e.Contacts.CreateRelationship(e.Admin(), contacts.CreateRelationshipInput{
		Kind: "deal_stakeholder", ContactID: &contactID, DealID: &dealID,
		Role: &role, Source: "manual",
	}); err != nil {
		t.Fatalf("seating the contact: %v", err)
	}
	return dealID
}

// logInboundDay logs one message the contact sent, dated by its day alone.
func logInboundDay(t *testing.T, e *Env, buyer ids.UUID, sentOn time.Time) {
	t.Helper()
	subject := "Re: renewal terms"
	if _, _, err := e.Activities.LogActivity(e.Admin(), activities.LogActivityInput{
		Kind: "email", Subject: &subject, Direction: new("inbound"), Source: "manual",
		OccurredAt: &sentOn, OccurredPrecision: activities.PrecisionDay,
		Links: []activities.ActivityLinkInput{{EntityType: "contact", EntityID: buyer}},
	}); err != nil {
		t.Fatalf("logging the message they sent: %v", err)
	}
}
