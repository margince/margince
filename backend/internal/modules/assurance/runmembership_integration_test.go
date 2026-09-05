// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assurance

// "Which run last confirmed this exception."
//
// The run row says how much a night checked; the exception row says what is
// open now. Neither answers the question a manager doubting a finding asks
// first: is this still true, or a leftover from a night before the deal moved?
//
// These cases are about the membership itself rather than the scan around it,
// so they call the writer directly with the keys the scan would have collected.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// recordSeen upserts one finding and records that a run observed it.
//
// No instant is passed: the membership takes observed_at from the run itself,
// which is the whole point — a caller-supplied one is a second copy of a fact
// the run already holds.
func recordSeen(t *testing.T, e *scanEnv, runID ids.UUID, f Finding) {
	t.Helper()
	ctx := e.as()
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := e.store.UpsertException(ctx, tx, f, e.rep.String()); err != nil {
			return err
		}
		return e.store.RecordRunFindings(ctx, tx, runID, []string{LogicalKey(f)})
	}); err != nil {
		t.Fatalf("recording the observation: %v", err)
	}
}

// startRun opens one run and hands back its id.
func startRun(t *testing.T, e *scanEnv, at time.Time) ids.UUID {
	t.Helper()
	var runID ids.UUID
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		var err error
		runID, err = e.store.StartRun(ctx, tx, at)
		return err
	}); err != nil {
		t.Fatalf("starting a run: %v", err)
	}
	return runID
}

// lastConfirmedBy reads which run most recently observed one exception.
func lastConfirmedBy(t *testing.T, e *scanEnv, logicalKey string) (ids.UUID, time.Time, bool) {
	t.Helper()
	var runID ids.UUID
	var at time.Time
	found := true
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		err := tx.QueryRow(ctx, `
			SELECT m.run_id, m.observed_at
			  FROM assurance_run_finding m
			  JOIN assurance_exception e ON e.id = m.exception_id
			 WHERE e.logical_key = $1
			 ORDER BY m.observed_at DESC
			 LIMIT 1`, logicalKey).Scan(&runID, &at)
		if err != nil && err.Error() == "no rows in result set" {
			found = false
			return nil
		}
		return err
	}); err != nil {
		t.Fatalf("reading the membership: %v", err)
	}
	return runID, at, found
}

// The question the table exists for, answered.
//
// Two nights observe the same finding. The exception is one row across both —
// that is the identity rule these sit beside — and the membership is what says
// which night last saw it.
func TestTheLastRunToConfirmAFindingIsAnswerable(t *testing.T) {
	t.Parallel()
	e := setupScan(t)

	finding := Finding{
		Type: TypeClosePast, SubjectID: ids.NewV7().String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	first := time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
	second := time.Date(2026, 5, 15, 3, 0, 0, 0, time.UTC)

	firstRun := startRun(t, e, first)
	recordSeen(t, e, firstRun, finding)

	secondRun := startRun(t, e, second)
	// The second night observes a later date — what a real second run sees, and
	// what the exception's identity deliberately does not key on.
	finding.Observed = map[string]any{"as_of": "2026-05-15"}
	recordSeen(t, e, secondRun, finding)

	gotRun, gotAt, found := lastConfirmedBy(t, e, LogicalKey(finding))
	if !found {
		t.Fatal("no run is recorded as having seen this finding; the question this table exists " +
			"for is still unanswerable")
	}
	if gotRun != secondRun {
		t.Errorf("last confirmed by run %s, want the second night's %s", gotRun, secondRun)
	}
	if !gotAt.Equal(second) {
		t.Errorf("observed_at = %s, want the run's own as-of %s — a reading belongs to the "+
			"instant it was taken at, not to whenever the transaction committed", gotAt, second)
	}
}

// Both nights stay on record. A membership that only kept the newest would
// answer "is it still true" and lose "was it true three nights running", which
// is what tells a manager a finding is stuck rather than new.
func TestEveryRunThatSawAFindingStaysOnRecord(t *testing.T) {
	t.Parallel()
	e := setupScan(t)

	finding := Finding{
		Type: TypeClosePast, SubjectID: ids.NewV7().String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	for night := range 3 {
		at := time.Date(2026, 5, 14+night, 3, 0, 0, 0, time.UTC)
		recordSeen(t, e, startRun(t, e, at), finding)
	}

	var nights int
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT count(*) FROM assurance_run_finding m
			  JOIN assurance_exception e ON e.id = m.exception_id
			 WHERE e.logical_key = $1`, LogicalKey(finding)).Scan(&nights)
	}); err != nil {
		t.Fatal(err)
	}
	if nights != 3 {
		t.Errorf("%d runs are recorded as having seen this finding, want 3 — a finding present "+
			"three nights running is a different fact from one that appeared tonight", nights)
	}
}

// One run observing one finding is one row, however many times it was named.
//
// A rule can fire on two subjects that share a logical key, and a retry inside
// the transaction re-sends the same set. Either would double a night's count of
// what it saw.
func TestOneRunSeeingAFindingTwiceRecordsItOnce(t *testing.T) {
	t.Parallel()
	e := setupScan(t)

	finding := Finding{
		Type: TypeClosePast, SubjectID: ids.NewV7().String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	at := time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
	runID := startRun(t, e, at)
	key := LogicalKey(finding)

	ctx := e.as()
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := e.store.UpsertException(ctx, tx, finding, e.rep.String()); err != nil {
			return err
		}
		// The same key twice in one call, and then the whole call again.
		if err := e.store.RecordRunFindings(ctx, tx, runID, []string{key, key}); err != nil {
			return err
		}
		return e.store.RecordRunFindings(ctx, tx, runID, []string{key})
	}); err != nil {
		t.Fatalf("recording the observation: %v", err)
	}

	var rows int
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM assurance_run_finding WHERE run_id = $1`, runID).Scan(&rows)
	}); err != nil {
		t.Fatal(err)
	}
	if rows != 1 {
		t.Errorf("one run recorded %d observations of one finding, want 1", rows)
	}
}

