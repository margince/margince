// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package assurance

// The preview's promises are about what is NOT in the database afterwards, and
// about agreeing with the pass it previews. Neither is visible to a unit test:
// "no run row was written" is a count, and the agreement is only meaningful
// against a real Scan writing real rows.

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// previewSubjects is one deal that trips two rules, so a preview has something
// to count and the counts have shape.
func previewSubjects(owner string) SubjectsFunc {
	past := time.Now().UTC().AddDate(0, 0, -30)
	silent := time.Now().UTC().AddDate(0, 0, -120)
	amount := int64(500000)
	return func(context.Context, pgx.Tx) ([]Subject, error) {
		return []Subject{{
			DealID: "11111111-1111-7111-8111-111111111111", Owner: owner,
			AmountMinor: &amount, Currency: "EUR",
			ExpectedClose: &past, LastInboundAt: &silent,
			Category: categoryCommit, StageName: "Negotiation",
		}}, nil
	}
}

func previewCoverage() CoverageFunc {
	return func(_ context.Context, _ pgx.Tx, now time.Time) []SourceCoverage {
		return []SourceCoverage{
			{Source: "mail", State: CoverageChecked, CheckedThrough: &now},
			{Source: "offers", State: CoverageChecked, CheckedThrough: &now},
		}
	}
}

