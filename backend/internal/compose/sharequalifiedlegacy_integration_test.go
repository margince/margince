// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A share of a snapshot frozen under the retired qualified forecast lands on
// the number that forecast stated. That is won plus the open money of the
// deals frozen as eligible, over the rows the reader may see. The landing
// names the measure the snapshot was frozen under.

import (
	"context"
	"encoding/json"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// qualifiedDeal is one seeded deal and the qualification the old writer froze
// on it; an empty qualification is a row it froze none on.
type qualifiedDeal struct {
	name          string
	owner         ids.UUID
	probability   int
	amount        int64
	won           bool
	qualification string
}

// The amounts are powers of two times a million, so every sum names its rows.
func qualifiedQuarter(e *forecastEnv) []qualifiedDeal {
	return []qualifiedDeal{
		{name: "Won", owner: e.Rep1, probability: 95, amount: 2_000_000, won: true},
		{name: "Eligible", owner: e.Rep1, probability: 95, amount: 1_000_000, qualification: "eligible"},
		{name: "Eligible below Likely", owner: e.Rep1, probability: 60, amount: 500_000, qualification: "eligible"},
		{name: "Excluded", owner: e.Rep1, probability: 95, amount: 4_000_000, qualification: "excluded"},
		{name: "Unsupported", owner: e.Rep1, probability: 95, amount: 8_000_000, qualification: "unsupported"},
		{name: "Unknown", owner: e.Rep1, probability: 95, amount: 16_000_000, qualification: "unknown"},
		{name: "Other team's eligible", owner: e.Rep3, probability: 95, amount: 32_000_000, qualification: "eligible"},
	}
}

func TestASnapshotFrozenUnderTheQualifiedForecastLandsOnItsEligibleDeals(t *testing.T) {
	e := setupForecast(t)
	deals := qualifiedQuarter(e)
	e.seedQualifiedQuarter(t, deals)
	current := e.freezeMeasured(t)
	company, teamLens := e.shareReaders()
	masked := e.forecastReader(e.maskedDealReader(principal.MaskAlways))

	type want struct{ won, remaining int64 }
	for _, tc := range []struct {
		stored, policy string
		reader         string
		ctx            context.Context
		measure        crmcontracts.ForecastLandingMeasure
		want           want
	}{
		// Likely counts the eligible deals at or above the threshold.
		{"qualified_commit_evidence", "v2", "company", company, "qualified_likely", want{2_000_000, 33_000_000}},
		{"qualified_likely", "v2", "company", company, "qualified_likely", want{2_000_000, 33_000_000}},
		// Weighted counts every eligible open deal at its chance: 95% and 60%.
		{"qualified_weighted", "v2", "company", company, "qualified_weighted", want{2_000_000, 31_650_000}},
		{"qualified_likely", "v2", "team", teamLens, "qualified_likely", want{2_000_000, 1_000_000}},
		{"qualified_weighted", "v2", "team", teamLens, "qualified_weighted", want{2_000_000, 1_250_000}},
		{"qualified_likely", "v2", "masked", masked, "qualified_likely", want{0, 0}},
		{"qualified_weighted", "v2", "masked", masked, "qualified_weighted", want{0, 0}},
		// With the policy off the qualified forecast landed on its baseline.
		{"qualified_likely", "", "company", company, "likely", want{2_000_000, 61_000_000}},
	} {
		legacy := e.freezeQualifiedCopy(t, current, tc.stored, tc.policy, deals)
		landing := e.openShare(tc.ctx, t, e.issueWorkspaceShare(t, shareKindSnapshot, &legacy)).Readings.Landing
		if landing == nil || landing.Measure != tc.measure || landing.WonMinor != tc.want.won ||
			landing.RemainingMinor != tc.want.remaining || landing.AmountMinor != tc.want.won+tc.want.remaining {
			t.Errorf("frozen under %q with policy %q, the %s reader's share lands %+v, want %s at won %d + %d",
				tc.stored, tc.policy, tc.reader, landing, tc.measure, tc.want.won, tc.want.remaining)
		}
	}
}

// seedQualifiedQuarter plants the deals, each closing today, on a stage of its
// own chance. The won one closed today at the base currency's own rate.
func (e *forecastEnv) seedQualifiedQuarter(t *testing.T, deals []qualifiedDeal) {
	t.Helper()
	stages := map[int]ids.UUID{}
	for _, deal := range deals {
		if _, ok := stages[deal.probability]; !ok {
			stages[deal.probability] = e.seedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
				VALUES ($1, $2, $3, $4, 'open', $5)`, e.pipeline, deal.name, 10+len(stages), deal.probability)
		}
		status := "open"
		if deal.won {
			status = "won"
		}
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency,
				expected_close_date, status, closed_at, fx_rate_to_base, fx_rate_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, 'EUR', $7, $8, CASE WHEN $8::text = 'won' THEN now() END,
				CASE WHEN $8::text = 'won' THEN 1 END, CASE WHEN $8::text = 'won' THEN $7::date END, 'manual', 'human:x')`,
			deal.name, e.pipeline, stages[deal.probability], deal.owner, deal.amount, e.forecastToday(t), status)
	}
}

// freezeQualifiedCopy copies a snapshot the current writer took. The copy
// sets the columns the removed qualified writer set: the measure, the policy,
// and each row's qualification and review standing. A frozen row refuses an UPDATE, so
// the copy is a new snapshot, taken on a recheck to keep the daily slot free.
func (e *forecastEnv) freezeQualifiedCopy(
	t *testing.T, from ids.UUID, measure, policy string, deals []qualifiedDeal,
) ids.UUID {
	t.Helper()
	qualifications := map[string]string{}
	for _, deal := range deals {
		if policy != "" && deal.qualification != "" {
			qualifications[deal.name] = deal.qualification
		}
	}
	byName, err := json.Marshal(qualifications)
	if err != nil {
		t.Fatal(err)
	}
	var id ids.UUID
	if err := e.owner.QueryRow(context.Background(), `
		WITH frozen AS (
		  INSERT INTO forecast_snapshot
		  SELECT (jsonb_populate_record(NULL::forecast_snapshot, to_jsonb(s) || jsonb_build_object(
		    'id', uuidv7(), 'trigger', 'recheck', 'forward_measure', $2::text, 'landing_measure', $2::text,
		    'qualified_policy_version', NULLIF($3::text, '')))).*
		    FROM forecast_snapshot s WHERE s.id = $1
		  RETURNING id
		), copied AS (
		  INSERT INTO forecast_contribution
		  SELECT (jsonb_populate_record(NULL::forecast_contribution, to_jsonb(c) || jsonb_build_object(
		    'id', uuidv7(), 'snapshot_id', frozen.id,
		    'qualification', q.qualification,
		    'qualified_policy_version', CASE WHEN q.qualification IS NOT NULL THEN $3::text END,
		    'viability_disposition', CASE WHEN q.qualification = 'excluded' THEN 'open_review'
		      WHEN q.qualification = 'unsupported' THEN 'unsupported'
		      WHEN q.qualification IS NOT NULL THEN 'none' END,
		    'qualification_complete', CASE WHEN q.qualification IS NOT NULL THEN q.qualification <> 'unknown' END))).*
		    FROM forecast_contribution c JOIN deal d ON d.id = c.deal_id CROSS JOIN frozen
		    CROSS JOIN LATERAL (SELECT $4::jsonb ->> d.name AS qualification) q
		   WHERE c.snapshot_id = $1
		  RETURNING 1
		)
		SELECT id FROM frozen WHERE (SELECT count(*) FROM copied) = $5`,
		from, measure, policy, string(byName), len(deals)).Scan(&id); err != nil {
		t.Fatalf("freezing a %s snapshot: %v", measure, err)
	}
	return id
}
