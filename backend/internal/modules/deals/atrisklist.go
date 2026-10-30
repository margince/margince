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

// AtRiskWindow is the at-risk candidate set in one predicate: open deals quiet
// for QuietForDays, or open deals whose expected close is before CloseBefore.
// The Worklist's two sweeps read its arms one at a time.
type AtRiskWindow struct {
	QuietForDays int
	CloseBefore  time.Time
}

// AtRiskSort orders the at-risk set by expected close, earliest first, with an
// undated deal last.
const AtRiskSort = closeDateField

// atRiskTerms adds the at-risk predicate and the position sink to a list read.
func (in ListDealsInput) atRiskTerms(pre *storekit.ListPrelude, where []string) []string {
	if in.positions != nil {
		pre.CollectPositions(in.positions)
	}
	if in.AtRisk == nil {
		return where
	}
	closes := fmt.Sprintf("(status = 'open' AND expected_close_date < $%d)",
		pre.Arg(in.AtRisk.CloseBefore.Format(time.DateOnly)))
	return append(where, "("+QuietSQL("", in.AtRisk.QuietForDays)+" OR "+closes+")")
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
