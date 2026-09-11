// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

// The nightly pass turns its findings into work, against a real database.
//
// This suite exists because the store's own tests could not prove it. Every one
// of them calls the bundling store directly and seeds its task rows with raw
// SQL, so all of them stayed green while NOTHING in the tree called the store —
// a running installation had zero cycles and zero task items, and no assertion
// anywhere said so.
//
// So these drive the WORKER, through the same assureWorkspace River calls per
// tenant, and read the result as a rep would: an ordinary task, linked to the
// deal, on somebody's list.

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/assurance"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// bundledTask is one task the pass minted, read back the way a list renders it.
type bundledTask struct {
	id       ids.UUID
	subject  string
	assignee *ids.UUID
	dealID   *ids.UUID
	findings int
}

// The pass hands a rep ONE task for a deal with several findings.
//
// This is the whole of #4004: a deal with four problems produced four findings,
// and a rep clearing them cleared the same deal four times. One task, several
// findings filed under it.
func TestTheNightlyPassBundlesADealsFindingsIntoOneTask(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the nightly check: %v", err)
	}

	tasks := e.bundledTasks(t)
	if len(tasks) != 1 {
		t.Fatalf("the pass minted %d task(s), want exactly 1 for the one deal the "+
			"fixture faults: %+v", len(tasks), tasks)
	}
	task := tasks[0]
	if task.findings < 2 {
		t.Fatalf("the task carries %d finding(s); this suite is about BUNDLING, so it "+
			"needs a deal the scan faults more than once — the fixture's faulted deal "+
			"no longer produces several findings and the assertion below cannot see a "+
			"grouping bug it would have caught", task.findings)
	}

	// THE assertion this test turns on, because a count of tasks cannot see the
	// grouping at all.
	//
	// OpenTaskFor re-finds the cycle's first task for a subject whatever the
	// seam grouped by, so a version minting one task PER FINDING still ends with
	// one task row and three items — the store absorbs the bug and every count
	// agrees. What does not agree is the subject line: it takes the size of the
	// group its task was minted for. Grouped by subject it says three; grouped
	// by finding it says one, on a task that then collects two more.
	want := bundleTaskSubject(task.findings)
	if task.subject != want {
		t.Errorf("the task carrying %d findings is titled %q, want %q — the subject "+
			"line is written from the group the task was minted for, so a wrong "+
			"count here means the findings were not grouped by their subject",
			task.findings, task.subject, want)
	}

	if task.dealID == nil {
		t.Error("the task is linked to no deal, so it renders on nobody's record and " +
			"a rep opening it cannot see what it is about")
	}
}

// The task goes to the DEAL'S OWNER, not to the system principal the pass runs
// as.
//
// activities.taskAssignee leaves the column NULL for a system actor that names
// nobody, and an unassigned remediation task waits in the unassigned queue that
// nobody opens. A task about a rep's own deal that never reaches their list is
// the failure this whole feature would quietly have.
func TestTheBundledTaskIsAssignedToTheDealsOwner(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the nightly check: %v", err)
	}

	tasks := e.bundledTasks(t)
	if len(tasks) != 1 {
		t.Fatalf("want 1 task, got %d", len(tasks))
	}
	if tasks[0].assignee == nil {
		t.Fatal("the bundled task is unassigned — it waits in the unassigned queue " +
			"rather than reaching the rep whose deal it is about")
	}
	if *tasks[0].assignee != e.Rep1 {
		t.Errorf("the task is assigned to %s, want the deal's owner %s",
			*tasks[0].assignee, e.Rep1)
	}
}

// A SECOND pass mints a second task, and that is the design rather than a leak.
//
// The cycle is the run, so tomorrow night is a new cycle and a finding still
// present is a fresh ask. What must NOT happen is two tasks from one pass, or
// the second pass filing its findings onto the first night's task — the first is
// the duplication the cycle exists to prevent, the second would leave a rep
// looking at a task whose findings kept changing under them.
func TestASecondPassAdoptsTheTaskTheFirstOneLeftOpen(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the first check: %v", err)
	}
	first := e.bundledTasks(t)
	if len(first) != 1 {
		t.Fatalf("the first pass minted %d task(s), want 1", len(first))
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the second check: %v", err)
	}
	second := e.bundledTasks(t)
	if len(second) != 1 {
		t.Fatalf("after two passes there are %d open task(s), want 1. The finding is the same "+
			"one, still unanswered — asking again as a second identical row does not make it "+
			"more likely to be answered, and five of them is what a rep actually found", len(second))
	}
	if second[0].id != first[0].id {
		t.Errorf("the second pass filed onto task %s, want the first night's %s",
			second[0].id, first[0].id)
	}
}

