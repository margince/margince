// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package assurance

// A pass over one deal, queued because that deal changed.
//
// It asks the nightly pass's rules through the same store, but closes only
// within its deal. Absence is a fact about what a pass walked, and this pass
// walked one deal. Its run names the deal, so nobody reads it as the pipeline.
//
// It commits only if the deal did not change while it read. The marker holds
// the generation of the deal's last change, and the pass removes it only at
// the generation it read. Otherwise it rolls back and asks to run again.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// DealSubjectsFunc reads one deal as a rule sees it: one subject, or none when
// the deal is no longer open.
type DealSubjectsFunc func(ctx context.Context, tx pgx.Tx, asOf time.Time, coverage []SourceCoverage, deal ids.UUID) ([]Subject, error)

// ErrInputsMovedDuringPass says the deal changed while a one-deal pass read
// it. Nothing was committed; the pass should run again.
var ErrInputsMovedDuringPass = errors.New("assurance: the deal changed while it was being checked")

// WithDealSubjects lets the scanner check one deal at a time.
func (s *Scanner) WithDealSubjects(subjects DealSubjectsFunc) *Scanner {
	wired := *s
	wired.dealSubjects = subjects
	return &wired
}

// ScanDeal checks one changed deal. A deal with no unread change is not
// checked again: a duplicate delivery finds nothing to do and records nothing.
func (s *Scanner) ScanDeal(ctx context.Context, now time.Time, deal ids.UUID) (Result, error) {
	if s.dealSubjects == nil {
		return Result{}, errors.New("assurance: no one-deal reader is wired, so a deal cannot be checked alone")
	}
	var out Result
	err := s.store.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		// The nightly pass's lock. A pass over the pipeline and a pass over one
		// of its deals must not write the same findings at once.
		if err := storekit.LockWriteIdentity(ctx, tx, "assurance", "pass"); err != nil {
			return err
		}
		generation, err := s.store.dealGeneration(ctx, tx, deal)
		if err != nil || generation == 0 {
			return err
		}
		return s.scanOneDeal(ctx, tx, now, deal, generation, &out)
	})
	if err != nil {
		return Result{}, err
	}
	return out, nil
}

func (s *Scanner) scanOneDeal(
	ctx context.Context, tx pgx.Tx, now time.Time, deal ids.UUID, generation int64, out *Result,
) error {
	runID, err := s.store.openRun(ctx, tx, now, nil, &deal)
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
	subjects, err := s.dealSubjects(ctx, tx, now, coverage, deal)
	if err != nil {
		out.Status, out.Readiness = StatusIncomplete, ReadinessChecksIncomplete
		return s.store.FinishRun(ctx, tx, runID, 0, 0, 0, out.Status, out.Readiness)
	}
	pass := askEveryRule(now, subjects, s.cfg, coverage)
	out.EligibleDeals, out.Findings = pass.eligible, len(pass.raised)
	for _, r := range pass.raised {
		if err := s.store.UpsertException(ctx, tx, r.finding, r.owner); err != nil {
			return err
		}
	}
	if err := s.store.RecordReceipts(ctx, tx, runID, now, s.cfg.ReceiptLifetime, subjects); err != nil {
		return err
	}
	if err := s.store.RecordRunFindings(ctx, tx, runID, pass.seen); err != nil {
		return err
	}
	if out.Cleared, err = s.store.CloseCleared(ctx, tx, clearableTypes(coverage), pass.deals, pass.seen); err != nil {
		return err
	}
	if out.Departed, err = s.store.CloseDeparted(ctx, tx, &deal, pass.deals); err != nil {
		return err
	}
	out.Readiness = Readiness(coverage, pass.findings(), s.cfg)
	out.Status = StatusComplete
	if out.Readiness == ReadinessChecksIncomplete {
		out.Status = StatusIncomplete
	} else if err := s.store.clearDealMarker(ctx, tx, deal, generation); err != nil {
		return err
	}
	return s.store.FinishRun(ctx, tx, runID, out.EligibleDeals, 0, out.Cleared, out.Status, out.Readiness)
}

// DealChangePending answers whether the deal has a change no pass has read.
func (s *Store) DealChangePending(ctx context.Context, deal ids.UUID) (bool, error) {
	var generation int64
	err := s.InTx(ctx, func(ctx context.Context, tx pgx.Tx) error {
		var err error
		generation, err = s.dealGeneration(ctx, tx, deal)
		return err
	})
	return generation > 0, err
}

// dealGeneration is the generation of the deal's unread change, zero when it
// has none.
func (s *Store) dealGeneration(ctx context.Context, tx pgx.Tx, deal ids.UUID) (int64, error) {
	var generation int64
	err := tx.QueryRow(ctx, `
		SELECT generation FROM assurance_input_change
		 WHERE subject_kind = $1 AND subject_id = $2`, ScopeDeal, deal).Scan(&generation)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("assurance: reading the deal's change: %w", err)
	}
	return generation, nil
}

// clearDealMarker removes the deal's marker while it still holds the
// generation the pass read. Removing nothing means it changed since.
func (s *Store) clearDealMarker(ctx context.Context, tx pgx.Tx, deal ids.UUID, generation int64) error {
	if err := auth.Require(ctx, "forecast", principal.ActionUpdate); err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM assurance_input_change
		 WHERE subject_kind = $1 AND subject_id = $2 AND generation = $3`, ScopeDeal, deal, generation)
	if err != nil {
		return fmt.Errorf("assurance: recording that the deal's change was read: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return ErrInputsMovedDuringPass
	}
	return nil
}
