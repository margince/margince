// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A record hit says where it stands. The search palette shows a deal's stage
// and amount or a lead's status beside the name without a second read.

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/modules/search"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type kielRecords struct {
	contact, company, deal, lead, project ids.UUID
}

func seedKielRecords(t *testing.T, e *SearchEnv) kielRecords {
	t.Helper()
	pipeline := e.SeedID(t, `INSERT INTO pipeline (id, name) VALUES ($1, 'Kiel pipeline')`)
	stage := e.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Negotiation', 1, 'open', 60)`, pipeline)
	touched := time.Date(2026, 3, 2, 9, 0, 0, 0, time.UTC)
	var r kielRecords
	r.contact = e.SeedID(t, `INSERT INTO contact (id, full_name, title, last_activity_at, source, captured_by)
		VALUES ($1, 'Kiel Contact', 'Head of Procurement', $2, 'manual', 'human:x')`, touched)
	r.company = e.SeedID(t, `INSERT INTO company (id, display_name, last_activity_at, source, captured_by)
		VALUES ($1, 'Kiel Werft', $2, 'manual', 'human:x')`, touched)
	r.deal = e.SeedID(t, `INSERT INTO deal (id, name, owner_id, amount_minor, currency, pipeline_id, stage_id, status,
		expected_close_date, source, captured_by)
		VALUES ($1, 'Kiel fleet renewal', $2, 42000000, 'EUR', $3, $4, 'open', DATE '2026-12-12', 'manual', 'human:x')`,
		e.Rep1, pipeline, stage)
	r.lead = e.SeedID(t, `INSERT INTO lead (id, company_name, email, status, source, captured_by)
		VALUES ($1, 'Kiel Robotics', 'demo@kiel.test', 'contacted', 'manual', 'human:x')`)
	r.project = e.SeedID(t, `INSERT INTO project (id, name, key, phase, target_end_date, source, captured_by)
		VALUES ($1, 'Kiel rollout', 'KIEL-1', 'delivering', DATE '2027-01-30', 'manual', 'human:x')`)
	return r
}

func standingsByID(ctx context.Context, t *testing.T, e *SearchEnv) map[ids.UUID]search.Hit {
	t.Helper()
	perType := 5
	page, err := e.Store.Search(ctx, search.Input{Query: "kiel", PerType: &perType})
	if err != nil {
		t.Fatal(err)
	}
	hits := map[ids.UUID]search.Hit{}
	for _, hit := range page.Hits {
		hits[hit.ID] = hit
	}
	return hits
}

func TestARecordHitSaysWhereItStands(t *testing.T) {
	e := SetupSearch(t)
	r := seedKielRecords(t, e)
	hits := standingsByID(searchSeat(e, nil, principal.RowScopeAll, standingGrants()), t, e)

	deal := hits[r.deal].Standing
	if deal == nil || deal.State == nil || *deal.State != "open" || deal.Stage == nil || *deal.Stage != "Negotiation" {
		t.Fatalf("deal standing = %+v, want open in Negotiation", deal)
	}
	if deal.AmountMinor == nil || *deal.AmountMinor != 42000000 || deal.Currency == nil || *deal.Currency != "EUR" {
		t.Errorf("deal money = %v %v, want 42000000 EUR", deal.AmountMinor, deal.Currency)
	}
	if deal.DueOn == nil || deal.DueOn.Format("2006-01-02") != "2026-12-12" {
		t.Errorf("deal due on = %v, want its expected close date", deal.DueOn)
	}
	if deal.MaskedFields != nil {
		t.Errorf("an unmasked reader was told %v is withheld", *deal.MaskedFields)
	}
	contact := hits[r.contact].Standing
	if contact == nil || contact.JobTitle == nil || *contact.JobTitle != "Head of Procurement" || contact.LastActivityAt == nil {
		t.Errorf("contact standing = %+v, want its title and last touch", contact)
	}
	if company := hits[r.company].Standing; company == nil || company.LastActivityAt == nil {
		t.Errorf("company standing = %+v, want its last touch", company)
	}
	if lead := hits[r.lead].Standing; lead == nil || lead.State == nil || *lead.State != "contacted" {
		t.Errorf("lead standing = %+v, want contacted", lead)
	}
	project := hits[r.project].Standing
	if project == nil || project.State == nil || *project.State != "delivering" || project.DueOn == nil {
		t.Errorf("project standing = %+v, want delivering with its target end", project)
	}
}

// A masked amount is withheld and named, so the palette never draws a priced
// deal as unpriced.
func TestADealStandingHonoursTheAmountMask(t *testing.T) {
	e := SetupSearch(t)
	r := seedKielRecords(t, e)
	seat := searchSeat(e, []ids.UUID{e.Team1}, principal.RowScopeTeam, searchReadGrants())
	actor, _ := principal.Actor(seat)
	actor.Permissions.FieldMasks = []principal.FieldMask{
		{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways},
		{Object: "deal", Field: "currency", Condition: principal.MaskAlways},
	}
	hits := standingsByID(principal.WithActor(seat, actor), t, e)

	deal := hits[r.deal].Standing
	if deal == nil {
		t.Fatal("the masked seat lost the deal's standing altogether")
	}
	if deal.AmountMinor != nil || deal.Currency != nil {
		t.Errorf("a masked seat read the deal's money: %v %v", deal.AmountMinor, deal.Currency)
	}
	if deal.MaskedFields == nil || !slices.Equal(*deal.MaskedFields, []string{"amount_minor", "currency"}) {
		t.Errorf("masked fields = %v, want amount_minor and currency", deal.MaskedFields)
	}
	if deal.Stage == nil || *deal.Stage != "Negotiation" {
		t.Errorf("the mask hid the stage too: %+v", deal)
	}
}

// standingGrants reads every type a hit can stand for, projects included.
func standingGrants() map[string]principal.ObjectGrant {
	grants := searchReadGrants()
	grants["project"] = principal.ObjectGrant{Read: true}
	return grants
}
