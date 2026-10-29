// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// Somebody who answered the retired buyer-silent check about a deal is not
// asked again about it until that answer runs out. The answer is given through
// the real Resolve, on a finding raised the way the retired rule raised it.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// answerEarlierCheck raises a buyer-silent finding on the silent deal and has
// an admin answer it as correct until the given day.
func (v *viabilityEnv) answerEarlierCheck(t *testing.T, until time.Time) {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(context.Background(), v.WS), assuranceActor)
	store := assurance.NewStore(InstallationDB(v.Pool))
	if err := store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return store.UpsertException(ctx, tx, assurance.Finding{
			SubjectKind: "deal", Type: assurance.TypeBuyerSilent,
			SubjectID: v.silent.String(), Severity: assurance.SeverityMedium,
		}, "")
	}); err != nil {
		t.Fatalf("raising the earlier finding: %v", err)
	}
	finding, err := ids.Parse(v.WsScalar(t, `SELECT id::text FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeBuyerSilent, v.silent))
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Resolve(v.Admin(), finding, assurance.Resolution{
		Outcome: assurance.OutcomeValueCorrect, Reason: "The buyer is on parental leave", ExpiresAt: &until,
	}, v.now); err != nil {
		t.Fatalf("answering the earlier check: %v", err)
	}
}

func TestAnEarlierAnswerIsHonouredUntilItRunsOut(t *testing.T) {
	v := setupViability(t)
	until := v.now.AddDate(0, 0, 30)
	v.answerEarlierCheck(t, until)

	v.check(t)
	if v.openReviews(t, v.silent) != 0 {
		t.Fatalf("a deal somebody answered got a fresh review before the answer ran out")
	}
	if outcome, reasons := v.receiptOf(t, v.silent); outcome != "waiting" || reasons != "earlier_answer" {
		t.Fatalf("receipt %s %q, want waiting on the earlier answer", outcome, reasons)
	}
	if err := assurance.NewStore(InstallationDB(v.Pool)).SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatal(err)
	}
	if q := v.forecast(t).Qualified; q == nil || q.ExcludedCount != 0 || q.UnknownCount != 0 {
		t.Fatalf("qualified readings %+v, want the answered deal counted", q)
	}

	later := until.Add(time.Hour)
	v.worker.now = func() time.Time { return later }
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE capture_sync_state SET last_success_at = $1`, later.Add(-3*time.Hour)); err != nil {
		t.Fatal(err)
	}
	v.check(t)
	if v.openReviews(t, v.silent) != 1 {
		t.Fatalf("once the answer ran out the silent deal got no review")
	}
}
