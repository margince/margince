// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The Account coverage section: the accounts at risk in one population. Three
// views over the same scope the other analytics sections take, each judged by
// a threshold the reporting framework holds.
//
// In compose because a verdict joins three owners. The deal's committee is
// deals.EngagedSeatCounts, the account's last exchange is company360, and the
// population a reader may measure is the analytics scope.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/company360"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// The read's bounds: this many candidates are judged per view, and this
// many rows are answered.
const (
	coverageCandidates = 500
	coverageRows       = 200
)

type accountCoverage struct {
	db  *database.DB
	now func() time.Time
}

// Read answers the counts of every view and the rows of the view asked for.
// A reader without the activity grant gets the single-threaded view alone,
// and the answer says so rather than counting the other two as zero.
func (s accountCoverage) Read(ctx context.Context, params crmcontracts.GetAccountCoverageParams) (crmcontracts.AccountCoverage, error) {
	if err := auth.RequireHuman(ctx); err != nil {
		return crmcontracts.AccountCoverage{}, err
	}
	for _, object := range []string{tableDeal, "company"} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return crmcontracts.AccountCoverage{}, err
		}
	}
	view := crmcontracts.AccountCoverageViewSingleThreaded
	if params.View != nil {
		view = crmcontracts.AccountCoverageView(*params.View)
	}
	now := s.now().UTC()
	out := crmcontracts.AccountCoverage{AsOf: now, View: view, Rows: []crmcontracts.AccountCoverageRow{}, ActivityReadable: auth.ReadGranted(ctx, "activity")}
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		var args []any
		arg := func(v any) int { args = append(args, v); return len(args) }
		resolved, population, err := AnalyticsPopulationClause(ctx, tx, requestedScopeOf(params), "d", arg, unownedIsPartOfDefault)
		if err != nil {
			return err
		}
		out.Scope = scopeToWire(resolved)
		out.Thresholds, out.FrameworkRevision, err = reporting.AccountCoverageThresholds(ctx, tx)
		if err != nil {
			return err
		}
		pop := &coveragePopulation{clause: population, args: args, now: now, thresholds: out.Thresholds}
		return s.judge(ctx, tx, pop, &out)
	})
	return out, err
}

func requestedScopeOf(params crmcontracts.GetAccountCoverageParams) RequestedScope {
	var requested RequestedScope
	if params.ScopeKind != nil {
		requested.Kind = string(*params.ScopeKind)
	}
	if params.ScopeId != nil {
		id := ids.UUID(*params.ScopeId)
		requested.ID = &id
	}
	return requested
}

// coveragePopulation is the deal population one answer is about. It holds
// the scope clause on the deal's owner with its arguments, the instant, and
// the thresholds the verdicts are judged at.
type coveragePopulation struct {
	clause     string
	args       []any
	now        time.Time
	thresholds crmcontracts.AccountCoverageThresholds
	// truncated is set by a candidate read that hit its cap. The answer then
	// says its counts cover the largest candidates and not every one.
	truncated bool
}

// judge runs every view for its count and keeps the rows of the one asked
// for. An answer costs three statements and one touch read. That buys counts
// a reader can trust on every segment.
func (s accountCoverage) judge(ctx context.Context, tx pgx.Tx, pop *coveragePopulation, out *crmcontracts.AccountCoverage) error {
	// Every verdict reads activity: who wrote, and whether anybody did. A
	// reader without that grant gets no verdict rather than a count of zero.
	if !out.ActivityReadable {
		return nil
	}
	single, withheld, err := singleThreadedDeals(ctx, tx, pop)
	if err != nil {
		return err
	}
	out.Counts.SingleThreaded = len(single)
	out.Withheld = &withheld
	if out.View == crmcontracts.AccountCoverageViewSingleThreaded {
		out.Rows = capRows(single)
	}
	untouched, err := untouchedAccounts(ctx, tx, pop)
	if err != nil {
		return err
	}
	silent, err := silentAccounts(ctx, tx, pop)
	if err != nil {
		return err
	}
	untouchedCount, silentCount := len(untouched), len(silent)
	out.Counts.Untouched, out.Counts.Silent = &untouchedCount, &silentCount
	out.Truncated = &pop.truncated
	switch out.View {
	case crmcontracts.AccountCoverageViewUntouched:
		out.Rows = capRows(untouched)
	case crmcontracts.AccountCoverageViewSilent:
		out.Rows = capRows(silent)
	}
	return nil
}

func capRows(rows []crmcontracts.AccountCoverageRow) []crmcontracts.AccountCoverageRow {
	if len(rows) > coverageRows {
		return rows[:coverageRows]
	}
	return rows
}