func TestAFindingStillTrueAfterSomebodyAnsweredItIsAskedAgain(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the first check: %v", err)
	}
	first := e.bundledTasks(t)
	if len(first) != 1 {
		t.Fatalf("the first pass minted %d task(s), want 1", len(first))
	}
	// The rep answers it. The condition is still there — the scan re-observes
	// it — and that is a new question, not the old one repeated.
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE activity SET is_done = true, done_at = now() WHERE id = $1`, first[0].id); err != nil {
		t.Fatalf("marking the task done: %v", err)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the second check: %v", err)
	}
	second := e.bundledTasks(t)
	if len(second) != 1 {
		t.Fatalf("%d open task(s) after the answered one, want 1", len(second))
	}
	if second[0].id == first[0].id {
		t.Error("the pass adopted a task somebody had already completed — a finding that " +
			"survived being answered is a question nobody has answered yet")
	}
}

func TestTheDuplicatesAnEarlierBuildLeftBehindAreSettled(t *testing.T) {
	e := setupAssuranceJob(t)

	// One real pass, then two more tasks filed under cycles of their own —
	// which is exactly what the cycle-scoped build left behind, one per night.
	// The task rows are minted by the real pass so they carry the subject line,
	// origin and deal link production writes; only their FILING is seeded.
	if err := e.run(t); err != nil {
		t.Fatalf("the first check: %v", err)
	}
	live := e.bundledTasks(t)
	if len(live) != 1 {
		t.Fatalf("the seeding pass left %d task(s), want 1", len(live))
	}
	original := live[0]

	for range 2 {
		if _, err := e.Pool.Exec(context.Background(), `
			WITH night AS (
			    INSERT INTO assurance_cycle (id, scope, opened_by, closed_at)
			    VALUES (gen_random_uuid(), 'seeded:' || gen_random_uuid()::text, 'system:test', now())
			    RETURNING id
			), copy AS (
			    INSERT INTO activity (kind, subject, source, captured_by, origin)
			    SELECT a.kind, a.subject, a.source, a.captured_by, a.origin
			      FROM activity a WHERE a.id = $1
			    RETURNING id
			), link AS (
			    INSERT INTO activity_link (activity_id, entity_type, deal_id)
			    SELECT (SELECT id FROM copy), 'deal', $2
			)
			INSERT INTO assurance_task_item
			       (cycle_id, exception_id, task_activity_id, subject_kind, subject_id, state)
			SELECT (SELECT id FROM night), i.exception_id, (SELECT id FROM copy),
			       i.subject_kind, i.subject_id, 'open'
			  FROM assurance_task_item i WHERE i.task_activity_id = $1 LIMIT 1`,
			original.id, original.dealID); err != nil {
			t.Fatalf("seeding an earlier night's task: %v", err)
		}
	}
	if got := e.bundledTasks(t); len(got) != 3 {
		t.Fatalf("seeded %d open tasks, want 3 — the fixture does not hold the duplicate shape", len(got))
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the reconciling check: %v", err)
	}
	after := e.bundledTasks(t)
	if len(after) != 1 {
		t.Fatalf("%d open task(s) after the sweep met three duplicates, want 1", len(after))
	}

	// Archived, not deleted: what was asked stays readable.
	var archived int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM activity WHERE kind = 'task' AND archived_at IS NOT NULL`).Scan(&archived); err != nil {
		t.Fatalf("counting the settled duplicates: %v", err)
	}
	if archived != 2 {
		t.Errorf("%d task(s) archived, want 2 — the extras were dropped rather than settled", archived)
	}
}

func TestATaskRelinkedToAnotherDealIsNeitherAdoptedNorArchived(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the first check: %v", err)
	}
	first := e.bundledTasks(t)
	if len(first) != 1 {
		t.Fatalf("the first pass minted %d task(s), want 1", len(first))
	}

	// Somebody moves the task to another deal. assurance_task_item still
	// records where it was FILED; the link is where it lives now.
	var other ids.UUID
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT id FROM deal WHERE id <> $1 ORDER BY created_at LIMIT 1`,
		*first[0].dealID).Scan(&other); err != nil {
		t.Fatalf("finding a second deal to move it to: %v", err)
	}
	if _, err := e.Pool.Exec(context.Background(),
		`UPDATE activity_link SET deal_id = $2 WHERE activity_id = $1`, first[0].id, other); err != nil {
		t.Fatalf("relinking the task: %v", err)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the second check: %v", err)
	}

	var archived bool
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT archived_at IS NOT NULL FROM activity WHERE id = $1`, first[0].id).Scan(&archived); err != nil {
		t.Fatalf("reading the moved task: %v", err)
	}
	if archived {
		t.Error("the sweep archived a task somebody had moved to another deal — the bundling row " +
			"says where it was filed, and the link says where it lives")
	}
	// This deal is owed a task again, and it must be a NEW one: the old task
	// belongs to the deal somebody moved it to, and filing this deal's findings
	// under it would hang them off that deal's row.
	var mine *bundledTask
	for _, task := range e.bundledTasks(t) {
		if task.dealID != nil && *task.dealID == *first[0].dealID {
			mine = &task
		}
	}
	if mine == nil {
		t.Fatal("the deal lost its task entirely when the old one was moved away")
	}
	if mine.id == first[0].id {
		t.Error("the sweep adopted a task that now belongs to another deal")
	}
}

