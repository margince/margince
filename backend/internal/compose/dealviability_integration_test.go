// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A deal whose customer went silent half a year ago, with a close date three
// months ahead, is what the close-date check missed. Its date is in the future,
// so nothing about it looked wrong. These tests drive the real
// worker over deals written through the deals and activities stores. Then they
// read the forecast for the period the close date falls in.

import (
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/riverqueue/river"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/config"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type viabilityEnv struct {
	*integration.Env
	worker    *assuranceSweepWorker
	now       time.Time
	closeDate time.Time
	silent    ids.UUID
	supported ids.UUID
	buyer     ids.UUID
}

func setupViability(t *testing.T) *viabilityEnv {
	t.Helper()
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	now := clocktest.Now(t)
	v := &viabilityEnv{
		Env: e, now: now, buyer: e.SeedContact(t, "Buyer", nil),
		closeDate: time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, time.UTC).AddDate(0, 0, 94),
		worker: &assuranceSweepWorker{
			pool: e.Pool, now: func() time.Time { return now }, log: slog.New(slog.DiscardHandler),
			activities: activities.NewStore(InstallationDB(e.Pool)),
		},
	}
	v.readMailboxOnTime(t)
	v.silent = v.commitDeal(t, pipeline, open, "Silent since spring", 500_000, now.AddDate(0, 0, -196))
	v.supported = v.commitDeal(t, pipeline, open, "Answered last week", 100_000, now.AddDate(0, 0, -5))
	return v
}

// readMailboxOnTime is a mail connector that synced three hours ago, so the
// check may observe silence. Without it the deal would read unknown.
func (v *viabilityEnv) readMailboxOnTime(t *testing.T) {
	t.Helper()
	owner := integration.OwnerConn(t)
	conn := integration.SeedIDRow(t, owner, `
		INSERT INTO capture_connection (id, provider, user_id, status)
		VALUES ($1, 'gmail', $2, 'connected')`, v.Rep1)
	if _, err := owner.Exec(context.Background(), `
		INSERT INTO capture_sync_state (connection_id, last_success_at)
		VALUES ($1, now() - interval '3 hours')`, conn); err != nil {
		t.Fatal(err)
	}
}

// commitDeal writes a committed deal with a confirmed close date and the
// customer's last mail at lastReply.
func (v *viabilityEnv) commitDeal(
	t *testing.T, pipeline ids.PipelineID, stage ids.StageID, name string, amount int64, lastReply time.Time,
) ids.UUID {
	t.Helper()
	owner := ids.From[ids.UserKind](v.Rep1)
	currency, commit := "EUR", "commit"
	created, err := v.Deals.CreateDeal(v.Admin(), deals.CreateDealInput{
		Name: name, PipelineID: pipeline, StageID: stage, OwnerID: &owner,
		AmountMinor: &amount, Currency: &currency, ExpectedClose: &v.closeDate,
	})
	if err != nil {
		t.Fatal(err)
	}
	deal := ids.UUID(created.Id)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](deal), deals.UpdateDealInput{
		ForecastCategory: &commit,
	}); err != nil {
		t.Fatal(err)
	}
	v.customerWrites(t, deal, lastReply)
	return deal
}

// customerWrites logs a call with the buyer on the deal: the customer moving it.
func (v *viabilityEnv) customerWrites(t *testing.T, deal ids.UUID, at time.Time) {
	t.Helper()
	subject := "Call with the buyer"
	if _, _, err := v.Activities.LogActivity(v.Admin(), activities.LogActivityInput{
		Kind: "call", Subject: &subject, OccurredAt: &at, Source: "manual",
		Links: []activities.ActivityLinkInput{
			{EntityType: "deal", EntityID: deal}, {EntityType: "contact", EntityID: v.buyer},
		},
	}); err != nil {
		t.Fatal(err)
	}
}

func (v *viabilityEnv) check(t *testing.T) {
	t.Helper()
	worker := &assuranceRunWorker{sweep: v.worker}
	if err := worker.Work(context.Background(), &river.Job[AssuranceRunArgs]{
		Args: AssuranceRunArgs{Workspace: v.WS, RequestedBy: v.Rep1.String()},
	}); err != nil {
		t.Fatalf("the input check: %v", err)
	}
}

// forecast reads the qualified forecast for the quarter the close date is in,
// as an admin, through the read every forecast surface shares.
func (v *viabilityEnv) forecast(t *testing.T) forecasting.Readings {
	t.Helper()
	var out forecasting.Readings
	ctx := v.Admin()
	if err := database.WithWorkspaceTx(ctx, v.Pool, func(tx pgx.Tx) error {
		period, base, err := ForecastPeriodAt(ctx, tx, forecasting.PeriodQuarter, v.closeDate)
		if err != nil {
			return err
		}
		dealsIn, _, _, err := ForecastDeals(ctx, tx, period, forecasting.Scope{Kind: ScopeKindWorkspace}, v.now, base)
		if err != nil {
			return err
		}
		out, err = forecasting.Compute(period, period.LocalDay(v.now), dealsIn)
		return err
	}); err != nil {
		t.Fatalf("reading the forecast: %v", err)
	}
	return out
}

