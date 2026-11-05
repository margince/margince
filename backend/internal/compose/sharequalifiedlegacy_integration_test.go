// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A share of a snapshot frozen under the retired qualified forecast lands on
// the number that forecast stated. That is won plus the open money of the
// deals frozen as still supported, named by the measure it was frozen under.

import (
	"context"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	supportedDealMinor   = 1_000_000
	unsupportedDealMinor = 4_000_000
)

func TestASnapshotFrozenUnderTheQualifiedForecastLandsOnItsSupportedDeals(t *testing.T) {
	e := setupForecast(t)
	likely := e.seedID(t, `INSERT INTO stage (id, pipeline_id, name, position, semantic, win_probability)
		VALUES ($1, $2, 'Signing', 3, 'open', 95)`, e.pipeline)
	for name, amount := range map[string]int64{"Supported": supportedDealMinor, "Unsupported": unsupportedDealMinor} {
		e.seedID(t, `INSERT INTO deal (id, name, pipeline_id, stage_id, owner_id, amount_minor, currency,
				expected_close_date, source, captured_by)
			VALUES ($1, $2, $3, $4, $5, $6, 'EUR', $7, 'manual', 'human:x')`,
			name, e.pipeline, likely, e.Rep1, amount, e.forecastToday(t))
	}
	current := e.freezeMeasured(t)
	supportedEvidence, supportedWeighted := e.frozenMoneyOf(t, current, "Supported")
	if supportedEvidence != supportedDealMinor {
		t.Fatalf("the supported deal froze %d of Likely money, want %d", supportedEvidence, supportedDealMinor)
	}
	company, _ := e.shareReaders()

	for _, tc := range []struct {
		stored, policy string
		want           crmcontracts.ForecastLandingMeasure
		amount         int64
	}{
		{"qualified_commit_evidence", "v2", "qualified_likely", supportedEvidence},
		{"qualified_likely", "v2", "qualified_likely", supportedEvidence},
		{"qualified_weighted", "v2", "qualified_weighted", supportedWeighted},
		// With the policy off the qualified forecast landed on its baseline.
		{"qualified_likely", "", "likely", supportedDealMinor + unsupportedDealMinor},
	} {
		legacy := e.freezeQualifiedCopy(t, current, tc.stored, tc.policy)
		landing := e.openShare(company, t, e.issueWorkspaceShare(t, shareKindSnapshot, &legacy)).Readings.Landing
		if landing == nil || landing.Measure != tc.want || landing.AmountMinor != tc.amount {
			t.Errorf("frozen under %q with policy %q: the share lands %+v, want %s at %d",
				tc.stored, tc.policy, landing, tc.want, tc.amount)
		}
	}
}

// frozenMoneyOf is one deal's frozen Likely and weighted money.
func (e *forecastEnv) frozenMoneyOf(t *testing.T, snapshot ids.UUID, deal string) (evidence, weighted int64) {
	t.Helper()
	if err := e.owner.QueryRow(context.Background(), `
		SELECT CASE WHEN c.in_evidence THEN c.base_minor ELSE 0 END, c.weighted_minor
		  FROM forecast_contribution c JOIN deal d ON d.id = c.deal_id
		 WHERE c.snapshot_id = $1 AND d.name = $2`, snapshot, deal).Scan(&evidence, &weighted); err != nil {
		t.Fatalf("reading the frozen money of %s: %v", deal, err)
	}
	return evidence, weighted
}

// freezeQualifiedCopy writes a snapshot as the qualified forecast's writer
// froze it, from one the current writer took. That writer no longer exists, so
// the rows are inserted with the columns it set. The measure goes in both
// measure columns, the policy on the snapshot and each row, and each row gets
// its qualification. A frozen row refuses an UPDATE, so the copy is a new
// snapshot. It is taken on a recheck, so it does not claim the daily slot.
func (e *forecastEnv) freezeQualifiedCopy(t *testing.T, from ids.UUID, measure, policy string) ids.UUID {
	t.Helper()
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
		    'qualification', CASE WHEN $3::text = '' THEN NULL WHEN d.name = 'Supported' THEN 'eligible' ELSE 'excluded' END,
		    'qualified_policy_version', NULLIF($3::text, ''),
		    'viability_disposition', CASE WHEN $3::text = '' THEN NULL WHEN d.name = 'Supported' THEN 'none' ELSE 'open_review' END,
		    'qualification_complete', CASE WHEN $3::text = '' THEN NULL ELSE true END))).*
		    FROM forecast_contribution c JOIN deal d ON d.id = c.deal_id CROSS JOIN frozen
		   WHERE c.snapshot_id = $1
		  RETURNING 1
		)
		SELECT id FROM frozen WHERE (SELECT count(*) FROM copied) = 2`, from, measure, policy).Scan(&id); err != nil {
		t.Fatalf("freezing a %s snapshot: %v", measure, err)
	}
	return id
}
