// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SubjectsFunc reads the deals a run should examine.
//
// Injected rather than imported: the deals live in another module and the read
// spans stages and activities, which is the composition layer's business.
type SubjectsFunc func(ctx context.Context, tx pgx.Tx) ([]Subject, error)

// CoverageFunc answers which sources the run could reach.
type CoverageFunc func(ctx context.Context, tx pgx.Tx, now time.Time) []SourceCoverage

// Scanner runs one nightly pass.
type Scanner struct {
	store    *Store
	subjects SubjectsFunc
	coverage CoverageFunc
	cfg      Config
}

// NewScanner wires a pass to its store and its two seams.
func NewScanner(store *Store, subjects SubjectsFunc, coverage CoverageFunc, cfg Config) *Scanner {
	return &Scanner{store: store, subjects: subjects, coverage: coverage, cfg: cfg}
}

// Result is what one pass came to.
type Result struct {
	RunID         ids.UUID
	EligibleDeals int
	Findings      int
	Readiness     string
	Status        string
}

// Scan asks every rule of every live open deal, once.
//
// It NEVER refuses to start. An upstream that could not be read makes the run
// incomplete and its readiness `checks_incomplete` — it does not make the run
// absent. Refusing would produce no record in exactly the case this pass exists
// to report, and the brief waiting on it would run without ever learning why.
func (s *Scanner) Scan(ctx context.Context, now time.Time) (Result, error) {
	var out Result
	err := s.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		runID, err := s.store.StartRun(ctx, tx, now)
		if err != nil {
			return err
		}
		out.RunID = runID

		coverage := s.coverage(ctx, tx, now)
		for _, c := range coverage {
			if err := s.store.RecordCoverage(ctx, tx, runID, c); err != nil {
				return err
			}
		}

		subjects, err := s.subjects(ctx, tx)
		if err != nil {
			// The deals could not be read at all. The run still stands and
			// says so, because a missing run is the one answer that tells
			// nobody anything.
			//
			// The RESULT says so too. Returning an empty one would leave the
			// row correct and the caller — a job that logs what the pass came
			// to — reporting nothing about the night the check could not run,
			// which is the night worth reporting.
			out.Status = StatusIncomplete
			out.Readiness = ReadinessChecksIncomplete
			return s.store.FinishRun(ctx, tx, runID, 0, 0, out.Status, out.Readiness)
		}

		var findings []Finding
		for _, subject := range subjects {
			// Counted in the LOOP, one per deal actually evaluated. Taken from
			// len(subjects) it would be the same number the query returned,
			// which makes the census assert x == x and leaves a loop that
			// broke early looking complete.
			out.EligibleDeals++
			for _, rule := range Rules() {
				found := rule.Ask(now, subject, s.cfg)
				if found == nil {
					continue
				}
				findings = append(findings, *found)
				if err := s.store.UpsertException(ctx, tx, *found, subject.Owner); err != nil {
					return err
				}
			}
		}
		out.Findings = len(findings)
		out.Readiness = Readiness(coverage, findings, s.cfg)
		out.Status = StatusComplete
		if out.Readiness == ReadinessChecksIncomplete {
			out.Status = StatusIncomplete
		}
		return s.store.FinishRun(ctx, tx, runID, out.EligibleDeals, 0,
			out.Status, out.Readiness)
	})
	if err != nil {
		return Result{}, err
	}
	return out, nil
}