// A preview leaves the database exactly as it found it.
//
// This is the whole guarantee. A preview that wrote even the run row would put
// a reading nobody should act on in front of every surface that reads the
// latest run — and would enrol the workspace in the nightly cadence by being
// looked at, which is the decision it exists to ASK for.
func TestAPreviewRecordsNothing(t *testing.T) {
	// NOT parallel, and the reason is the assertion itself: assurance tables
	// carry no workspace column — the module is installation-wide — so counting
	// them counts every sibling test's rows too. Go holds parallel tests until
	// the sequential ones finish, which is what makes this count mean what it
	// says. Scoping it to this test's own deal instead would leave the run and
	// coverage rows, the ones a preview would most plausibly leak, unwatched.
	e := setupScan(t)
	ctx := e.as()

	scanner := NewScanner(e.store, previewSubjects(e.rep.String()), previewCoverage(), DefaultConfig())

	before := e.countRows(t)
	preview, err := scanner.Preview(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	after := e.countRows(t)

	// A preview that read nothing also writes nothing, and would pass the
	// comparison below without proving anything. It has to have found something
	// first.
	if len(preview.Counts) == 0 || len(preview.Coverage) == 0 {
		t.Fatal("the preview found no findings or no coverage, so it may not have read " +
			"anything at all — and a preview that looked at nothing writes nothing " +
			"for reasons this test is not about")
	}

	for table, was := range before {
		if after[table] != was {
			t.Errorf("%s went from %d rows to %d across a preview", table, was, after[table])
		}
	}
}

// Before anybody starts it, a workspace says so; afterwards it says so too.
//
// `started` is what the panel reads to know whether it is offering a first
// check or a recheck, and the nightly sweep reads the same fact to decide
// whether this workspace is its business at all.
func TestAWorkspaceSaysWhetherItHasEverBeenChecked(t *testing.T) {
	// Sequential for the same reason TestAPreviewRecordsNothing is: enrolment
	// is read off assurance_run, which carries no workspace column, so a
	// sibling test's pass would answer this one's question for it.
	e := setupScan(t)
	ctx := e.as()

	scanner := NewScanner(e.store, previewSubjects(e.rep.String()), previewCoverage(), DefaultConfig())

	before, err := scanner.Preview(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	if before.Started {
		t.Error("a workspace with no runs reported itself started — the sweep would " +
			"check it before anybody chose to be checked")
	}

	if _, err := scanner.Scan(ctx, time.Now().UTC(), nil); err != nil {
		t.Fatalf("the first pass: %v", err)
	}

	after, err := scanner.Preview(ctx, time.Now().UTC())
	if err != nil {
		t.Fatalf("previewing after the pass: %v", err)
	}
	if !after.Started {
		t.Error("a workspace that has run a pass reported itself unstarted — the sweep " +
			"would stop checking an installation that was already being checked")
	}
}

// The preview counts what the pass then raises.
//
// A preview promising findings the run does not raise is worse than no preview:
// somebody consents to one queue and gets another. This is the test that fails
// if the preview and the pass ever stop agreeing — which is what a second walk
// of the rules would cause.
func TestAPreviewCountsWhatTheRunThenRaises(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	scanner := NewScanner(e.store, previewSubjects(e.rep.String()), previewCoverage(), DefaultConfig())
	at := time.Now().UTC()

	preview, err := scanner.Preview(ctx, at)
	if err != nil {
		t.Fatalf("previewing: %v", err)
	}
	previewed := 0
	for _, c := range preview.Counts {
		previewed += c.Count
	}

	result, err := scanner.Scan(ctx, at, nil)
	if err != nil {
		t.Fatalf("the pass: %v", err)
	}
	if previewed != result.Findings {
		t.Errorf("the preview promised %d findings and the pass raised %d", previewed, result.Findings)
	}
	if preview.EligibleDeals != result.EligibleDeals {
		t.Errorf("the preview counted %d eligible deals and the pass counted %d",
			preview.EligibleDeals, result.EligibleDeals)
	}
	if preview.Readiness != result.Readiness {
		t.Errorf("the preview predicted readiness %q and the pass reached %q",
			preview.Readiness, result.Readiness)
	}
}

// A pass a human asked for names them; the nightly one names nobody.
//
// captured_by stays the system on both, because the pass reads the whole
// pipeline and a run captured_by a manager would sign them to every exception
// it raises. Who ASKED is the separate fact, and this is where it lands.
func TestAPassRecordsWhoAskedForItAndTheNightlyOneDoesNot(t *testing.T) {
	t.Parallel()
	e := setupScan(t)
	ctx := e.as()

	scanner := NewScanner(e.store, previewSubjects(e.rep.String()), previewCoverage(), DefaultConfig())

	nightly, err := scanner.Scan(ctx, time.Now().UTC(), nil)
	if err != nil {
		t.Fatalf("the nightly pass: %v", err)
	}
	asker := e.rep.String()
	asked, err := scanner.Scan(ctx, time.Now().UTC().Add(time.Minute), &asker)
	if err != nil {
		t.Fatalf("the requested pass: %v", err)
	}

	if got := e.requesterOf(t, nightly.RunID.String()); got != nil {
		t.Errorf("the nightly run names %q as having asked for it; nobody did", *got)
	}
	got := e.requesterOf(t, asked.RunID.String())
	if got == nil || *got != asker {
		t.Errorf("the requested run recorded %v as the asker, want %q", got, asker)
	}
}

// countRows reads every table a pass writes. Compared across the preview rather
// than against zero: these tables carry no workspace column, so the rows a
// sibling test left are in the count too, and only the DELTA is this preview's.
func (e *scanEnv) countRows(t *testing.T) map[string]int {
	t.Helper()
	ctx := e.as()
	tables := []string{"assurance_run", "assurance_exception", "assurance_run_finding", "assurance_source_coverage"}
	out := map[string]int{}
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		for _, table := range tables {
			var count int
			if err := tx.QueryRow(ctx,
				`SELECT count(*) FROM `+pgx.Identifier{table}.Sanitize()).Scan(&count); err != nil {
				return err
			}
			out[table] = count
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *scanEnv) requesterOf(t *testing.T, runID string) *string {
	t.Helper()
	ctx := e.as()
	var out *string
	if err := e.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`SELECT requested_by FROM assurance_run WHERE id = $1`, runID).Scan(&out)
	}); err != nil {
		t.Fatal(err)
	}
	return out
}