// A run that saw nothing writes nothing, and does not fail.
//
// A clean night is the ordinary case, and a writer that refused an empty set
// would fail the run that had the least to report.
func TestARunThatSawNothingRecordsNothing(t *testing.T) {
	t.Parallel()
	e := setupScan(t)

	at := time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
	runID := startRun(t, e, at)
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		return e.store.RecordRunFindings(ctx, tx, runID, nil)
	}); err != nil {
		t.Fatalf("a clean night failed: %v", err)
	}

	var rows int
	if err := e.store.InTx(e.as(), func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM assurance_run_finding WHERE run_id = $1`, runID).Scan(&rows)
	}); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("a run that saw nothing recorded %d observations", rows)
	}
}

// The membership dies with its run. A run deleted for retention leaves no rows
// claiming a night that no longer exists confirmed something.
func TestMembershipDiesWithItsRun(t *testing.T) {
	t.Parallel()
	e := setupScan(t)

	finding := Finding{
		Type: TypeClosePast, SubjectID: ids.NewV7().String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	at := time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
	runID := startRun(t, e, at)
	recordSeen(t, e, runID, finding)

	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM assurance_run WHERE id = $1`, runID); err != nil {
		t.Fatalf("deleting the run: %v", err)
	}
	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM assurance_run_finding WHERE run_id = $1`, runID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d membership rows outlived their run", rows)
	}
}

// The REAL nightly path records membership.
//
// The cases above call the writer directly, which proves the writer. This
// proves the scan calls it: without this, deleting the call from scan.go leaves
// every test green while production records nothing at all.
func TestTheNightlyScanRecordsWhatItSaw(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	past := time.Now().UTC().AddDate(0, 0, -10)
	sick := Subject{
		DealID: ids.NewV7().String(), Owner: e.rep.String(),
		ExpectedClose: &past, Category: "commit", HasNextStep: true, HasEconomicBuyer: true,
	}
	scanner := NewScanner(e.store,
		func(context.Context, pgx.Tx) ([]Subject, error) { return []Subject{sick}, nil },
		checkedCoverage, DefaultConfig())

	got, err := scanner.Scan(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("the scan failed: %v", err)
	}
	if got.Findings == 0 {
		t.Fatal("the scan found nothing; this fixture cannot prove membership was recorded")
	}

	var recorded int
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT count(*) FROM assurance_run_finding WHERE run_id = $1`, got.RunID).Scan(&recorded)
	}); err != nil {
		t.Fatal(err)
	}
	if recorded != got.Findings {
		t.Errorf("the run reported %d findings and recorded %d as seen — a nightly scan that "+
			"records nothing leaves every membership question unanswerable in production",
			got.Findings, recorded)
	}
}

// A finding somebody already RESOLVED is still a finding this run observed.
//
// Filtering the lookup to open exceptions would silently drop exactly the ones
// a manager is most likely to question — "you told me this was resolved, which
// night last saw it?" — and no other case here notices.
func TestARunRecordsAFindingItSawEvenIfResolved(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	finding := Finding{
		Type: TypeClosePast, SubjectID: ids.NewV7().String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	at := time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
	runID := startRun(t, e, at)

	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		if err := e.store.UpsertException(ctx, tx, finding, e.rep.String()); err != nil {
			return err
		}
		// Somebody answers it before tonight's membership is written.
		if _, err := tx.Exec(ctx,
			`UPDATE assurance_exception SET status = 'resolved' WHERE logical_key = $1`,
			LogicalKey(finding)); err != nil {
			return err
		}
		return e.store.RecordRunFindings(ctx, tx, runID, []string{LogicalKey(finding)})
	}); err != nil {
		t.Fatalf("recording the observation: %v", err)
	}

	if _, _, found := lastConfirmedBy(t, e, LogicalKey(finding)); !found {
		t.Error("a resolved finding this run observed is recorded against no run; the question " +
			"survives the resolution, and so must the answer")
	}
}

// The membership dies with its EXCEPTION too, not only with its run.
//
// Erasure and retention both reach a finding, and a membership row outliving
// one would still assert that some night confirmed a thing no longer held.
func TestMembershipDiesWithItsException(t *testing.T) {
	t.Parallel()
	e := setupScan(t)

	finding := Finding{
		Type: TypeClosePast, SubjectID: ids.NewV7().String(), Severity: SeverityHigh,
		Claim:    map[string]any{"expected_close": "2026-04-30"},
		Observed: map[string]any{"as_of": "2026-05-14"},
	}
	at := time.Date(2026, 5, 14, 3, 0, 0, 0, time.UTC)
	runID := startRun(t, e, at)
	recordSeen(t, e, runID, finding)

	if _, err := e.owner.Exec(context.Background(),
		`DELETE FROM assurance_exception WHERE logical_key = $1`, LogicalKey(finding)); err != nil {
		t.Fatalf("deleting the exception: %v", err)
	}
	var rows int
	if err := e.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM assurance_run_finding WHERE run_id = $1`, runID).Scan(&rows); err != nil {
		t.Fatal(err)
	}
	if rows != 0 {
		t.Errorf("%d membership rows outlived the finding they point at", rows)
	}
}
