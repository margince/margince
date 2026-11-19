// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package contacts

// open_pipeline is a sum of deal.amount_minor. A reader whose role masks that
// column reads each deal's amount as withheld, so the total must not reach them.
// On a one-open-deal account the total is that one deal's value. This proves the
// masked reader is withheld the figure and the unmasked reader still receives
// it. The open-deal count, which is not that money, stays visible to both.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func TestCompanyOpenPipelineWithholdsMaskedDealMoney(t *testing.T) {
	e := setupDedupe(t)
	frozen := time.Date(2026, 6, 2, 12, 0, 0, 0, time.UTC)
	original := rollupClock
	t.Cleanup(func() { rollupClock = original })
	rollupClock = func() time.Time { return frozen }
	e.store.SetClock(rollupClock)

	companies := seedRollupCountCompanies(t, e)
	companyID := ids.From[ids.CompanyKind](companies[0])
	const dealAmount = int64(4_850_000)
	seedOnePricedOpenDeal(t, e, companies[0], dealAmount)

	openPipelineOf := func(ctx context.Context) crmcontracts.ComputedField {
		t.Helper()
		company, err := e.store.GetCompany(ctx, companyID, storekit.LiveOnly)
		if err != nil {
			t.Fatal(err)
		}
		if company.OpenDealCount == nil || *company.OpenDealCount != 1 {
			t.Fatalf("open_deal_count = %v, want 1 — the count is not the masked money and must stay", company.OpenDealCount)
		}
		if company.ComputedFields == nil {
			t.Fatal("computed_fields absent for a reader holding deal + computed_field read")
		}
		for _, field := range *company.ComputedFields {
			if field.Key == "open_pipeline" {
				return field
			}
		}
		t.Fatal("computed fields lack open_pipeline")
		return crmcontracts.ComputedField{}
	}

	// Positive control: without the mask the same account reports the real sum.
	// If this stops holding, the test below could pass for the wrong reason.
	unmasked := openPipelineOf(pipelineReader(e, nil))
	if !unmasked.Computable || unmasked.ValueMinor == nil || *unmasked.ValueMinor != dealAmount {
		t.Fatalf("control: unmasked open_pipeline = %+v, want computable %d", unmasked, dealAmount)
	}

	masked := openPipelineOf(pipelineReader(e, []principal.FieldMask{
		{Object: "deal", Field: "amount_minor", Condition: principal.MaskAlways},
	}))
	if masked.Computable {
		t.Fatalf("open_pipeline was computable for a reader who masks deal.amount_minor: %+v", masked)
	}
	if masked.Reason == nil || *masked.Reason != amountMaskedReason {
		t.Fatalf("open_pipeline.reason = %v, want %q", masked.Reason, amountMaskedReason)
	}
	if masked.ValueMinor != nil {
		t.Fatalf("open_pipeline.value_minor = %v, want nil — the masked deal money must not reach the wire", masked.ValueMinor)
	}
}

// pipelineReader holds the two grants open_pipeline needs, deal and
// computed_field read, plus any field masks the case adds. The scope is "own",
// not "all". An all-scope reader is unbounded and reads every column, so a mask
// only bites a bounded seat. That is the finding's attacker: a rep who still
// reads the colleague's company because customer identity is workspace-readable.
func pipelineReader(e *dedupeEnv, masks []principal.FieldMask) context.Context {
	return principal.WithActor(e.as(), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:" + e.rep.String(), UserID: e.rep,
		Permissions: principal.Permissions{
			RowScope: principal.RowScopeOwn,
			Objects: map[string]principal.ObjectGrant{
				"company":        {Read: true},
				"deal":           {Read: true},
				"computed_field": {Read: true},
			},
			FieldMasks: masks,
		},
	})
}

// seedOnePricedOpenDeal attaches one open deal, priced in the installation base
// currency, to the company. A deal in the base currency converts at parity, so
// the rollup publishes amountMinor as the total and needs no fx_rate row. The
// migrated module template carries no installation settings, so the
// base-currency row is seeded here too.
func seedOnePricedOpenDeal(t *testing.T, e *dedupeEnv, company ids.UUID, amountMinor int64) {
	t.Helper()
	const base = "EUR"
	ctx := e.as()
	pipeline, stage := ids.NewV7(), ids.NewV7()
	if err := e.store.tx(ctx, func(tx pgx.Tx) error {
		for _, seed := range []struct {
			sql  string
			args []any
		}{
			{`INSERT INTO setting (key, value) VALUES ('installation.base_currency', $1::jsonb)
			  ON CONFLICT (key) DO UPDATE SET value = excluded.value`, []any{`"` + base + `"`}},
			{`INSERT INTO pipeline (id, name) VALUES ($1, 'Sales')`, []any{pipeline}},
			{`INSERT INTO stage (id, pipeline_id, name, position) VALUES ($1, $2, 'Qualify', 10)`, []any{stage, pipeline}},
			{
				`INSERT INTO deal (id, name, amount_minor, currency, pipeline_id, stage_id, company_id, source, captured_by)
			  VALUES ($1, 'Rollout', $2, $3, $4, $5, $6, 'manual', 'human:test')`,
				[]any{ids.NewV7(), amountMinor, base, pipeline, stage, company},
			},
		} {
			if _, err := tx.Exec(ctx, seed.sql, seed.args...); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatalf("seed a priced open deal: %v", err)
	}
}
