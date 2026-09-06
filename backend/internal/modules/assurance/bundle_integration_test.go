// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assurance

// Bundling against a real database.
//
// The whole design rests on constraints Postgres evaluates: the unique pair that
// makes a re-run a no-op, the partial index that admits one open cycle per
// scope, and the closed-cycle predicate inside the insert. None of it is
// reachable from a unit test, and each can be wrong in a way that compiles and
// quietly mints a second task somebody has to notice.

import (
	"context"
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A SECOND pass over the same cycle mints nothing.
//
// This is the property #4004 asked for by name: the constraint is the
// idempotency, not a cache. A dedupe in Redis answers this correctly until it is
// flushed and then answers it wrong — quietly, by filing the same finding twice.
func TestBundlingTheSameFindingTwiceIsANoOp(t *testing.T) {
	e := setupScan(t)
	cycle := e.openCycle(t, t.Name())
	exception := e.seedException(t, "bundle-once", ids.NewV7())
	task := e.seedTask(t, "Fix the pipeline data")

	first, err := e.store.BundleException(e.as(), BundleInput{
		CycleID: cycle, ExceptionID: exception, TaskActivityID: task,
	})
	if err != nil || !first {
		t.Fatalf("first bundle: applied=%v err=%v, want it to land", first, err)
	}

	second, err := e.store.BundleException(e.as(), BundleInput{
		CycleID: cycle, ExceptionID: exception, TaskActivityID: task,
	})
	if err != nil {
		t.Fatalf("a re-run must not be an error: %v", err)
	}
	if second {
		t.Error("a second bundle of the same finding reported itself as new")
	}
	if n := e.itemCount(t, cycle); n != 1 {
		t.Errorf("items = %d, want 1 — the constraint did not hold the re-run", n)
	}
}

// Several findings about ONE deal join ONE task.
//
// That is what bundling means. Without it a deal with four problems produces
// four tasks, and a rep clearing them clears the same deal four times.
func TestFindingsAboutOneDealShareOneTask(t *testing.T) {
	e := setupScan(t)
	cycle := e.openCycle(t, t.Name())
	deal := ids.NewV7()
	task := e.seedTask(t, "Fix this deal")

	for _, key := range []string{"missing-close-date", "stale-amount", "no-owner"} {
		exception := e.seedException(t, key, deal)
		if _, err := e.store.BundleException(e.as(), BundleInput{
			CycleID: cycle, ExceptionID: exception, TaskActivityID: task,
		}); err != nil {
			t.Fatalf("bundling %s: %v", key, err)
		}
	}

	// Three findings, three items, and ONE task between them.
	if n := e.itemCount(t, cycle); n != 3 {
		t.Errorf("items = %d, want 3 — each finding is recorded", n)
	}
	if n := e.taskCount(t, cycle); n != 1 {
		t.Errorf("distinct tasks = %d, want 1 — the deal's findings did not bundle", n)
	}

	// And the read that makes the bundle work: a fourth finding asks what task
	// this deal already has and gets the one above rather than minting another.
	got, found, err := e.store.OpenTaskFor(e.as(), cycle, "deal", deal)
	if err != nil {
		t.Fatalf("reading the deal's task: %v", err)
	}
	if !found || got != task {
		t.Errorf("OpenTaskFor = %v (found=%v), want the task the first finding minted (%v)", got, found, task)
	}
}

// A CLOSED cycle bundles nothing. Its window is over, and a finding after it
// belongs to the next pass.
func TestAClosedCycleBundlesNothing(t *testing.T) {
	e := setupScan(t)
	cycle := e.openCycle(t, t.Name())
	if err := e.store.CloseCycle(e.as(), cycle); err != nil {
		t.Fatalf("closing the cycle: %v", err)
	}
	exception := e.seedException(t, "after-the-bell", ids.NewV7())
	task := e.seedTask(t, "Too late")

	applied, err := e.store.BundleException(e.as(), BundleInput{
		CycleID: cycle, ExceptionID: exception, TaskActivityID: task,
	})
	// A quiet no-op rather than an error: the pass ended, which is not a fault
	// of the finding that arrived after it.
	if err != nil {
		t.Fatalf("bundling into a closed cycle should be a quiet no-op, got: %v", err)
	}
	if applied {
		t.Error("a closed cycle accepted a finding")
	}
	if n := e.itemCount(t, cycle); n != 0 {
		t.Errorf("items = %d, want 0", n)
	}
}

// ONE open cycle per scope.
//
// Two would each mint their own task for the same deal — the duplication this
// whole table exists to prevent, arriving one level up where the row constraint
// cannot see it.
func TestOnlyOneCycleIsOpenPerScope(t *testing.T) {
	e := setupScan(t)
	first := e.openCycle(t, t.Name())

	_, err := e.store.OpenCycle(e.as(), t.Name())

	if !errors.Is(err, errCycleAlreadyOpen) {
		t.Fatalf("second open over one scope: err = %v, want it refused", err)
	}

	// Closing the first frees the scope: the next pass is a new cycle, not a
	// reopening of the old one.
	if err := e.store.CloseCycle(e.as(), first); err != nil {
		t.Fatalf("closing: %v", err)
	}
	second, err := e.store.OpenCycle(e.as(), t.Name())
	if err != nil {
		t.Fatalf("opening after the first closed: %v", err)
	}
	if second == first {
		t.Error("the second open returned the first cycle rather than a new one")
	}
}

// Closing a cycle twice does not move the moment it closed.
func TestClosingACycleTwiceIsRefused(t *testing.T) {
	e := setupScan(t)
	cycle := e.openCycle(t, t.Name())
	if err := e.store.CloseCycle(e.as(), cycle); err != nil {
		t.Fatalf("first close: %v", err)
	}

	err := e.store.CloseCycle(e.as(), cycle)

	if !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("second close: err = %v, want ErrNotFound — an already-closed cycle is not open to close", err)
	}
}