// singleThreadedDeals are the open deals of the population with at most the
// threshold's engaged seats. A deal whose committee the reader cannot read
// whole is counted as withheld and judged by nobody.
func singleThreadedDeals(ctx context.Context, tx pgx.Tx, pop *coveragePopulation) ([]crmcontracts.AccountCoverageRow, int, error) {
	candidates, err := openDealsOf(ctx, tx, pop)
	if err != nil {
		return nil, 0, err
	}
	dealIDs := make([]ids.UUID, 0, len(candidates))
	for _, candidate := range candidates {
		dealIDs = append(dealIDs, ids.UUID(candidate.Deal.Id))
	}
	seats, err := deals.EngagedSeatCounts(ctx, tx, dealIDs, pop.now)
	// A reader refused the seats cannot judge any committee: every candidate
	// is withheld, and none is called single-threaded.
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, len(candidates), nil
	}
	if err != nil {
		return nil, 0, err
	}
	var rows []crmcontracts.AccountCoverageRow
	withheld := 0
	for _, candidate := range candidates {
		seat := seats[ids.UUID(candidate.Deal.Id)]
		if seat.Withheld {
			withheld++
			continue
		}
		if seat.Engaged > *pop.thresholds.MaxEngagedSeats {
			continue
		}
		engaged, named := seat.Engaged, seat.Named
		candidate.EngagedSeats, candidate.NamedSeats = &engaged, &named
		rows = append(rows, candidate)
	}
	return rows, withheld, nil
}

// untouchedAccounts are the accounts with an open deal in the population and
// no exchange either way within the threshold.
func untouchedAccounts(ctx context.Context, tx pgx.Tx, pop *coveragePopulation) ([]crmcontracts.AccountCoverageRow, error) {
	candidates, err := accountsInPipeline(ctx, tx, pop)
	if err != nil {
		return nil, err
	}
	return silentSince(ctx, tx, candidates, pop, *pop.thresholds.UntouchedDays)
}

// silentAccounts are the accounts with a deal won within the window, no open
// deal, and no exchange either way within the threshold.
func silentAccounts(ctx context.Context, tx pgx.Tx, pop *coveragePopulation) ([]crmcontracts.AccountCoverageRow, error) {
	candidates, err := wonAccounts(ctx, tx, pop)
	if err != nil {
		return nil, err
	}
	return silentSince(ctx, tx, candidates, pop, *pop.thresholds.SilentDays)
}

// silentSince keeps the candidates whose newest exchange is older than the
// threshold, or who never had one, and fills the moments the drawer shows.
// The exchange is the company page's own last-touch read, so an account
// answered by a colleague is not silent here either.
func silentSince(ctx context.Context, tx pgx.Tx, candidates []crmcontracts.AccountCoverageRow, pop *coveragePopulation, days int) ([]crmcontracts.AccountCoverageRow, error) {
	if len(candidates) == 0 {
		return nil, nil
	}
	companyIDs := make([]ids.CompanyID, 0, len(candidates))
	for _, candidate := range candidates {
		companyIDs = append(companyIDs, ids.From[ids.CompanyKind](ids.UUID(candidate.Company.Id)))
	}
	touches, err := company360.LastTouchFor(ctx, tx, companyIDs, pop.now, company360.AssembleOptions{})
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read when the accounts last exchanged a word: %w", err)
	}
	cutoff := pop.now.AddDate(0, 0, -days)
	var rows []crmcontracts.AccountCoverageRow
	for _, candidate := range candidates {
		touch := touches[ids.From[ids.CompanyKind](ids.UUID(candidate.Company.Id))]
		if touch.Contact != nil && !touch.Contact.At.Before(cutoff) {
			continue
		}
		candidate.LastInbound, candidate.LastOutbound = touch.InboundAt, touch.OutboundAt
		if since, ok := silentDays(touch, candidate, pop.now); ok {
			candidate.DaysSilent = &since
		}
		rows = append(rows, candidate)
	}
	return rows, nil
}

// silentDays counts from the newest exchange, or from the won deal's close
// when there was none. An account with neither has no count to print.
func silentDays(touch company360.LastTouch, row crmcontracts.AccountCoverageRow, now time.Time) (int, bool) {
	switch {
	case touch.Contact != nil:
		return int(now.Sub(touch.Contact.At).Hours() / 24), true
	case row.WonDeal != nil && row.WonDeal.ClosedAt != nil:
		return int(now.Sub(*row.WonDeal.ClosedAt).Hours() / 24), true
	}
	return 0, false
}
