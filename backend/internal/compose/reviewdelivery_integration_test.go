// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A review reaches its task through a delivery job the check queues with the
// finding. These tests work the queued rows the way River does: the real
// worker, attempt by attempt, over findings the real check raised.

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"

	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// queuedDelivery is one waiting review delivery row.
type queuedDelivery struct {
	id   int64
	args AssuranceReviewDeliveryArgs
}

func (v *viabilityEnv) queuedDeliveries(t *testing.T) []queuedDelivery {
	t.Helper()
	rows, err := integration.OwnerConn(t).Query(context.Background(), `
		SELECT id, args FROM river_job WHERE kind = $1 AND state = 'available' ORDER BY id`,
		AssuranceReviewDeliveryArgs{}.Kind())
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []queuedDelivery
	for rows.Next() {
		var q queuedDelivery
		var raw []byte
		if err := rows.Scan(&q.id, &raw); err != nil {
			t.Fatal(err)
		}
		if err := json.Unmarshal(raw, &q.args); err != nil {
			t.Fatal(err)
		}
		out = append(out, q)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

// workDelivery runs one attempt of a queued delivery. deliver, when set,
// stands in for the delivery itself.
func (v *viabilityEnv) workDelivery(
	t *testing.T, q queuedDelivery, attempt int,
	deliver func(context.Context, bundleDeps, AssuranceReviewDeliveryArgs) error,
) error {
	t.Helper()
	worker := &assuranceReviewDeliveryWorker{sweep: v.worker, deliver: deliver}
	return worker.Work(context.Background(), &river.Job[AssuranceReviewDeliveryArgs]{
		JobRow: &rivertype.JobRow{ID: q.id, Attempt: attempt, MaxAttempts: oneOffJobMaxAttempts},
		Args:   q.args,
	})
}

// finishDelivery settles the River row, as River does after the last attempt.
func (v *viabilityEnv) finishDelivery(t *testing.T, q queuedDelivery, state rivertype.JobState) {
	t.Helper()
	if _, err := integration.OwnerConn(t).Exec(context.Background(),
		`UPDATE river_job SET state = $2, finalized_at = now() WHERE id = $1`, q.id, string(state)); err != nil {
		t.Fatal(err)
	}
}

// deliverReviews works every queued delivery once, as River's first attempt.
func (v *viabilityEnv) deliverReviews(t *testing.T) {
	t.Helper()
	for _, q := range v.queuedDeliveries(t) {
		if err := v.workDelivery(t, q, 1, nil); err != nil {
			t.Fatalf("delivering review %s: %v", q.args.Exception, err)
		}
		v.finishDelivery(t, q, rivertype.JobStateCompleted)
	}
}

func (v *viabilityEnv) deliveryCounts(t *testing.T) assurance.DeliveryCounts {
	t.Helper()
	run, err := assurance.NewStore(InstallationDB(v.Pool)).LatestRun(v.Admin())
	if err != nil {
		t.Fatal(err)
	}
	return run.Deliveries
}

func (v *viabilityEnv) reviewTask(t *testing.T) ids.UUID {
	t.Helper()
	task, err := ids.Parse(v.WsScalar(t, `SELECT task_activity_id::text FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.silent))
	if err != nil {
		t.Fatalf("the review has no task: %v", err)
	}
	return task
}

// settledAfterDelivery dates a completion after the delivery that just wrote
// the task. The database clock can run a little ahead of the test's, and a
// completion dated before the task's last write is refused as stale.
func settledAfterDelivery(t *testing.T) time.Time {
	t.Helper()
	return clocktest.Now(t).Add(time.Minute)
}

func (v *viabilityEnv) assigneeOf(t *testing.T, task ids.UUID) string {
	t.Helper()
	return v.WsScalar(t, `SELECT coalesce(assignee_id::text, '') FROM activity WHERE id = $1`, task)
}

var errDeliveryBroke = errors.New("the delivery lost its connection")

func failDelivery(context.Context, bundleDeps, AssuranceReviewDeliveryArgs) error {
	return errDeliveryBroke
}

func TestAFailedDeliveryIsRetriedAndDeliversExactlyOneTask(t *testing.T) {
	v := setupViability(t)
	v.runCheck(t)
	queued := v.queuedDeliveries(t)
	if len(queued) != 1 {
		t.Fatalf("%d deliveries queued, want one for the silent deal's review", len(queued))
	}
	if err := v.workDelivery(t, queued[0], 1, failDelivery); !errors.Is(err, errDeliveryBroke) {
		t.Fatalf("the failing attempt answered %v", err)
	}
	if got := v.deliveryCounts(t); got != (assurance.DeliveryCounts{Pending: 1}) {
		t.Fatalf("after a failed first attempt the counts are %+v, want one pending", got)
	}

	// River's retry, and a second run of the same row, deliver one task.
	for attempt := 2; attempt <= 3; attempt++ {
		if err := v.workDelivery(t, queued[0], attempt, nil); err != nil {
			t.Fatalf("retry %d: %v", attempt, err)
		}
	}
	if all, open := v.reviewTasks(t, v.silent); all != 1 || open != 1 {
		t.Fatalf("%d review tasks, %d open; want exactly one open task", all, open)
	}
	if got := v.deliveryCounts(t); got != (assurance.DeliveryCounts{Delivered: 1}) {
		t.Fatalf("after the retry the counts are %+v, want one delivered", got)
	}
	if got := v.WsCount(t, `SELECT delivery_attempts FROM assurance_exception WHERE id = $1`,
		queued[0].args.Exception); got != 3 {
		t.Errorf("%d attempts recorded, want 3", got)
	}
}

func TestADeliveryOutOfRetriesCountsAsFailedUntilTheNextCheckDeliversIt(t *testing.T) {
	v := setupViability(t)
	v.runCheck(t)
	queued := v.queuedDeliveries(t)
	if len(queued) != 1 {
		t.Fatalf("%d deliveries queued, want one", len(queued))
	}
	if err := v.workDelivery(t, queued[0], oneOffJobMaxAttempts, failDelivery); err == nil {
		t.Fatal("the failing last attempt answered no error")
	}
	v.finishDelivery(t, queued[0], rivertype.JobStateDiscarded)
	if got := v.deliveryCounts(t); got != (assurance.DeliveryCounts{Failed: 1}) {
		t.Fatalf("after the last attempt failed the counts are %+v, want one failed", got)
	}

	v.check(t)
	if got := v.deliveryCounts(t); got != (assurance.DeliveryCounts{Delivered: 1}) {
		t.Fatalf("after the next check the counts are %+v, want one delivered", got)
	}
	if all, _ := v.reviewTasks(t, v.silent); all != 1 {
		t.Fatalf("%d review tasks, want one", all)
	}
}

func TestACompletedReviewTaskStillCountsAsDelivered(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	if done, err := v.Activities.CompleteTask(v.Admin(), ids.From[ids.ActivityKind](v.reviewTask(t)),
		settledAfterDelivery(t)); err != nil || !done {
		t.Fatalf("completing the review task: done=%v err=%v", done, err)
	}
	if got := v.deliveryCounts(t); got != (assurance.DeliveryCounts{Delivered: 1}) {
		t.Fatalf("the counts are %+v, want the open review with its done task delivered", got)
	}
}

func (v *viabilityEnv) moveDeal(t *testing.T, to ids.UUID) {
	t.Helper()
	owner := ids.From[ids.UserKind](to)
	if _, err := v.Deals.UpdateDeal(v.Admin(), ids.From[ids.DealKind](v.silent), deals.UpdateDealInput{
		OwnerID: &owner,
	}); err != nil {
		t.Fatalf("moving the deal to a new owner: %v", err)
	}
}

func TestTheReviewTaskFollowsTheDealToItsNewOwner(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	task := v.reviewTask(t)
	if got := v.assigneeOf(t, task); got != v.Rep1.String() {
		t.Fatalf("the review task went to %q, want the deal owner %s", got, v.Rep1)
	}

	v.moveDeal(t, v.Rep2)
	v.check(t)
	if got := v.assigneeOf(t, task); got != v.Rep2.String() {
		t.Errorf("after the deal changed hands the task is with %q, want the new owner %s", got, v.Rep2)
	}

	v.moveDeal(t, v.Rep3)
	v.check(t)
	if got := v.assigneeOf(t, task); got != v.Rep3.String() {
		t.Errorf("after a second change the task is with %q, want %s", got, v.Rep3)
	}
}

func TestAReviewTaskSomebodyReassignedStaysWhenTheDealChangesOwner(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	task := v.reviewTask(t)
	chosen := ids.From[ids.UserKind](v.Rep3)
	if _, err := v.Activities.UpdateActivity(v.Admin(), ids.From[ids.ActivityKind](task),
		activities.UpdateActivityInput{AssigneeID: &chosen}); err != nil {
		t.Fatalf("a colleague taking the task: %v", err)
	}

	v.moveDeal(t, v.Rep2)
	v.check(t)
	if got := v.assigneeOf(t, task); got != v.Rep3.String() {
		t.Errorf("the pass moved the task to %q; the colleague who took it, %s, keeps it", got, v.Rep3)
	}
}

func (v *viabilityEnv) taskDone(t *testing.T, task ids.UUID) bool {
	t.Helper()
	return v.WsScalar(t, `SELECT is_done::text FROM activity WHERE id = $1`, task) == "true"
}

func TestTheReviewTaskCompletesWhenItsLastFindingSettlesAndNotBefore(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	task := v.reviewTask(t)
	other, err := ids.Parse(v.WsScalar(t, `SELECT id::text FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeNoEconomicBuyer, v.silent))
	if err != nil {
		t.Fatalf("the silent deal carries no second finding to file under the task: %v", err)
	}

	v.customerWrites(t, v.silent, clocktest.Now(t).Add(-time.Hour))
	if err := v.reviewStore().Resolve(v.Admin(), v.reviewOf(t, v.silent),
		assurance.Resolution{Outcome: assurance.OutcomeFixedRecord}, clocktest.Now(t)); err != nil {
		t.Fatalf("reviving the deal: %v", err)
	}
	v.check(t)
	if v.taskDone(t, task) {
		t.Fatal("the task completed while a second finding filed under it is still open")
	}

	until := clocktest.Now(t).AddDate(0, 0, 30)
	if err := v.reviewStore().Resolve(v.Admin(), other, assurance.Resolution{
		Outcome: assurance.OutcomeValueCorrect, Reason: "The buyer signs off", ExpiresAt: &until,
	}, clocktest.Now(t)); err != nil {
		t.Fatalf("answering the second finding: %v", err)
	}
	v.check(t)
	if !v.taskDone(t, task) {
		t.Error("the review task stayed open after every finding on it was answered")
	}
	if got := v.WsCount(t, `SELECT count(*) FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'task' AND a.source = 'manual' AND NOT a.is_done`, v.silent); got != 1 {
		t.Errorf("%d open next-step tasks, want the rep's own task left open", got)
	}
}

// deliveryAudits counts the audit rows the review's delivery wrote whose after
// image carries key.
func (v *viabilityEnv) deliveryAudits(t *testing.T, review ids.UUID, key string) int {
	t.Helper()
	return v.WsCount(t, `SELECT count(*) FROM audit_log
		WHERE entity_type = 'assurance_exception' AND entity_id = $1 AND after ? $2`, review, key)
}

func TestADeliveryGoesToTheOwnerTheDealHasWhenItRuns(t *testing.T) {
	v := setupViability(t)
	v.runCheck(t)
	queued := v.queuedDeliveries(t)
	if len(queued) != 1 {
		t.Fatalf("%d deliveries queued, want one", len(queued))
	}
	v.moveDeal(t, v.Rep2)
	if err := v.workDelivery(t, queued[0], 1, nil); err != nil {
		t.Fatal(err)
	}
	if got := v.assigneeOf(t, v.reviewTask(t)); got != v.Rep2.String() {
		t.Errorf("the task went to %q, want the deal's owner when it was delivered, %s", got, v.Rep2)
	}
	review := queued[0].args.Exception
	if got := v.WsScalar(t, `SELECT coalesce(delivered_assignee_id::text, '') FROM assurance_exception
		WHERE id = $1`, review); got != v.Rep2.String() {
		t.Errorf("the episode records %q as the seat it gave the task, want %s", got, v.Rep2)
	}
	if v.deliveryAudits(t, review, "delivery_state") != 1 || v.deliveryAudits(t, review, "delivery_attempt") != 1 {
		t.Errorf("the queueing and the attempt each want one audit row on the review")
	}
}

func TestADeliveryFilesTheFindingsOfTheNewestCheck(t *testing.T) {
	v := setupViability(t)
	v.runCheck(t)
	next, err := ids.Parse(v.WsScalar(t, `SELECT a.id::text FROM activity a JOIN activity_link l ON l.activity_id = a.id
		WHERE l.deal_id = $1 AND a.kind = 'task' AND a.source = 'manual'`, v.silent))
	if err != nil {
		t.Fatal(err)
	}
	if done, err := v.Activities.CompleteTask(v.Admin(), ids.From[ids.ActivityKind](next), settledAfterDelivery(t)); err != nil || !done {
		t.Fatalf("completing the deal's next step: done=%v err=%v", done, err)
	}
	v.runCheck(t)
	queued := v.queuedDeliveries(t)
	if len(queued) != 1 {
		t.Fatalf("%d deliveries queued after two checks, want the one", len(queued))
	}
	if err := v.workDelivery(t, queued[0], 1, nil); err != nil {
		t.Fatal(err)
	}
	if got := v.WsCount(t, `SELECT count(*) FROM assurance_task_item i
		JOIN assurance_exception e ON e.id = i.exception_id
		WHERE i.task_activity_id = $1 AND e.type = $2`, v.reviewTask(t), assurance.TypeNoNextStep); got != 1 {
		t.Errorf("the task holds %d no-next-step findings, want the one only the second check saw", got)
	}
}

func TestAMoveWhoseRecordWasLostIsRecordedOnTheNextPass(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	task := v.reviewTask(t)
	review := v.reviewOf(t, v.silent)

	// The task moved, and the pass failed before it recorded the move.
	v.moveDeal(t, v.Rep2)
	rep1, rep2 := ids.From[ids.UserKind](v.Rep1), ids.From[ids.UserKind](v.Rep2)
	if moved, err := v.Activities.FollowSystemTaskOwner(v.Admin(), ids.From[ids.ActivityKind](task), &rep1, rep2); err != nil || !moved {
		t.Fatalf("moving the task: moved=%v err=%v", moved, err)
	}
	v.check(t)
	if got := v.WsScalar(t, `SELECT coalesce(delivered_assignee_id::text, '') FROM assurance_exception
		WHERE id = $1`, review); got != v.Rep2.String() {
		t.Fatalf("the episode still records %q as the seat it gave the task, want %s", got, v.Rep2)
	}
	if v.deliveryAudits(t, review, "delivered_assignee_id") != 1 {
		t.Errorf("recording the move wants one audit row on the review")
	}

	v.moveDeal(t, v.Rep3)
	v.check(t)
	if got := v.assigneeOf(t, task); got != v.Rep3.String() {
		t.Errorf("after the next owner change the task is with %q, want %s", got, v.Rep3)
	}
}

// The check walks findings by value, largest first. The delivered review on
// the larger deal comes first, and it must not end the walk.
func TestADeliveredReviewDoesNotKeepALaterReviewFromItsDelivery(t *testing.T) {
	v := setupViability(t)
	v.check(t)
	pipeline, err := ids.Parse(v.WsScalar(t, `SELECT pipeline_id::text FROM deal WHERE id = $1`, v.silent))
	if err != nil {
		t.Fatal(err)
	}
	stage, err := ids.Parse(v.WsScalar(t, `SELECT stage_id::text FROM deal WHERE id = $1`, v.silent))
	if err != nil {
		t.Fatal(err)
	}
	smaller := v.commitDeal(t, ids.From[ids.PipelineKind](pipeline), ids.From[ids.StageKind](stage),
		"Silent since summer", 50_000, v.now.AddDate(0, 0, -150))

	v.runCheck(t)
	queued := v.queuedDeliveries(t)
	if len(queued) != 1 {
		t.Fatalf("%d deliveries queued, want one for the new review", len(queued))
	}
	v.deliverReviews(t)
	if all, open := v.reviewTasks(t, smaller); all != 1 || open != 1 {
		t.Fatalf("%d review tasks on the smaller deal, %d open; want one open task", all, open)
	}
}
