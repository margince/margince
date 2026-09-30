// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
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
	// Cleared counts formerly open findings this pass closed because their
	// condition is no longer present.
	Cleared   int64
	Readiness string
	Status    string
}

// walked is what one pass of the rules came to, before anything is written.
//
// ONE walk serves the nightly pass and the preview a human reads before
// starting the cycle. Two copies would let the preview promise findings the run
// it authorises then does not raise, which is the one way a preview can be
// worse than no preview at all.
type walked struct {
	// eligible is counted in the LOOP, one per deal actually evaluated. Taken
	// from len(subjects) it would be the same number the query returned, which
	// makes the census assert x == x and leaves a loop that broke early looking
	// complete.
	eligible int
	raised   []raised
	// seen are the logical keys of this walk's findings, and deals are the ids
	// it visited. Together they are what absence stands on: a finding whose key
	// is missing from a walk that visited its deal has no condition left.
	seen  []string
	deals []string
}

// raised is one finding and the seat it belongs to. The owner comes from the
// subject rather than the rule, so it travels with the finding instead of being
// looked up a second time at the write.
type raised struct {
	finding Finding
	owner   string
}

// findings is the walk's findings alone, for readiness and for counting.
func (w walked) findings() []Finding {
	out := make([]Finding, 0, len(w.raised))
	for _, r := range w.raised {
		out = append(out, r.finding)
	}
	return out
}

// askEveryRule asks every rule of every subject, once, and writes nothing.
func askEveryRule(now time.Time, subjects []Subject, cfg Config) walked {
	var out walked
	for _, subject := range subjects {
		out.deals = append(out.deals, subject.DealID)
		out.eligible++
		for _, rule := range Rules() {
			found := rule.Ask(now, subject, cfg)
			if found == nil {
				continue
			}
			out.raised = append(out.raised, raised{finding: *found, owner: subject.Owner})
			out.seen = append(out.seen, LogicalKey(*found))
		}
	}
	return out
}

// Scan asks every rule of every live open deal, once, and records what it found.
//
// It NEVER refuses to start. An upstream that could not be read makes the run
// incomplete and its readiness `checks_incomplete` — it does not make the run
// absent. Refusing would produce no record in exactly the case this pass exists
// to report, and the brief waiting on it would run without ever learning why.
// requestedBy names the seat that asked, and nil is the nightly cadence.
func (s *Scanner) Scan(ctx context.Context, now time.Time, requestedBy *string) (Result, error) {
	var out Result
	err := s.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// ONE pass at a time, and the lock is the whole reason a human may ask
		// for one. Two overlapping passes walk two snapshots, and CloseCleared
		// closes any open finding its own walk did not re-mint — so the older
		// snapshot can close a finding the newer one just raised, and the deal
		// it belongs to loses the task that was about to be minted for it.
		// Nothing arbitrated this while the nightly sweep was the only caller.
		//
		// The key carries no workspace, following LockWriteIdentity's own rule:
		// one installation serves one company, so a workspace would distinguish
		// nothing. A pass that has to wait is correct — it runs next, over the
		// records the first one left.
		if err := storekit.LockWriteIdentity(ctx, tx, "assurance", "pass"); err != nil {
			return err
		}
		runID, err := s.store.StartRun(ctx, tx, now, requestedBy)
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
			return s.store.FinishRun(ctx, tx, runID, 0, 0, 0, out.Status, out.Readiness)
		}

		pass := askEveryRule(now, subjects, s.cfg)
		out.EligibleDeals = pass.eligible
		out.Findings = len(pass.raised)
		for _, r := range pass.raised {
			if err := s.store.UpsertException(ctx, tx, r.finding, r.owner); err != nil {
				return err
			}
		}
		// Which findings THIS run saw, before the clearing below removes the
		// ones it did not. Recorded from the same `seen` set that decides what
		// stays open, so the membership and the clearing can never disagree
		// about what tonight observed.
		if err := s.store.RecordRunFindings(ctx, tx, runID, pass.seen); err != nil {
			return err
		}
		// A finding this complete walk did not re-mint has no condition left to
		// report — close it, but only for rules whose required sources were
		// read tonight. Absence is a claim, and it stands on what was looked at.
		cleared, err := s.store.CloseCleared(ctx, tx, clearableTypes(coverage), pass.deals, pass.seen)
		if err != nil {
			return err
		}
		out.Cleared = cleared
		out.Readiness = Readiness(coverage, pass.findings(), s.cfg)
		out.Status = StatusComplete
		if out.Readiness == ReadinessChecksIncomplete {
			out.Status = StatusIncomplete
		}
		return s.store.FinishRun(ctx, tx, runID, out.EligibleDeals, 0, out.Cleared,
			out.Status, out.Readiness)
	})
	if err != nil {
		return Result{}, err
	}
	return out, nil
}

// clearableTypes names the rule types whose findings tonight's pass may close
// in absence: every source the rule needs was actually read. A rule with no
// declared needs stands on the subjects read alone, which succeeding is what
// got us here.
func clearableTypes(coverage []SourceCoverage) []string {
	checked := map[string]bool{}
	for _, c := range coverage {
		checked[c.Source] = c.State == CoverageChecked
	}
	var out []string
	for _, rule := range Rules() {
		clearable := true
		for _, need := range rule.Needs {
			if !checked[need] {
				clearable = false
			}
		}
		if clearable {
			out = append(out, rule.Type)
		}
	}
	return out
}