func (v *viabilityEnv) reviewTasks(t *testing.T, deal ids.UUID) (all, open int) {
	t.Helper()
	all = v.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'task' AND a.origin = 'system_remediation'`, deal)
	open = v.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'task' AND a.origin = 'system_remediation' AND NOT a.is_done`, deal)
	return all, open
}

func TestASilentDealWithAFarCloseDateLeavesTheQualifiedForecastAndGetsOneReview(t *testing.T) {
	v := setupViability(t)

	before := v.forecast(t)
	if before.Qualified != nil {
		t.Fatalf("qualified readings %+v before anybody enforced them", before.Qualified)
	}
	if before.EvidenceMinor != 600_000 {
		t.Fatalf("baseline evidence %d, want both commits (600000) — the fixture no longer puts "+
			"them in the quarter of their close date, so the assertions below would prove nothing",
			before.EvidenceMinor)
	}

	// A call dated three days ahead has not happened, so it is no progress.
	v.customerWrites(t, v.silent, v.now.AddDate(0, 0, 3))
	v.check(t)
	if got := v.WsCount(t, `SELECT count(*) FROM assurance_exception
		WHERE type = $1 AND status = 'open' AND subject_id = $2`, assurance.TypeDealViability, v.silent); got != 1 {
		t.Fatalf("%d open viability reviews on the silent deal, want 1", got)
	}
	if got := v.WsCount(t, `SELECT count(*) FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.supported); got != 0 {
		t.Fatalf("the deal whose customer answered last week got %d viability reviews", got)
	}
	if got := v.WsCount(t, `SELECT count(*) FROM assurance_deal_receipt WHERE deal_id IN ($1, $2)`,
		v.silent, v.supported); got != 2 {
		t.Fatalf("%d receipts for the two deals, want one each — an eligible deal needs its own", got)
	}
	if all, _ := v.reviewTasks(t, v.silent); all != 1 {
		t.Fatalf("%d review tasks on the silent deal, want 1", all)
	}

	if err := assurance.NewStore(InstallationDB(v.Pool)).SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatalf("enforcing: %v", err)
	}
	after := v.forecast(t)
	if after.EvidenceMinor != before.EvidenceMinor || after.WeightedMinor != before.WeightedMinor {
		t.Errorf("the baseline moved from %d/%d to %d/%d — enforcement must not rewrite it",
			before.EvidenceMinor, before.WeightedMinor, after.EvidenceMinor, after.WeightedMinor)
	}
	q := after.Qualified
	if q == nil || q.EvidenceMinor != 100_000 || q.ExcludedMinor != 500_000 || q.ExcludedCount != 1 || q.UnknownCount != 0 {
		t.Fatalf("qualified readings %+v, want 100000 evidence with the silent 500000 excluded", q)
	}
}

func TestACompletedReviewTaskIsNotMintedAgainAndAReplyAloneKeepsTheReview(t *testing.T) {
	v := setupViability(t)
	v.check(t)

	var task ids.UUID
	if err := v.Pool.QueryRow(context.Background(), `SELECT task_activity_id FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.silent).Scan(&task); err != nil {
		t.Fatalf("the review carries no task link: %v", err)
	}
	// Settled now rather than at the fixture's clock: the task was written after
	// that instant, and a completion dated before it is refused as stale.
	if done, err := v.Activities.CompleteTask(v.Admin(), ids.From[ids.ActivityKind](task), clocktest.Now(t)); err != nil || !done {
		t.Fatalf("completing the review task: done=%v err=%v", done, err)
	}
	v.check(t)
	if all, open := v.reviewTasks(t, v.silent); all != 1 || open != 0 {
		t.Fatalf("after completing the task and a second check: %d tasks, %d open; want the one "+
			"completed task and no replacement", all, open)
	}

	// The customer answers today. Silence is over, but nobody validated the
	// next step or the date, so the review stays and the deal stays out.
	v.customerWrites(t, v.silent, v.now.Add(-time.Hour))
	v.check(t)
	if got := v.WsCount(t, `SELECT count(*) FROM assurance_exception
		WHERE type = $1 AND status = 'open' AND subject_id = $2`, assurance.TypeDealViability, v.silent); got != 1 {
		t.Fatalf("a reply closed the review (%d open) — only somebody's answer may", got)
	}
	if got := v.WsScalar(t, `SELECT array_to_string(reasons, ',') FROM assurance_deal_receipt
		WHERE deal_id = $1`, v.silent); got == "" || hasReason(got, "customer_silent") || !hasReason(got, "requalification_pending") {
		t.Fatalf("receipt reasons %q, want requalification pending and no customer silence", got)
	}
	if err := assurance.NewStore(InstallationDB(v.Pool)).SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatal(err)
	}
	if q := v.forecast(t).Qualified; q == nil || q.ExcludedCount != 1 {
		t.Fatalf("qualified readings %+v, want the replied deal still excluded until it is revived", q)
	}
}

