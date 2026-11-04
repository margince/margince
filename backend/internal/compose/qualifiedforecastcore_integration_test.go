// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The qualified forecast as the default landing while enforced, and the basis
// each new snapshot row freezes. The fixture is setupViability's: a silent
// deal with a far close date that the review excludes, and a supported one.

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

// enforcedViability is the fixture after one check, with enforcement on.
func enforcedViability(t *testing.T) *viabilityEnv {
	t.Helper()
	v := setupViability(t)
	v.check(t)
	if err := assurance.NewStore(InstallationDB(v.Pool)).SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatalf("enforcing: %v", err)
	}
	return v
}

// landing reads the measure and the landing for the close date's quarter, as
// GET /forecast does.
func (v *viabilityEnv) landing(t *testing.T) (forecasting.Landing, forecasting.Readings) {
	t.Helper()
	var landing forecasting.Landing
	var readings forecasting.Readings
	ctx := v.Admin()
	if err := database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, v.closeDate)
		if err != nil {
			return err
		}
		assembly, err := ForecastAssemble(ctx, tx, period, forecasting.Scope{Kind: ScopeKindWorkspace}, v.now, base)
		if err != nil {
			return err
		}
		readings = assembly.Readings
		measure, err := ForecastForwardMeasure(ctx, tx)
		if err != nil {
			return err
		}
		landing, err = forecasting.ProjectLanding(readings, measure, nil)
		return err
	}); err != nil {
		t.Fatalf("reading the landing: %v", err)
	}
	return landing, readings
}

func TestUnsetMeasureLandsQualifiedUnderEnforcement(t *testing.T) {
	v := enforcedViability(t)

	landing, readings := v.landing(t)
	if landing.Measure != forecasting.MeasureQualifiedCommitEvidence || landing.AmountMinor != 100_000 {
		t.Fatalf("with no measure saved the landing is %d on %s, want 100000 on qualified commit "+
			"evidence: the silent deal stays out", landing.AmountMinor, landing.Measure)
	}
	if readings.QualificationMode() != forecasting.QualificationEnforcing {
		t.Errorf("mode = %q, want enforcing", readings.QualificationMode())
	}

	weighted := string(forecasting.MeasureWeighted)
	if err := config.Save(v.Admin(), StoredConfig(v.Pool), identity.ForecastForwardMeasure, weighted); err != nil {
		t.Fatal(err)
	}
	landing, readings = v.landing(t)
	if landing.Measure != forecasting.MeasureWeighted || landing.RemainingMinor != readings.WeightedMinor {
		t.Errorf("a saved weighted measure landed on %s with %d remaining, want weighted with the "+
			"baseline %d: a choice somebody saved is kept", landing.Measure, landing.RemainingMinor, readings.WeightedMinor)
	}
}

func TestAnEnforcedScopeWithNoDealsSendsAZeroQualifiedBlock(t *testing.T) {
	v := enforcedViability(t)
	ctx := v.Admin()
	var readings forecasting.Readings
	if err := database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
		// A quarter long before the fixture's close date holds no deal.
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, v.closeDate.AddDate(-2, 0, 0))
		if err != nil {
			return err
		}
		assembly, err := ForecastAssemble(ctx, tx, period, forecasting.Scope{Kind: ScopeKindWorkspace}, v.now, base)
		readings = assembly.Readings
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if len(readings.Contributions) != 0 {
		t.Fatalf("the earlier quarter holds %d deals, want none", len(readings.Contributions))
	}
	if readings.Qualified == nil || *readings.Qualified != (forecasting.QualifiedReadings{}) {
		t.Fatalf("an enforced empty quarter answered %+v, want a zero qualified block", readings.Qualified)
	}
}

func TestNewContributionFreezesViabilityBasis(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	old := v.freeze(t)
	if err := assurance.NewStore(InstallationDB(v.Pool)).SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatal(err)
	}
	fresh := v.freeze(t)

	if got := v.WsCount(t, `SELECT count(*) FROM forecast_contribution
		WHERE snapshot_id = $1 AND (qualified_policy_version IS NOT NULL OR viability_disposition IS NOT NULL
		  OR qualification_complete IS NOT NULL OR viability_assessed_at IS NOT NULL)`, old); got != 0 {
		t.Errorf("%d rows frozen before enforcement carry a basis, want none", got)
	}
	for deal, want := range map[ids.UUID]string{
		v.silent: forecasting.DispositionOpenReview, v.supported: forecasting.DispositionNone,
	} {
		got := v.WsScalar(t, `SELECT concat_ws('|', qualified_policy_version, viability_disposition,
			qualification_complete::text, (viability_assessed_at IS NOT NULL)::text)
			FROM forecast_contribution WHERE snapshot_id = $1 AND deal_id = $2`, fresh, deal)
		if wantRow := viability.PolicyVersion + "|" + want + "|true|true"; got != wantRow {
			t.Errorf("deal %s froze %q, want %q", deal, got, wantRow)
		}
	}
}