// Every cycle the pass opens is CLOSED when it finishes.
//
// A partial unique index admits one open cycle per scope, and a cycle left open
// is one nothing closes later — the scope is a run id nobody revisits. The cost
// is not this night: it is that assurance_cycle accumulates open rows for ever,
// and any future move to a longer-lived scope finds them in the way.
func TestThePassClosesTheCycleItOpened(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the nightly check: %v", err)
	}

	var open int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM assurance_cycle WHERE closed_at IS NULL`).Scan(&open); err != nil {
		t.Fatalf("counting the open cycles: %v", err)
	}
	if open != 0 {
		t.Errorf("%d cycle(s) left open after the pass finished — the next pass over "+
			"the same scope is refused by the partial unique index", open)
	}
}

// The task is filed as system_remediation, so asking about a silent deal does
// not make it look freshly touched.
//
// This is the feedback loop migration 1788386600 was written to prevent, before
// this writer existed: every recency reading folds the newest activity into
// last_activity_at, and the four helpers carve out exactly this origin. Left as
// the default `human`, buyer_silent would stop firing on a deal it had just
// faulted, CloseCleared would close its own finding as condition_cleared, and
// the engine would switch itself off one deal at a time with nothing failing.
func TestTheBundledTaskIsNotBuyerActivity(t *testing.T) {
	e := setupAssuranceJob(t)

	if err := e.run(t); err != nil {
		t.Fatalf("the nightly check: %v", err)
	}

	var origin string
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT origin FROM activity WHERE kind = 'task'`).Scan(&origin); err != nil {
		t.Fatalf("reading the task's origin: %v", err)
	}
	if origin != "system_remediation" {
		t.Errorf("the bundled task's origin is %q, want system_remediation — as %q it "+
			"counts as buyer activity, and the deal this task ASKS ABOUT reads as "+
			"freshly touched by the asking", origin, origin)
	}

	// The consequence, asserted rather than inferred: the deal the task is
	// filed against must not have had its recency moved by the filing.
	var moved bool
	if err := e.Pool.QueryRow(context.Background(), `
		SELECT EXISTS (
		    SELECT 1 FROM deal d
		      JOIN activity_link l ON l.deal_id = d.id
		      JOIN activity a ON a.id = l.activity_id AND a.kind = 'task'
		     WHERE d.last_activity_at = a.occurred_at)`).Scan(&moved); err != nil {
		t.Fatalf("reading the deal's recency: %v", err)
	}
	if moved {
		t.Error("the deal's last_activity_at is the remediation task's own timestamp — " +
			"the rule that noticed the silence will not fire again")
	}
}

// A finding somebody DEFERRED is not asked again tonight.
//
// remind_later leaves the exception open on purpose, and CloseCleared carves the
// same rows out of its own sweep. Without the matching clause here a rep who
// deferred a finding for two weeks is handed a fresh task about it every night
// for fourteen nights, which makes the deferral a no-op against this feature.
func TestADeferredFindingIsNotBundledAgain(t *testing.T) {
	e := setupAssuranceJob(t)

	// One pass, so there are findings to defer.
	if err := e.run(t); err != nil {
		t.Fatalf("the first check: %v", err)
	}
	if got := len(e.bundledTasks(t)); got != 1 {
		t.Fatalf("the first pass minted %d task(s), want 1", got)
	}

	// The rep answers "remind me later" on every one of them.
	if _, err := e.Pool.Exec(context.Background(), `
		INSERT INTO assurance_resolution (exception_id, actor_id, outcome, reason, remind_at, captured_by)
		SELECT id, $1, 'remind_later', 'not this week', now() + interval '14 days', 'human:test'
		  FROM assurance_exception WHERE status = 'open'`, e.Rep1); err != nil {
		t.Fatalf("deferring the findings: %v", err)
	}

	if err := e.run(t); err != nil {
		t.Fatalf("the second check: %v", err)
	}
	if got := len(e.bundledTasks(t)); got != 1 {
		t.Errorf("after deferring every finding the second pass left %d task(s), want "+
			"the original 1 — a deferred finding was bundled into a new task, so the "+
			"rep is asked again tonight and every night until the deferral expires", got)
	}
}