// The row records the EXCEPTION's subject, not anything a caller chose.
//
// This is what makes the exclusion constraint mean "one task per subject". The
// caller passes no subject at all, so the only way two findings about one deal
// can land under two tasks is if this derivation is wrong — and it was: the
// first version took subject_kind and subject_id straight off BundleInput, so
// two exceptions about deal S could be filed under subjects X and Y, clear the
// constraint, and mint the second task the whole table exists to prevent.
func TestTheBundledRowCarriesTheExceptionsOwnSubject(t *testing.T) {
	e := setupScan(t)
	cycle := e.openCycle(t, t.Name())
	deal := ids.NewV7()
	exception := e.seedException(t, "subject-is-derived", deal)
	task := e.seedTask(t, "The deal's task")

	if _, err := e.store.BundleException(e.as(), BundleInput{
		CycleID: cycle, ExceptionID: exception, TaskActivityID: task,
	}); err != nil {
		t.Fatalf("bundling: %v", err)
	}

	var kind string
	var subject ids.UUID
	if err := e.owner.QueryRow(context.Background(),
		`SELECT subject_kind, subject_id FROM assurance_task_item WHERE exception_id = $1`,
		exception).Scan(&kind, &subject); err != nil {
		t.Fatalf("reading the bundled row: %v", err)
	}
	if kind != subjectDeal || subject != deal {
		t.Errorf("the row records (%s, %v), want the exception's own (%s, %v)",
			kind, subject, subjectDeal, deal)
	}

	// And the read agrees: the task is findable under the subject the exception
	// is actually about, which is the lookup a second finding does.
	got, found, err := e.store.OpenTaskFor(e.as(), cycle, subjectDeal, deal)
	if err != nil {
		t.Fatalf("reading the task for the exception's subject: %v", err)
	}
	if !found || got != task {
		t.Errorf("OpenTaskFor(%v) = %v (found=%v), want %v", deal, got, found, task)
	}
}

// openCycle opens one over a scope named for the calling test.
//
// The scope is the test's own name because only ONE cycle may be open per scope
// and these tests share a database — a shared literal would have the first test
// to run block every other, which says nothing about the product and everything
// about the fixture.
func (e *scanEnv) openCycle(t *testing.T, scope string) ids.UUID {
	t.Helper()
	id, err := e.store.OpenCycle(e.as(), scope)
	if err != nil {
		t.Fatalf("opening a cycle over %q: %v", scope, err)
	}
	return id
}

