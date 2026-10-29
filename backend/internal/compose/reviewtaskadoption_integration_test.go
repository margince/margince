// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// A review that begins on a deal which already has an open task from an
// earlier nightly pass takes that task over. Those older tasks were minted
// with no due date and often with nobody assigned, so the review fills both
// in. These tests write the older task through the same mint and filing the
// earlier passes used.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// earlierPassTask is an open task an earlier pass left on the silent deal,
// keyed by its run, with no due date and no assignee.
func (v *viabilityEnv) earlierPassTask(t *testing.T) ids.UUID {
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
	deps := bundleDeps{bundles: store, activities: v.worker.activities, exceptions: bundleableFindings, now: v.worker.now}
	run := ids.NewV7()
	cycle, err := store.OpenCycle(ctx, cycleScopeForRun(run))
	if err != nil {
		t.Fatal(err)
	}
	subject := subjectFindings{kind: "deal", id: v.silent, findings: []assurance.Exception{{ID: finding}}}
	task, err := mintBundleTask(ctx, deps, runTaskKey(subject, run), subject, nil)
	if err != nil {
		t.Fatalf("minting the earlier task: %v", err)
	}
	if err := fileUnderTask(ctx, deps, cycle, task, subject); err != nil {
		t.Fatal(err)
	}
	if err := store.CloseCycle(ctx, cycle); err != nil {
		t.Fatal(err)
	}
	return task
}

// adoptedTask checks the review took over the earlier task and minted none.
func (v *viabilityEnv) adoptedTask(t *testing.T, earlier ids.UUID) {
	t.Helper()
	if got := v.WsScalar(t, `SELECT task_activity_id::text FROM assurance_exception
		WHERE type = $1 AND subject_id = $2`, assurance.TypeDealViability, v.silent); got != earlier.String() {
		t.Fatalf("the review links task %q, want the earlier task %s", got, earlier)
	}
	if all, _ := v.reviewTasks(t, v.silent); all != 1 {
		t.Fatalf("%d tasks on the deal, want the one earlier task and no second", all)
	}
}

func TestAnAdoptedTaskGetsTheReviewDueDateAndTheDealOwner(t *testing.T) {
	v := setupViability(t)
	earlier := v.earlierPassTask(t)

	v.check(t)
	v.adoptedTask(t, earlier)
	if got := v.WsScalar(t, `SELECT coalesce(assignee_id::text, '') FROM activity WHERE id = $1`, earlier); got != v.Rep1.String() {
		t.Errorf("the adopted task is assigned to %q, want the deal owner %s", got, v.Rep1)
	}
	due := v.WsScalar(t, `SELECT coalesce(to_char(due_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), '')
		FROM activity WHERE id = $1`, earlier)
	at, err := time.Parse(time.RFC3339, due)
	if err != nil || !at.After(v.now) || at.After(v.now.Add(72*time.Hour)) {
		t.Errorf("the adopted task is due %q, want the end of the next local day after %s", due, v.now)
	}
}

func TestAnAdoptedTaskKeepsTheDateAndAssigneeSomebodyChose(t *testing.T) {
	v := setupViability(t)
	earlier := v.earlierPassTask(t)
	chosen := time.Date(2031, 3, 4, 9, 0, 0, 0, time.UTC)
	rep2 := ids.From[ids.UserKind](v.Rep2)
	if _, err := v.Activities.UpdateActivity(v.Admin(), ids.From[ids.ActivityKind](earlier),
		activities.UpdateActivityInput{DueAt: &chosen, AssigneeID: &rep2}); err != nil {
		t.Fatalf("a person reassigning and re-dating the task: %v", err)
	}

	v.check(t)
	v.adoptedTask(t, earlier)
	if got := v.WsScalar(t, `SELECT assignee_id::text FROM activity WHERE id = $1`, earlier); got != v.Rep2.String() {
		t.Errorf("the pass moved the task to %q; the person's choice %s must stand", got, v.Rep2)
	}
	if got := v.WsCount(t, `SELECT count(*) FROM activity WHERE id = $1 AND due_at = $2`, earlier, chosen); got != 1 {
		t.Errorf("the pass moved the due date the person set")
	}
}

func TestAnAdoptedTaskWhoseOwnerLeftStaysUnassignedWithADueDate(t *testing.T) {
	v := setupViability(t)
	earlier := v.earlierPassTask(t)
	v.WsExec(t, `UPDATE app_user SET archived_at = now() WHERE id = $1`, v.Rep1)

	v.check(t)
	v.adoptedTask(t, earlier)
	if got := v.WsCount(t, `SELECT count(*) FROM activity
		WHERE id = $1 AND assignee_id IS NULL AND due_at IS NOT NULL`, earlier); got != 1 {
		t.Errorf("the task of a departed owner should wait unassigned with a due date")
	}
}