// freeze takes a snapshot of the close date's quarter through the assembly
// every snapshot writer reads.
func (v *viabilityEnv) freeze(t *testing.T) ids.UUID {
	t.Helper()
	store := forecasting.NewStore(InstallationDB(v.Pool))
	var id ids.UUID
	if err := store.InTx(snapshotWriterCtx(v.WS), func(ctx context.Context, tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, v.closeDate)
		if err != nil {
			return err
		}
		scope := forecasting.Scope{Kind: ScopeKindWorkspace}
		assembly, err := ForecastAssemble(ctx, tx, period, scope, v.now, base)
		if err != nil {
			return err
		}
		snapshot := forecasting.NewSnapshot{
			Period: period, Scope: scope, Trigger: forecasting.TriggerRecheck, BaseCurrency: base,
			Readings: assembly.Readings, TakenAt: v.now,
		}
		if err := stampForecastBasis(ctx, tx, &snapshot); err != nil {
			return err
		}
		id, err = store.TakeSnapshot(ctx, tx, snapshot)
		return err
	}); err != nil {
		t.Fatalf("freezing the forecast: %v", err)
	}
	return id
}

// moved is the qualified-evidence movement between two snapshots, read
// through the store every movement surface reads.
func (v *viabilityEnv) moved(t *testing.T, from, to ids.UUID) forecasting.Movement {
	t.Helper()
	out, err := newForecastStoreFor(v.Pool).Movement(v.Admin(), forecasting.ReadingQualifiedEvidence, from, to)
	if err != nil {
		t.Fatalf("reading the movement: %v", err)
	}
	return out
}

func bucketOfDeal(t *testing.T, m forecasting.Movement, deal ids.UUID) string {
	t.Helper()
	for _, d := range m.Deals {
		if d.DealID == deal.String() {
			return d.Bucket
		}
	}
	t.Fatalf("deal %s moved in no bucket: %+v", deal, m.Buckets)
	return ""
}

func TestFrozenReviewChangesMoveTheQualifiedReadingBetweenSnapshots(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	beforeEnforcement := v.freeze(t)
	assurances := assurance.NewStore(InstallationDB(v.Pool))
	if err := assurances.SetQualifiedForecastEnforced(v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatal(err)
	}
	excluded := v.freeze(t)

	if m := v.moved(t, beforeEnforcement, excluded); len(m.Buckets) != 1 || m.Buckets[0].Name != forecasting.BucketDefinition {
		t.Errorf("against a snapshot frozen with no basis the movement is %+v, want only definition", m.Buckets)
	}

	var review ids.UUID
	if err := v.Pool.QueryRow(context.Background(), `SELECT id FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.silent).Scan(&review); err != nil {
		t.Fatal(err)
	}
	until := v.now.AddDate(0, 0, 30)
	if err := assurances.Resolve(v.Admin(), review, assurance.Resolution{
		Outcome: assurance.OutcomeValueCorrect, Reason: "Signed order in the post", ExpiresAt: &until,
	}, v.now); err != nil {
		t.Fatal(err)
	}
	reinstated := v.freeze(t)
	if got := bucketOfDeal(t, v.moved(t, excluded, reinstated), v.silent); got != forecasting.BucketReinstated {
		t.Errorf("the overridden deal moved in %q, want reinstated", got)
	}

	v.WsExec(t, `UPDATE assurance_resolution
		SET created_at = now() - interval '2 days', expires_at = now() - interval '1 minute'
		WHERE exception_id = $1`, review)
	unchecked := v.freeze(t)
	if got := bucketOfDeal(t, v.moved(t, reinstated, unchecked), v.silent); got != forecasting.BucketNotChecked {
		t.Errorf("the deal whose override ended moved in %q, want not_checked", got)
	}
}
