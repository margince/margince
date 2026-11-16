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
	"fmt"
	"sort"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

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

// Contribution is one finding a pass raised and the seat it was raised for.
type Contribution struct {
	Finding Finding
	Owner   string
}

// Contributions are the seats behind a pass's figures: the owner of each deal
// it checked, and each finding it raised.
type Contributions struct {
	DealOwners []string
	Raised     []Contribution
}

// PreviewFloor holds a preview's figures to the small-group floor of "Show
// individual performance", inside the preview's own transaction.
type PreviewFloor func(ctx context.Context, tx pgx.Tx, seats Contributions) error

// Preview asks every rule the real pass asks and records none of the answers.
//
// It shares askEveryRule with Scan, so it cannot promise a finding the run it
// authorises then does not raise. What it does not share is the writing half,
// and the transaction it runs in is rolled back regardless — a rule that later
// learns to write cannot quietly start writing here.
func (s *Scanner) Preview(ctx context.Context, now time.Time, floor PreviewFloor) (Preview, error) {
	var out Preview
	started, err := s.store.EverRun(ctx)
	if err != nil {
		return Preview{}, err
	}
	out.Started = started

	err = s.store.InPreviewTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		out.Coverage = s.coverage(ctx, tx, now)
		subjects, err := s.subjects(ctx, tx, now, out.Coverage)
		if err != nil {
			return unreadablePreview(ctx, tx, &out, floor)
		}
		pass := askEveryRule(now, subjects, s.cfg, out.Coverage)
		if _, err := s.askSignals(ctx, tx, now, &pass); err != nil {
			return unreadablePreview(ctx, tx, &out, floor)
		}
		if err := s.askCalls(ctx, tx, now, &pass); err != nil {
			return unreadablePreview(ctx, tx, &out, floor)
		}
		out.EligibleDeals = pass.eligible
		out.Counts = countByType(pass.findings())
		out.Readiness = Readiness(out.Coverage, pass.findings(), s.cfg)
		return floor(ctx, tx, pass.contributions())
	})
	if err != nil {
		return Preview{}, err
	}
	return out, nil
}

// unreadablePreview is the answer Scan gives when the deals could not be read:
// we could not look. It counts nothing, because zero findings would tell a
// reader the pipeline is clean when nothing was read, and it names no seat.
func unreadablePreview(ctx context.Context, tx pgx.Tx, out *Preview, floor PreviewFloor) error {
	out.EligibleDeals, out.Counts, out.Readiness = 0, nil, ReadinessChecksIncomplete
	return floor(ctx, tx, Contributions{})
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

// InPreviewTx runs fn against this workspace and always rolls back what fn
// wrote.
//
// fn runs in a savepoint rolled back however fn ends, so a rule that learns to
// write cannot write in preview. The floor fn reads after its walk sees the
// walk's snapshot. A detached context never unwinds a caller's transaction.
//
// Gated on create: the only reason to look is to decide whether to start a pass.
func (s *Store) InPreviewTx(ctx context.Context, fn func(context.Context, pgx.Tx) error) error {
	if err := auth.Require(ctx, "forecast", principal.ActionCreate); err != nil {
		return err
	}
	return s.db.TxIsolated(database.Detached(ctx), pgx.RepeatableRead, func(tx pgx.Tx) error {
		walk, err := tx.Begin(ctx)
		if err != nil {
			return fmt.Errorf("assurance: opening the preview's savepoint: %w", err)
		}
		err = fn(ctx, walk)
		if unwound := walk.Rollback(ctx); unwound != nil {
			return fmt.Errorf("assurance: rolling the preview back: %w", unwound)
		}
		return err
	})
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
		var err error
		ever, err = s.EverRunTx(ctx, tx)
		return err
	})
	return ever, err
}

// EverRunTx is EverRun in the caller's transaction. A pass over one deal is
// not an enrolment, so only a pass over every deal counts.
func (s *Store) EverRunTx(ctx context.Context, tx pgx.Tx) (bool, error) {
	if err := auth.Require(ctx, "forecast", principal.ActionRead); err != nil {
		return false, err
	}
	var ever bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM assurance_run WHERE subject_deal_id IS NULL)`).Scan(&ever); err != nil {
		return false, fmt.Errorf("assurance: reading whether this workspace has been checked: %w", err)
	}
	return ever, nil
}
