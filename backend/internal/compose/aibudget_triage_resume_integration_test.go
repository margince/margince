// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A regression for margince#5694: a domain-triage site read carries
// systemDomainTriageActor, not systemAutoEnrichActor, so a naive equality
// check against the auto-enrich sentinel alone leaves the resume sweep unable
// to recognise it — every other deferred read in the batch still resumes
// (each runs inside its own savepoint), but the mismatched row never advances,
// and the sweep's own returned error goes red on every tick until its
// deferral window lapses. This proves the sweep now recognises the lane, not
// just the earlier one.

import (
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/platform/jobs"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestDomainTriageReadResumesAfterBudgetDeferral(t *testing.T) {
	e := integration.Setup(t)
	next := time.Date(2099, 1, 1, 0, 0, 0, 0, time.UTC)

	// The real writer path: EnsureCounterparty opens the triage question and
	// StartDomainTriageSiteRead queues its dossier under systemDomainTriageActor
	// — exactly what the capture pipeline does, not a hand-rolled row.
	args := openTriageQuestion(t, e, triageTestDomain, "hannah@"+triageTestDomain, "Hannah Voss")
	worker := newTriageTestWorker(e, acmeDeepSite(), budgetDeferringBrain{next: next}, budgetDeferringBrain{next: next})
	if err := worker.run(e.As(e.Rep1, nil, integration.AdminPerms), args); err == nil {
		t.Fatal("expected the classification call to defer for budget")
	}

	status, statusCode, before := triageReadDeferralState(t, e, args.SiteReadID)
	if status != "deferred" || statusCode != "budget_deferred" || !before.Equal(next) {
		t.Fatalf("dossier not left budget-deferred: status=%q code=%q next=%s", status, statusCode, before)
	}

	// openTriageQuestion starts the dossier without going through the
	// production enqueue callback (capturedomaintriage.go's
	// startDomainTriageRead), so the retry job the sweep resumes has to be
	// inserted here, exactly as production's worker snooze would have left it.
	runner, err := jobs.NewInserter(e.Pool, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	if err := runner.Enqueue(e.Admin(), args, &river.InsertOpts{ScheduledAt: next, MaxAttempts: 3}); err != nil {
		t.Fatal(err)
	}

	setRecoveryAllowance(t, e, 1000000)
	recovery := newAIBudgetResumeWorker(e.Pool, slog.New(slog.DiscardHandler))
	if err := recovery.resumeWorkspace(e.Admin(), e.WS); err != nil {
		t.Fatalf("resumeWorkspace returned an error for a domain-triage read: %v", err)
	}

	status, _, after := triageReadDeferralState(t, e, args.SiteReadID)
	if status != "deferred" {
		// The worker still has to actually run the retry; the sweep's job is
		// only to make the row runnable again.
		t.Fatalf("resume changed the read's own status to %q", status)
	}
	if !after.Before(before) {
		t.Fatalf("resume did not advance next_attempt_at: before=%s after=%s", before, after)
	}
}

func triageReadDeferralState(t *testing.T, e *integration.Env, readID ids.UUID) (status, statusCode string, nextAttemptAt time.Time) {
	t.Helper()
	if err := e.DB().Tx(e.Admin(), func(tx pgx.Tx) error {
		return tx.QueryRow(e.Admin(), `SELECT status, status_code, next_attempt_at FROM site_read WHERE id=$1`, readID).
			Scan(&status, &statusCode, &nextAttemptAt)
	}); err != nil {
		t.Fatal(err)
	}
	return status, statusCode, nextAttemptAt
}
