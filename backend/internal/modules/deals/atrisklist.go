// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

import (
	"context"
	"fmt"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// AtRiskWindow is the at-risk rule that every at-risk read builds on. A deal
// is at risk when it is open and quiet for QuietForDays at AsOf. It is also at
// risk when it is open and its expected close is before CloseBefore, the
// workspace day of AsOf. Arm picks one half, as the two bounded sweeps do.
type AtRiskWindow struct {
	QuietForDays int
	AsOf         time.Time
	CloseBefore  time.Time
	Arm          AtRiskArm
}

// AtRiskArm names which half of the at-risk rule a read applies.
type AtRiskArm int

const (
	// AtRiskEither is the whole rule.
	AtRiskEither AtRiskArm = iota
	// AtRiskQuiet is the quiet half.
	AtRiskQuiet
	// AtRiskOverdue is the past-its-close half.
	AtRiskOverdue
)

// AtRiskSort orders the at-risk set by expected close, earliest first, with an
// undated deal last.
const AtRiskSort = closeDateField

// atRiskTerms adds the at-risk predicate and the position sink to a list read.
// Both halves are judged at the window's own instant, never the database's.
func (in ListDealsInput) atRiskTerms(pre *storekit.ListPrelude, where []string) []string {
	if in.positions != nil {
		pre.CollectPositions(in.positions)
	}
	window := in.AtRisk
	if window == nil {
		return where
	}
	quiet := func() string {
		return quietAtSQL("", window.QuietForDays, fmt.Sprintf("$%d::timestamptz", pre.Arg(window.AsOf)))
	}
	closes := func() string {
		return fmt.Sprintf("(status = 'open' AND expected_close_date < $%d)", pre.Arg(window.CloseBefore.Format(time.DateOnly)))
	}
	switch window.Arm {
	case AtRiskQuiet:
		return append(where, quiet())
	case AtRiskOverdue:
		return append(where, closes())
	}
	return append(where, "("+quiet()+" OR "+closes()+")")
}

// ListDealsPositioned is ListDeals that also answers, for each row, the cursor
// that resumes the list after it.
func (s *Store) ListDealsPositioned(
	ctx context.Context, in ListDealsInput,
) ([]crmcontracts.Deal, []string, storekit.Page, error) {
	positions := []string{}
	in.positions = &positions
	rows, page, err := s.ListDeals(ctx, in)
	return rows, positions, page, err
}