// subjectDeal is the one subject kind these tests use. The table admits four,
// and bundling treats them identically — a fixture cycling through all four
// would assert that sameness rather than anything about bundling.
const subjectDeal = "deal"

// seedException writes one finding ABOUT a named subject.
//
// The subject is a parameter because it is the fact bundling turns on: the
// insert reads subject_kind and subject_id off this row, and the exclusion
// constraint compares that pair. An earlier version stored a throwaway id here
// while the tests bundled a different one, so every assertion below was made
// against a subject no exception was ever about.
//
// Through SQL rather than the scanner: this file is about BUNDLING, and driving
// a rule to produce an exception would make every test here depend on that
// rule's own conditions staying true.
func (e *scanEnv) seedException(t *testing.T, key string, subject ids.UUID) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO assurance_exception
		    (id, logical_key, type, subject_kind, subject_id, severity, status,
		     claim, observed, evidence_refs, first_seen_at, last_seen_at, captured_by)
		VALUES ($1, $2, 'test_rule', $3, $4, 'medium', 'open',
		        '{}'::jsonb, '{}'::jsonb, '[]'::jsonb, now(), now(), 'human:test')`,
		id, key+"-"+id.String(), subjectDeal, subject); err != nil {
		t.Fatalf("seeding the exception: %v", err)
	}
	return id
}

// seedTask writes the activity a bundle files findings under. In production the
// caller mints it through LogActivityTx; this module reaches for no sibling, so
// the fixture supplies the id the same way a caller would.
func (e *scanEnv) seedTask(t *testing.T, subject string) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := e.owner.Exec(context.Background(), `
		INSERT INTO activity (id, kind, subject, occurred_at, thread_key, source, captured_by)
		VALUES ($1, 'task', $2, now(), gen_random_uuid()::text, 'manual', 'human:test')`,
		id, subject); err != nil {
		t.Fatalf("seeding the task: %v", err)
	}
	return id
}

func (e *scanEnv) itemCount(t *testing.T, cycle ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM assurance_task_item WHERE cycle_id = $1`, cycle).Scan(&n); err != nil {
		t.Fatalf("counting items: %v", err)
	}
	return n
}

func (e *scanEnv) taskCount(t *testing.T, cycle ids.UUID) int {
	t.Helper()
	var n int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(DISTINCT task_activity_id) FROM assurance_task_item WHERE cycle_id = $1`,
		cycle).Scan(&n); err != nil {
		t.Fatalf("counting tasks: %v", err)
	}
	return n
}

// One subject gets ONE task, and a second is refused by the database.
//
// This is the other half of the bundling rule, and the half the first draft of
// the migration had backwards: it forbade a second FINDING about one deal
// (which is the feature) while allowing a second TASK (which is the defect).
// Several findings sharing a task is proven above; this proves the constraint
// that stops them drifting onto two.
func TestASubjectCannotAcquireASecondTaskInOneCycle(t *testing.T) {
	e := setupScan(t)
	cycle := e.openCycle(t, t.Name())
	deal := ids.NewV7()
	first := e.seedTask(t, "The deal's task")
	second := e.seedTask(t, "A second task for the same deal")

	if _, err := e.store.BundleException(e.as(), BundleInput{
		CycleID: cycle, ExceptionID: e.seedException(t, "first", deal),
		TaskActivityID: first,
	}); err != nil {
		t.Fatalf("the first finding: %v", err)
	}

	_, err := e.store.BundleException(e.as(), BundleInput{
		CycleID: cycle, ExceptionID: e.seedException(t, "second", deal),
		TaskActivityID: second,
	})

	if err == nil {
		t.Fatal("one deal acquired two tasks in one cycle — the bundle split in half")
	}
	// The first task stands: a refused second write leaves the bundle intact.
	if n := e.taskCount(t, cycle); n != 1 {
		t.Errorf("distinct tasks = %d, want 1", n)
	}
}
