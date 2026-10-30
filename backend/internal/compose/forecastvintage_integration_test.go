// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/viability"
)

func TestAVintageRecordsTheBasisItWasTakenUnder(t *testing.T) {
	e := setupEvaluation(t)
	before := wholeWorkspace(e.dataset(t))[1]
	if before.Measure != string(forecasting.MeasureCommitEvidence) || before.Policy != "" {
		t.Fatalf("with qualification off the vintage says measure %q policy %q", before.Measure, before.Policy)
	}
	sweep := &assuranceSweepWorker{
		pool: e.Pool, now: func() time.Time { return e.second }, log: slog.New(slog.DiscardHandler),
		activities: activities.NewStore(InstallationDB(e.Pool)),
	}
	if err := (&assuranceRunWorker{sweep: sweep}).Work(context.Background(), &river.Job[AssuranceRunArgs]{
		Args: AssuranceRunArgs{Workspace: e.WS, RequestedBy: e.Rep1.String()},
	}); err != nil {
		t.Fatalf("the input check enforcement waits on: %v", err)
	}
	if err := assurance.NewStore(InstallationDB(e.Pool)).SetQualifiedForecastEnforced(
		e.Admin(), StoredConfig(e.Pool), true); err != nil {
		t.Fatalf("enforcing: %v", err)
	}
	e.freeze(t, e.second.AddDate(0, 0, 1))
	vintages := wholeWorkspace(e.dataset(t))
	if last := vintages[len(vintages)-1]; last.Policy != viability.PolicyVersion {
		t.Fatalf("a vintage taken under enforcement says policy %q, want %q", last.Policy, viability.PolicyVersion)
	}
}

// The week's close is the second snapshot writer, and it stamps the same
// basis. Its snapshot records a period already over, so it is no prediction.
func TestAWeeksCloseStampsTheBasisAndIsNoPrediction(t *testing.T) {
	e := setupEvaluation(t)
	weekStart := e.quarter.Start.AddDate(0, 0, 7*4)
	ctx := e.As(e.Rep1, nil, integration.AdminPerms)
	if err := newForecastStoreFor(e.Pool).InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		_, _, _, err := NewWeeklyForecast(newForecastStoreFor(e.Pool)).CloseWeek(ctx, tx, weekStart, weekStart.AddDate(0, 0, 7))
		return err
	}); err != nil {
		t.Fatalf("closing the week: %v", err)
	}
	var measure, trigger string
	var localDay, periodEnd time.Time
	if err := integration.OwnerConn(t).QueryRow(context.Background(), `
		SELECT COALESCE(forward_measure, ''), trigger, local_day, period_end FROM forecast_snapshot
		 WHERE trigger = $1 ORDER BY period_end LIMIT 1`, forecasting.TriggerPeriodClose).
		Scan(&measure, &trigger, &localDay, &periodEnd); err != nil {
		t.Fatalf("reading the week's closing snapshot: %v", err)
	}
	if measure != string(forecasting.MeasureCommitEvidence) {
		t.Fatalf("the week's closing snapshot says measure %q", measure)
	}
	if kind := forecasting.PredictionKindOf(trigger, localDay, periodEnd); kind != forecasting.PredictionAfterOutcome {
		t.Fatalf("a week's closing snapshot reads as %s", kind)
	}
}

// The dataset read takes every deal id the snapshots froze, under no row
// scope, so only the evaluation's own pass may run it.
func TestOnlyTheEvaluationPassReadsTheDataset(t *testing.T) {
	e := setupEvaluation(t)
	store := newForecastStoreFor(e.Pool)
	read := func(ctx context.Context) error {
		return store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
			_, err := store.EvaluationVintagesTx(ctx, tx, evaluationBounds.Vintages, evaluationBounds.Deals)
			return err
		})
	}
	if err := read(e.Admin()); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("an admin seat read the evaluation dataset: %v", err)
	}
	other := principal.SystemActing(principal.WithWorkspaceID(context.Background(), e.WS), forecastSnapshotActor)
	if err := read(other); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Fatalf("another system pass read the evaluation dataset: %v", err)
	}
}

// With individual performance off, a vintage whose deals belong to one seat
// is left out. A vintage below the deal floor is left out either way.
func TestAVintageOfOneSeatOrFewDealsIsLeftOut(t *testing.T) {
	e := setupEvaluation(t)
	e.WsExec(t, `UPDATE deal SET owner_id = $1`, e.Rep1)
	e.freeze(t, e.second.AddDate(0, 0, 2))
	ctx := principal.WithActor(principal.WithWorkspaceID(context.Background(), e.WS), principal.Principal{
		Type: principal.PrincipalHuman, ID: "human:settings-admin", UserID: ids.NewV7(),
		Permissions: principal.Permissions{
			Objects:  map[string]principal.ObjectGrant{"installation_settings": {Read: true, Update: true}},
			RowScope: principal.RowScopeAll,
		},
	})
	if err := config.Save(ctx, StoredConfig(e.Pool), identity.IndividualPerformance, false); err != nil {
		t.Fatalf("switching individual performance off: %v", err)
	}
	dataset := e.dataset(t)
	left := map[string]int{}
	for _, l := range dataset.LeftOut {
		left[l.Reason] = l.Vintages
	}
	if left[forecasting.ExcludedIndividualOff] == 0 {
		t.Fatalf("a one-seat vintage stayed in with individual performance off: %+v", dataset.LeftOut)
	}
	for _, v := range wholeWorkspace(dataset) {
		if v.TakenAt.After(e.second.AddDate(0, 0, 1)) {
			t.Fatal("the one-seat vintage reached the dataset")
		}
	}

	few := setupEvaluation(t)
	few.WsExec(t, `UPDATE deal SET archived_at = now() WHERE id = ANY($1)`, few.deals[1:])
	few.freeze(t, few.second.AddDate(0, 0, 2))
	found := false
	for _, l := range few.dataset(t).LeftOut {
		found = found || (l.Reason == forecasting.ExcludedBelowFloor && l.Vintages > 0)
	}
	if !found {
		t.Fatal("a one-deal vintage stayed in the dataset")
	}
}

// A frozen snapshot refuses an update to anything but its call link, a
// contribution refuses every update, and both can still be deleted.
func TestAFrozenForecastCannotBeRewritten(t *testing.T) {
	e := setupEvaluation(t)
	owner := integration.OwnerConn(t)
	var snapshot ids.UUID
	if err := owner.QueryRow(context.Background(), `SELECT id FROM forecast_snapshot ORDER BY taken_at LIMIT 1`).Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	for _, statement := range []string{
		`UPDATE forecast_snapshot SET won_minor = won_minor + 1 WHERE id = $1`,
		`UPDATE forecast_contribution SET stage_probability = 100 WHERE snapshot_id = $1`,
	} {
		if err := e.WsExecErr(t, statement, snapshot); err == nil {
			t.Fatalf("%q rewrote a frozen forecast", statement)
		}
	}
	e.WsExec(t, `UPDATE forecast_snapshot SET call_id = NULL WHERE id = $1`, snapshot)
	e.WsExec(t, `DELETE FROM forecast_snapshot WHERE id = $1`, snapshot)
	if left := e.WsCount(t, `SELECT count(*) FROM forecast_contribution WHERE snapshot_id = $1`, snapshot); left != 0 {
		t.Fatalf("deleting a snapshot left %d contribution rows", left)
	}
}