// The cycle closes even when the pass FAILS after opening it.
//
// The happy-path test above cannot see this: it is the failure paths that leave
// a cycle behind, and a cycle left behind is left behind for ever — its scope is
// a run id nobody revisits, and the partial unique index refuses a second open
// cycle over the same scope. The failure is injected at the exception read
// because that is the return the close was originally registered BELOW, so this
// fails against that ordering and passes against the current one.
func TestAFailedPassStillClosesItsCycle(t *testing.T) {
	e := setupAssuranceJob(t)

	run, err := e.runWithFindings(t, func(context.Context, pgx.Tx, ids.UUID) ([]assurance.Exception, error) {
		return nil, errors.New("the findings could not be read")
	})
	if err == nil {
		t.Fatal("a pass whose read failed reported success")
	}
	if run != 0 {
		t.Errorf("minted %d task(s) from a failed read, want 0", run)
	}

	var open int
	if err := e.Pool.QueryRow(context.Background(),
		`SELECT count(*) FROM assurance_cycle WHERE closed_at IS NULL`).Scan(&open); err != nil {
		t.Fatalf("counting the open cycles: %v", err)
	}
	if open != 0 {
		t.Errorf("%d cycle(s) left open after the pass failed — nothing closes them "+
			"later, and the next pass over that scope is refused by the partial "+
			"unique index", open)
	}
}

// runWithFindings drives the bundler directly with a read of the caller's
// choosing, which is how a failing read is reached: the worker's own read is
// wired to the real one, and a failure there is not otherwise producible.
func (e *assuranceJobEnv) runWithFindings(
	t *testing.T,
	read func(context.Context, pgx.Tx, ids.UUID) ([]assurance.Exception, error),
) (int, error) {
	t.Helper()
	ctx := principal.WithCorrelationID(
		principal.WithActor(
			principal.WithWorkspaceID(context.Background(), e.WS),
			principal.Principal{Type: principal.PrincipalSystem, ID: assuranceActor}),
		ids.NewV7())
	return bundleRunFindings(ctx, bundleDeps{
		bundles:    assurance.NewStore(InstallationDB(e.Pool)),
		activities: activities.NewStore(InstallationDB(e.Pool)),
		exceptions: read,
	}, ids.NewV7())
}

// bundledTasks reads back what the pass minted, joined to the finding count each
// task stands for.
//
// It reads the ACTIVITY table rather than assurance_task_item alone, because the
// question this suite asks is whether a rep gets a task — a bundle row pointing
// at an activity that was never written would satisfy the module's own tests and
// none of these.
func (e *assuranceJobEnv) bundledTasks(t *testing.T) []bundledTask {
	t.Helper()
	// activity_link is polymorphic by COLUMN, not by a shared entity_id: a shape
	// CHECK admits exactly one of contact_id/company_id/deal_id/lead_id/
	// project_id per row. A finding's subject is a deal, so deal_id is the one
	// this reads — and reading the wrong column would return NULL for every row
	// rather than failing, which is why it is named rather than coalesced.
	rows, err := e.Pool.Query(context.Background(), `
		SELECT a.id, a.subject, a.assignee_id, l.deal_id, count(i.id)
		  FROM assurance_task_item i
		  JOIN activity a ON a.id = i.task_activity_id
		  LEFT JOIN activity_link l ON l.activity_id = a.id
		 WHERE a.kind = 'task' AND a.archived_at IS NULL AND a.is_done = false
		 GROUP BY a.id, a.subject, a.assignee_id, l.deal_id
		 ORDER BY a.created_at`)
	if err != nil {
		t.Fatalf("reading the bundled tasks: %v", err)
	}
	defer rows.Close()

	var out []bundledTask
	for rows.Next() {
		var task bundledTask
		var subject *string
		if err := rows.Scan(&task.id, &subject, &task.assignee, &task.dealID, &task.findings); err != nil {
			t.Fatalf("scanning a bundled task: %v", err)
		}
		if subject != nil {
			task.subject = *subject
		}
		out = append(out, task)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("walking the bundled tasks: %v", err)
	}
	return out
}
