// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// What a pass WOULD find, for somebody deciding whether to start one.
//
// The first pass over a workspace with a backlog raises every exception it can
// see at once, and a queue that arrives that way is one nobody reads. So the
// first pass is a decision somebody makes, and this is what they read to make
// it.
//
// A preview is NOT a run and deliberately not a status a run can be in. It
// writes nothing — no run row, no exception, no task — which is what makes it
// safe to look at and also what keeps it out of every surface that reads the
// latest run. A reading nobody should act on must not be available to act on.
//
// COUNTS ONLY, and no record ever names itself here. The scan reads the whole
// pipeline on purpose (a duplicate rule seeing one rep's deals would report no
// duplicates), so a preview that carried subjects or money would hand a seat
// aggregates over deals it cannot open — the same reason listInputChecks
// publishes no count of what it withheld. Eligible deals and the readiness
// verdict are already whole-pipeline figures on GET /forecast/assurance, so
// what is added here is the per-type shape and nothing sharper.

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// errSubjectsUnreadable is the deals not being readable, carried out of the
// preview's transaction as an OUTCOME rather than a failure.
//
// The same answer Scan gives: we could not look. It travels as an error because
// that is how a transaction is told to stop, and it is unwrapped at the top
// because a preview reporting zero findings here would tell a reader the
// pipeline is clean when nothing was read — and they would start the cycle on
// it. The cause is wrapped with it so a log still says what actually broke.
var errSubjectsUnreadable = errors.New("assurance: the deals could not be read")

// Preview is one pass's findings as counts, plus what it would conclude.
type Preview struct {
	EligibleDeals int
	Counts        []TypeCount
	Coverage      []SourceCoverage
	Readiness     string
	// Started is whether any pass has ever run here. It is what tells a first
	// check from a recheck, and therefore which of the two acts the reader is
	// about to authorise.
	Started bool
}

// TypeCount is one exception type at one severity, and how many of it a pass
// would raise.
type TypeCount struct {
	Type     string
	Severity string
	Count    int
}

// Preview asks every rule the real pass asks and records none of the answers.
//
// It shares askEveryRule with Scan, so it cannot promise a finding the run it
// authorises then does not raise. What it does not share is the writing half,
// and the transaction it runs in is rolled back regardless — a rule that later
// learns to write cannot quietly start writing here.
func (s *Scanner) Preview(ctx context.Context, now time.Time) (Preview, error) {
	var out Preview
	started, err := s.store.EverRun(ctx)
	if err != nil {
		return Preview{}, err
	}
	out.Started = started

	err = s.store.InPreviewTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out.Coverage = s.coverage(ctx, tx, now)
		subjects, err := s.subjects(ctx, tx)
		if err != nil {
			return fmt.Errorf("%w: %w", errSubjectsUnreadable, err)
		}
		pass := askEveryRule(now, subjects, s.cfg)
		out.EligibleDeals = pass.eligible
		out.Counts = countByType(pass.findings())
		out.Readiness = Readiness(out.Coverage, pass.findings(), s.cfg)
		return nil
	})
	if errors.Is(err, errSubjectsUnreadable) {
		out.EligibleDeals, out.Counts = 0, nil
		out.Readiness = ReadinessChecksIncomplete
		return out, nil
	}
	if err != nil {
		return Preview{}, err
	}
	return out, nil
}

// countByType tallies findings into the shape a reader decides on, ordered so
// two previews of the same pipeline read the same way.
func countByType(findings []Finding) []TypeCount {
	tally := map[TypeCount]int{}
	for _, f := range findings {
		tally[TypeCount{Type: f.Type, Severity: f.Severity}]++
	}
	out := make([]TypeCount, 0, len(tally))
	for key, count := range tally {
		key.Count = count
		out = append(out, key)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Type != out[j].Type {
			return out[i].Type < out[j].Type
		}
		return out[i].Severity < out[j].Severity
	})
	return out
}

// errPreviewUnwind rolls a preview's transaction back. It is not a failure —
// it is how this driver's helper is told not to commit, and the preview's whole
// guarantee is that nothing it touched survives it.
var errPreviewUnwind = errors.New("assurance: preview complete")

// InPreviewTx runs fn against this workspace and ALWAYS rolls back.
//
// The guarantee is structural rather than a promise in a comment: a rule that
// later learns to write cannot start writing in preview without the rollback
// taking it back. Detached first, because a preview joined to an ambient read
// snapshot would unwind the caller's transaction along with its own.
//
// Gated on create, not read. The only reason to open this door is to decide
// whether to start a pass, and starting one is a create.
func (s *Store) InPreviewTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	err := database.WithWorkspaceTx(database.Detached(ctx), s.db.Pool(), func(tx pgx.Tx) error {
		if err := fn(ctx, tx); err != nil {
			return err
		}
		return errPreviewUnwind
	})
	if errors.Is(err, errPreviewUnwind) {
		return nil
	}
	return err
}

// EverRun answers whether any pass has ever run in this workspace.
//
// This is the enrolment, and it is DERIVED rather than stored: a workspace is
// in the nightly cadence because somebody started it, and the run history is
// already that record. A flag beside it would be a second answer to one
// question — and it would need a backfill for every installation already being
// checked, where the runs say so for free. A preview writes no run, so looking
// cannot enrol anybody.
func (s *Store) EverRun(ctx context.Context) (bool, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return false, err
	}
	var ever bool
	err := database.WithWorkspaceTx(ctx, s.db.Pool(), func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assurance_run)`).Scan(&ever)
	})
	if err != nil {
		return false, fmt.Errorf("assurance: reading whether this workspace has been checked: %w", err)
	}
	return ever, nil
}
