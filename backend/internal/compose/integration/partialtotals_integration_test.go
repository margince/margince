// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Totals outside the report engine add the deals that convert. Each says which
// deals it left out. These run the account coverage view and the team change
// report through their own read paths.

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAnAccountsOpenAmountAddsTheDealsThatConvertAndCountsTheRest(t *testing.T) {
	for _, tc := range []struct {
		name     string
		deals    []partialSumDeal
		amount   *int64
		noRate   int
		currency []string
	}{
		{"mixed", []partialSumDeal{{10_000, "EUR"}, {20_000, "EUR"}, {5_000, "CHF"}}, new(int64(30_000)), 1, []string{"CHF"}},
		{"all without a rate", []partialSumDeal{{5_000, "CHF"}, {7_000, "CHF"}}, nil, 2, []string{"CHF"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			d := newTwoDoors(t, "account-partial", "read")
			stages := apptest.DiscoverSeededPipeline(t, d.e)
			company := createdID(t, d.e, "/v1/companies", AnyMap{"display_name": "Partial Account", "source": "manual"})
			for _, deal := range tc.deals {
				createdID(t, d.e, "/v1/deals", AnyMap{
					"name": "Deal", "pipeline_id": stages.PipelineID, "stage_id": stages.Open, "company_id": company,
					"source": "manual", "amount_minor": deal.amount, "currency": deal.currency,
				})
			}
			var out struct {
				Rows []crmcontracts.AccountCoverageRow `json:"rows"`
			}
			if err := json.Unmarshal(d.rest(t, "GET", "/v1/analytics/account-coverage?view=untouched", nil), &out); err != nil {
				t.Fatal(err)
			}
			if len(out.Rows) != 1 {
				t.Fatalf("rows = %+v, want the one account", out.Rows)
			}
			row := out.Rows[0]
			if (row.OpenAmountMinor == nil) != (tc.amount == nil) || (tc.amount != nil && *row.OpenAmountMinor != *tc.amount) {
				t.Errorf("open amount = %v, want %v", row.OpenAmountMinor, tc.amount)
			}
			c := row.OpenAmountCoverage
			if c == nil || !c.Partial || c.NoRateDeals != tc.noRate || strings.Join(c.MissingCurrencies, ",") != strings.Join(tc.currency, ",") {
				t.Errorf("coverage = %+v, want partial with %d deals with no rate in %v", c, tc.noRate, tc.currency)
			}
		})
	}
}

func TestATeamChangeGroupAddsTheDealsThatConvertAndCountsTheRest(t *testing.T) {
	for _, tc := range []struct {
		name            string
		deals           []partialSumDeal
		value           int64
		noValue, noRate int
	}{
		{"mixed", []partialSumDeal{{10_000, "EUR"}, {5_000, "CHF"}, {0, ""}}, 10_000, 1, 1},
		{"all without a rate", []partialSumDeal{{5_000, "CHF"}, {7_000, "CHF"}}, 0, 0, 2},
	} {
		t.Run(tc.name, func(t *testing.T) {
			e := Setup(t)
			pipeline, open, _ := DealFixture(t, e)
			for _, deal := range tc.deals {
				id := ids.NewV7()
				var amount, currency any
				if deal.currency != "" {
					amount, currency = deal.amount, deal.currency
				}
				e.WsExec(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency, source, captured_by)
					VALUES ($1, 'Changed', $2, $3, $4, $5, $6, 'manual', 'human:x')`, id, pipeline, open, e.Rep1, amount, currency)
				audit := ids.NewV7()
				e.WsExec(t, `INSERT INTO audit_log (id, actor_type, actor_id, action, entity_type, entity_id)
					VALUES ($1, 'agent', 'agent:x', 'update', 'deal', $2)`, audit, id)
				e.WsExec(t, `INSERT INTO deal_correction (deal_id, audit_log_id, correction, fields, remaining_open_stages, asking, standing_close_date, captured_by)
					VALUES ($1, $2, $3, ARRAY['amount_minor'], '1', '1', '2026-01-01', 'agent:x')`, id, audit, deals.VerdictAmountCorrection)
			}
			report, err := e.Deals.TeamChanges(e.Admin(), []ids.UUID{e.Rep1}, clocktest.Now(t).Add(-24*time.Hour))
			if err != nil || len(report.Groups) != 1 {
				t.Fatalf("groups = %+v, %v, want the one amount group", report.Groups, err)
			}
			g := report.Groups[0]
			if g.ValueBaseMinor != tc.value || g.ValueNoAmount != tc.noValue || g.ValueNoRate != tc.noRate || (tc.noRate > 0 && g.ValueMissingCurrencies != "CHF") {
				t.Errorf("group = %+v, want value %d with %d without an amount and %d without a rate in CHF", g, tc.value, tc.noValue, tc.noRate)
			}
		})
	}
}
