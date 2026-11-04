// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A reference named for its role is a hop like any other. A deal's partner and
// a company's parent are both companies, and each answers under its own name
// in both directions: `partner_company` and `deals_by_partner_company`,
// `parent_company` and `companies_by_parent_company`. The plain `deals` hop
// beside them still runs on `deal.company_id`.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// roleHopFixture is a partner, a customer, and a holding with one subsidiary.
// Its two deals are one the partner brought in for the customer, and one sold
// to the partner itself.
type roleHopFixture struct {
	partner, customer, holding, subsidiary ids.UUID
	partnerSourced, soldToPartner          ids.UUID
}

func (q *queryEnv) seedRoleHops(t *testing.T) roleHopFixture {
	t.Helper()
	var f roleHopFixture
	company := func(name string, parent any) ids.UUID {
		return q.SeedID(t, `INSERT INTO company (id, owner_id, display_name, parent_company_id, source, captured_by)
			VALUES ($1, $2, $3, $4, 'manual', 'human:x')`, q.Rep1, name, parent)
	}
	f.partner = company("Partner GmbH", nil)
	f.customer = company("Kunde AG", nil)
	f.holding = company("Holding SE", nil)
	f.subsidiary = company("Tochter GmbH", f.holding)

	pipeline := q.SeedID(t, `INSERT INTO pipeline (id, name, is_default, position) VALUES ($1, 'Sales', true, 0)`)
	stage := q.SeedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Qualify', 0, 'open', 10)`, pipeline)
	deal := func(name string, company, partner any, attribution any) ids.UUID {
		return q.SeedID(t, `INSERT INTO deal (id, owner_id, name, pipeline_id, stage_id, company_id,
			partner_company_id, partner_attribution, amount_minor, currency, status, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, 100000, 'EUR', 'open', 'manual', 'human:x')`,
			q.Rep1, name, pipeline, stage, company, partner, attribution)
	}
	f.partnerSourced = deal("Rollout", f.customer, f.partner, "sourced")
	f.soldToPartner = deal("Partner licence", f.partner, nil, nil)
	return f
}

func TestADealIsSelectedByThePartnerThatBroughtItIn(t *testing.T) {
	q := setupQuery(t)
	f := q.seedRoleHops(t)

	byPartner := idSet(q.run(q.admin(), t, `{"version": "v1", "target": "deal",
		"traverse": {"relation": "partner_company",
		             "where": [{"field": "display_name", "op": "eq", "value": "Partner GmbH"}]}}`))
	if !byPartner[f.partnerSourced] || byPartner[f.soldToPartner] || len(byPartner) != 1 {
		t.Errorf("deals brought in by the partner are %v, want only the deal it sourced", byPartner)
	}

	// The same deal name, asked from the company: the role hop answers the
	// partner, the plain one answers the customer, and neither shadows the other.
	for relation, want := range map[string]ids.UUID{"deals_by_partner_company": f.partner, "deals": f.customer} {
		companies := idSet(q.run(q.admin(), t, `{"version": "v1", "target": "company",
			"traverse": {"relation": "`+relation+`",
			             "where": [{"field": "name", "op": "eq", "value": "Rollout"}]}}`))
		if !companies[want] || len(companies) != 1 {
			t.Errorf("companies through %s are %v, want only %s", relation, companies, want)
		}
	}
}

func TestACompanyIsSelectedByItsParentAndItsSubsidiaries(t *testing.T) {
	q := setupQuery(t)
	f := q.seedRoleHops(t)

	subsidiaries := idSet(q.run(q.admin(), t, `{"version": "v1", "target": "company",
		"traverse": {"relation": "parent_company",
		             "where": [{"field": "display_name", "op": "eq", "value": "Holding SE"}]}}`))
	if !subsidiaries[f.subsidiary] || len(subsidiaries) != 1 {
		t.Errorf("the holding's subsidiaries are %v, want only Tochter GmbH", subsidiaries)
	}
	parents := idSet(q.run(q.admin(), t, `{"version": "v1", "target": "company",
		"traverse": {"relation": "companies_by_parent_company",
		             "where": [{"field": "display_name", "op": "eq", "value": "Tochter GmbH"}]}}`))
	if !parents[f.holding] || len(parents) != 1 {
		t.Errorf("the parents of Tochter GmbH are %v, want only the holding", parents)
	}
}