func TestEvidenceThatMovedSinceTheCheckMakesTheDealUnknown(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	if err := assurance.NewStore(InstallationDB(v.Pool)).SetQualifiedForecastEnforced(
		v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatal(err)
	}
	// The supported deal's customer writes again after the check. The receipt
	// describes evidence the deal no longer carries, so it may not be trusted.
	v.customerWrites(t, v.supported, v.now.Add(-time.Minute))
	q := v.forecast(t).Qualified
	if q == nil || q.UnknownCount != 1 || q.UnknownMinor != 100_000 || q.EvidenceMinor != 0 {
		t.Fatalf("qualified readings %+v, want the changed deal unknown until the next check", q)
	}
}

func hasReason(list, item string) bool {
	return slices.Contains(strings.Split(list, ","), item)
}

// While the qualified forecast is enforced the viability review decides what
// counts, so the sweep keeps its date work and lowers no category. A notch a
// night would otherwise walk a quiet deal down to omitted with nobody deciding.
func TestCloseDateSweepLowersNoCategoryWhileTheQualifiedForecastIsEnforced(t *testing.T) {
	e := setupCloseDate(t)
	if _, err := e.owner.Exec(context.Background(), `UPDATE deal SET archived_at=now() WHERE pipeline_id=$1 AND status='won'`, e.pipeline); err != nil {
		t.Fatal(err)
	}
	if err := config.Save(e.Admin(), StoredConfig(e.Pool), assurance.QualifiedForecastEnforced, true); err != nil {
		t.Fatal(err)
	}
	id := e.seedSweepDeal(t, "Gone quiet under enforcement", e.late, new("commit"), new(30), 90)

	if err := e.sweep(); err != nil {
		t.Fatal(err)
	}

	swept := e.readSwept(t, id)
	if swept.forecastCat == nil || *swept.forecastCat != "commit" {
		t.Errorf("forecast_category = %v, want commit kept while the review decides what counts", swept.forecastCat)
	}
	if want := today().AddDate(0, 0, 30); swept.expectedClose == nil || !swept.expectedClose.Equal(want) {
		t.Errorf("date = %v, want retained %s", swept.expectedClose, want)
	}
}

// A review is answered with an override or a reminder. "Fixed" with nothing
// checked would put the deal back in the qualified forecast unseen. An override
// counts it again until it ends, and an ended override reads unknown at once.
func TestAReviewAnswerIsAnOverrideThatEndsOrNothing(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	store := assurance.NewStore(InstallationDB(v.Pool))
	if err := store.SetQualifiedForecastEnforced(v.Admin(), StoredConfig(v.Pool), true); err != nil {
		t.Fatal(err)
	}
	var review ids.UUID
	if err := v.Pool.QueryRow(context.Background(), `SELECT id FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.silent).Scan(&review); err != nil {
		t.Fatal(err)
	}
	now := clocktest.Now(t)
	if err := store.Resolve(v.Admin(), review, assurance.Resolution{Outcome: assurance.OutcomeFixedRecord}, now); err == nil {
		t.Fatal("a review was closed as fixed with nothing checked")
	}
	until := now.AddDate(0, 0, 30)
	if err := store.Resolve(v.Admin(), review, assurance.Resolution{
		Outcome: assurance.OutcomeValueCorrect, Reason: "Signed order in the post", ExpiresAt: &until,
	}, now); err != nil {
		t.Fatalf("overriding the review: %v", err)
	}
	if q := v.forecast(t).Qualified; q == nil || q.ExcludedCount != 0 || q.EvidenceMinor != 600_000 {
		t.Fatalf("qualified readings %+v, want the overridden deal counted again", q)
	}
	// An override cannot end before it was given, so the lapse is reached by
	// moving the answer back in time rather than its end alone.
	v.WsExec(t, `UPDATE assurance_resolution
		SET created_at = now() - interval '2 days', expires_at = now() - interval '1 minute'
		WHERE exception_id = $1`, review)
	if q := v.forecast(t).Qualified; q == nil || q.UnknownCount != 1 || q.UnknownMinor != 500_000 {
		t.Fatalf("qualified readings %+v, want the deal unknown once its override ended", q)
	}
}
