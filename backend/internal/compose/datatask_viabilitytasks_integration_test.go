// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"log/slog"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/viabilitytasks"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Ten days after the close, the bundler files each deal's task while no
// mailbox is read. Then Rep1's mailbox is read. Only Rep1's task holding
// nothing but a past close date is one the viability decision answers.
func TestTheDataTaskCompletesOnlyTasksTheViabilityDecisionAnswers(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	now := clocktest.Now(t)
	later := now.AddDate(0, 0, 10)
	v := &viabilityEnv{Env: e, now: now, worker: &assuranceSweepWorker{
		pool: e.Pool, now: func() time.Time { return later }, log: slog.New(slog.DiscardHandler),
		activities: activities.NewStore(InstallationDB(e.Pool)),
	}}
	pastDeal := func(name string, rep ids.UUID, chance int) ids.UUID {
		owner, closes := ids.From[ids.UserKind](rep), now
		created, err := e.Deals.CreateDeal(e.Admin(), deals.CreateDealInput{
			Name: name, PipelineID: pipeline, StageID: open, OwnerID: &owner, ExpectedClose: &closes,
		})
		if err == nil {
			_, err = e.Deals.UpdateDeal(e.Admin(), ids.From[ids.DealKind](ids.UUID(created.Id)), deals.UpdateDealInput{WinProbability: new(chance)})
		}
		if err != nil {
			t.Fatal(err)
		}
		v.planNextStep(t, ids.UUID(created.Id))
		return ids.UUID(created.Id)
	}
	answered, mixed := pastDeal("Only a past close", e.Rep1, 5), pastDeal("No economic buyer either", e.Rep1, 95)
	unjudged := pastDeal("Its owner's mail is unread", e.Rep2, 5)
	v.check(t)
	first, err := ids.Parse(e.WsScalar(t, `SELECT id::text FROM assurance_run ORDER BY id DESC LIMIT 1`))
	if err != nil {
		t.Fatal(err)
	}
	taskOf := func(deal ids.UUID) string {
		return e.WsScalar(t, `SELECT coalesce(max(task_activity_id::text), '') FROM assurance_task_item WHERE subject_id = $1`, deal)
	}
	answeredTask, mixedTask, unjudgedTask := taskOf(answered), taskOf(mixed), taskOf(unjudged)
	if answeredTask == "" || mixedTask == "" || unjudgedTask == "" {
		t.Fatalf("the unjudged pass filed tasks %q, %q and %q, want one per deal", answeredTask, mixedTask, unjudgedTask)
	}
	connection := integration.SeedIDRow(t, integration.OwnerConn(t),
		`INSERT INTO capture_connection (id, provider, user_id, status) VALUES ($1, 'gmail', $2, 'connected')`, e.Rep1)
	e.WsExec(t, `INSERT INTO capture_sync_state (connection_id, last_success_at) VALUES ($1, $2)`, connection, later.Add(-3*time.Hour))
	v.check(t)

	// The first pass files by its own unjudged reading, not the later receipt.
	ctx := principal.SystemActing(e.Admin(), "system:datatask-test")
	if _, err := bundleRunFindings(ctx, bundleDeps{
		bundles:    assurance.NewStore(InstallationDB(e.Pool)),
		activities: v.worker.activities, exceptions: bundleableFindings,
	}, first); err != nil {
		t.Fatal(err)
	}
	if filed := e.WsCount(t, `SELECT count(*) FROM assurance_task_item WHERE subject_id = $1`, answered); filed != 2 {
		t.Fatalf("the past close is filed %d times, want twice: once per pass that did not judge the deal", filed)
	}
	audits := func() int {
		return e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'activity' AND entity_id = $1::uuid`, answeredTask)
	}
	done := func(task string) string {
		return e.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1::uuid AND archived_at IS NULL`, task)
	}
	before := audits()
	for range 2 {
		runMachineAddressTask(ctx, t, e, viabilitytasks.DataTask(activities.NewStore(InstallationDB(e.Pool))))
		if done(answeredTask) != "true" || done(mixedTask) != "false" || done(unjudgedTask) != "false" || audits() != before+1 {
			t.Fatalf("answered task done=%s with %d new audit rows, mixed done=%s, unjudged done=%s; want only the answered one completed, audited once",
				done(answeredTask), audits()-before, done(mixedTask), done(unjudgedTask))
		}
	}
}
