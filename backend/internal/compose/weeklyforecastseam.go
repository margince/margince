// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Where a closed week was landing, for the review that freezes it.
//
// The binding between two modules that may not import each other: forecasting
// owns the snapshots and the arithmetic, compose/weekly owns the review they
// are frozen into. Everything here is assembly — no reading is computed a
// second time, because a bridge that re-derived its own figures would be a
// second answer to a question the engine already answers.

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/weekly"
	"github.com/margince/margince/backend/internal/modules/forecasting"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// weeklyHorizons are the windows a weekly review reports its landing over.
//
// Three, because a rep asks three different questions on a Monday: what this
// week has to do, what the month still needs, and whether the quarter is safe.
// One horizon would answer whichever of them the reader was not asking.
var weeklyHorizons = []forecasting.PeriodKind{
	forecasting.PeriodWeek, forecasting.PeriodMonth, forecasting.PeriodQuarter,
}

// WeeklyForecast freezes a closed week's landing into its review.
type WeeklyForecast struct {
	store *forecasting.Store
}

// NewWeeklyForecast binds the seam to the forecasting store.
func NewWeeklyForecast(store *forecasting.Store) *WeeklyForecast {
	return &WeeklyForecast{store: store}
}

// CloseWeek takes the closing snapshot for each horizon and reports the bars
// between the week's opening snapshot and it.
//
// The snapshot is taken HERE rather than read, because the week closing is the
// event that should freeze it: a review assembled from whatever daily snapshot
// happened to exist would report a landing measured at an arbitrary hour.
func (w *WeeklyForecast) CloseWeek(
	ctx context.Context, tx pgx.Tx, weekStart, weekEnd time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	// A weekly review is a PERSONAL record, so its landing is the rep's own
	// book. Asking for the workspace would be refused outright for any rep
	// without an installation-wide lens — and the refusal rolls back the
	// surrounding transaction, costing them the whole retrospective rather
	// than just its outlook.
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, nil, nil, apperrors.ErrPermissionDenied
	}
	owner := ids.UUID(actor.UserID)
	return w.closeHorizons(ctx, tx,
		forecasting.Scope{Kind: forecasting.ScopeOwner, ID: &owner}, weekStart, weekEnd)
}

// CloseTeamWeek is CloseWeek over the TEAM's book.
//
// A separate entry point and not a flag, because the scope is the whole
// difference and it is the part that can be refused: forecasting's scope
// authority admits ScopeTeam only to a caller who is IN the team
// (scopeauthority.go), so the lead the weekly job binds passes and anybody else
// gets ErrNotFound.
//
// The team's landing is NOT the sum of its members'. A deal owned by nobody on
// the team is in neither, and one the team works but a member owns is in both —
// so summing six personal outlooks would answer a question nobody asked.
func (w *WeeklyForecast) CloseTeamWeek(
	ctx context.Context, tx pgx.Tx, teamID ids.UUID, weekStart, weekEnd time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	if teamID.IsZero() {
		return nil, nil, nil, apperrors.ErrNotFound
	}
	return w.closeHorizons(ctx, tx,
		forecasting.Scope{Kind: forecasting.ScopeTeam, ID: &teamID}, weekStart, weekEnd)
}

// closeHorizons freezes every window this review reports, over one book.
//
// Shared by the rep's and the team's entry points above so the three horizons,
// their order and their bar folding are decided once: two copies would let a
// team's outlook quietly report two windows where a rep's reports three.
func (w *WeeklyForecast) closeHorizons(
	ctx context.Context, tx pgx.Tx, scope forecasting.Scope, weekStart, weekEnd time.Time,
) ([]weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	var outlooks []weekly.Outlook
	var movements []weekly.Movement
	var drivers []weekly.Driver

	for _, kind := range weeklyHorizons {
		outlook, bars, deals, err := w.closeHorizon(ctx, tx, kind, scope, weekStart, weekEnd)
		if err != nil {
			return nil, nil, nil, err
		}
		outlooks = append(outlooks, outlook)
		movements = append(movements, bars...)
		drivers = append(drivers, deals...)
	}
	return outlooks, movements, drivers, nil
}

// closeHorizon freezes one window.
func (w *WeeklyForecast) closeHorizon(
	ctx context.Context, tx pgx.Tx, kind forecasting.PeriodKind,
	scope forecasting.Scope, weekStart, weekEnd time.Time,
) (weekly.Outlook, []weekly.Movement, []weekly.Driver, error) {
	// The week's LAST INSTANT, not weekEnd. weekEnd is the exclusive bound —
	// the following Monday's midnight — so resolving a period at it lands in
	// the NEXT week, and for a month or quarter whose window ends on that
	// boundary, in the next one of those too. The whole outlook would then
	// freeze figures for a window the review is not about.
	at := weekEnd.Add(-time.Nanosecond)
	period, baseCurrency, err := ForecastPeriodAt(ctx, tx, kind, at)
	if err != nil {
		return weekly.Outlook{}, nil, nil, err
	}

	closing, readings, measure, err := w.freezeClosing(ctx, tx, period, scope, at, baseCurrency)
	if err != nil {
		return weekly.Outlook{}, nil, nil, err
	}

	outlook := weekly.Outlook{
		PeriodKind:     string(kind),
		PeriodStart:    period.StartDate,
		PeriodEnd:      period.EndDate,
		Closing:        weekly.OutlookSide{Known: true, SnapshotID: closing, LandingMinor: readings.landing},
		ForwardMeasure: string(measure),
		BaseCurrency:   baseCurrency,
		WonMinor:       readings.won,
		CommitMinor:    readings.commit,
		BestCaseMinor:  readings.bestCase,
		WeightedMinor:  readings.weighted,
	}

	// The opening side is the snapshot this week STARTED from. Absent is a real
	// answer — an installation in its first week has no Monday snapshot — and
	// absent is not zero: a zero opening draws a week that started from nothing
	// and made everything.
	opening, found, err := w.openingSnapshot(ctx, tx, period, scope, weekStart)
	if err != nil {
		return weekly.Outlook{}, nil, nil, err
	}
	if !found {
		return outlook, nil, nil, nil
	}

	// The bridge explains the reading the LANDING was built from, not a fixed
	// one. A commit-evidence installation whose bars explained the weighted
	// pipeline would show a rep movements that do not add up to the number
	// above them.
	movement, err := w.store.MovementTx(ctx, tx, movementReadingFor(measure), opening, closing)
	if err != nil {
		return weekly.Outlook{}, nil, nil, err
	}
	outlook.Opening = weekly.OutlookSide{
		Known: true, SnapshotID: opening, LandingMinor: movement.OpeningMinor,
	}

	bars, deals, err := foldMovement(ctx, tx, string(kind), movement)
	if err != nil {
		return weekly.Outlook{}, nil, nil, err
	}
	return outlook, bars, deals, nil
}

// movementReadingFor says which of the four money answers the bridge explains.
//
// The landing's forward half IS one of them, so the bars explain that one and
// the waterfall reconciles to the figure printed above it. A manager call has
// no reading behind it — it is an authored total — so its bridge explains the
// commit evidence the fallback landing would have used, which is the closest
// honest account of what the pipeline did.
func movementReadingFor(measure forecasting.ForwardMeasure) forecasting.Reading {
	if measure == forecasting.MeasureWeighted {
		return forecasting.ReadingWeighted
	}
	return forecasting.ReadingEvidence
}

// topDrivers picks the deals worth naming under each bar, with the labels they
// carried this week.
//
// Capped per bar: the panel asks "which ones?" and a reader wants the ones that
// moved it, not a ledger. The cap also bounds what one bad week can write.
func topDrivers(
	ctx context.Context, tx pgx.Tx, kind string, deltas []forecasting.DealDelta,
) ([]weekly.Driver, error) {
	byBar := map[string][]forecasting.DealDelta{}
	for _, delta := range deltas {
		bar, err := weekly.BarFor(delta.Bucket, delta.AmountMinor)
		if err != nil {
			return nil, err
		}
		byBar[bar] = append(byBar[bar], delta)
	}

	var wanted []forecasting.DealDelta
	for _, bar := range weekly.BridgeBars() {
		inBar := byBar[bar]
		// Largest ABSOLUTE movement first: a slip of 90k matters as much as a
		// win of 90k, and sorting by signed size would bury every loss.
		// Tie broken on the deal id, because SortFunc is not stable: two equal
		// movements at the three-driver cutoff would otherwise swap between
		// runs and drop a different one each time, which makes a frozen record
		// disagree with itself.
		slices.SortFunc(inBar, func(a, b forecasting.DealDelta) int {
			if order := biggerMovementFirst(a.AmountMinor, b.AmountMinor); order != 0 {
				return order
			}
			return strings.Compare(a.DealID, b.DealID)
		})
		wanted = append(wanted, inBar[:min(len(inBar), weekly.DriversPerBar())]...)
	}
	if len(wanted) == 0 {
		return nil, nil
	}

	labels, err := dealLabels(ctx, tx, wanted)
	if err != nil {
		return nil, err
	}
	out := make([]weekly.Driver, 0, len(wanted))
	for _, delta := range wanted {
		id, err := ids.Parse(delta.DealID)
		if err != nil {
			return nil, fmt.Errorf("compose: a movement names deal %q: %w", delta.DealID, err)
		}
		label, named := labels[id]
		if !named {
			// A deal the caller cannot read, or one deleted between the
			// snapshot and now. Skipped rather than written with a placeholder:
			// a frozen driver whose label says "unknown" is a row nobody can
			// act on, and inventing one would leak that the deal exists.
			continue
		}
		bar, err := weekly.BarFor(delta.Bucket, delta.AmountMinor)
		if err != nil {
			return nil, err
		}
		out = append(out, weekly.Driver{
			PeriodKind: kind, Bar: bar, DealID: id,
			DealLabel: label, DeltaMinor: delta.AmountMinor,
		})
	}
	return out, nil
}

// dealLabels reads the names of the deals a bar is opened to, row-scoped.
//
// Scoped because a driver is a record reference: a frozen retrospective must
// not name a deal its reader was never allowed to see.
func dealLabels(
	ctx context.Context, tx pgx.Tx, deltas []forecasting.DealDelta,
) (map[ids.UUID]string, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }

	wanted := make([]ids.UUID, 0, len(deltas))
	for _, delta := range deltas {
		id, err := ids.Parse(delta.DealID)
		if err != nil {
			return nil, fmt.Errorf("compose: a movement names deal %q: %w", delta.DealID, err)
		}
		wanted = append(wanted, id)
	}

	scopeClause, err := auth.ScopeClauseFor(ctx, tableDeal, "d", arg)
	if err != nil {
		return nil, err
	}
	if scopeClause == "" {
		scopeClause = sqlUnnarrowed
	}
	sql := fmt.Sprintf(
		`SELECT d.id, d.name FROM deal d WHERE d.id = ANY($%d) AND %s`,
		arg(wanted), scopeClause)

	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("compose: reading the movement's deal labels: %w", err)
	}
	defer rows.Close()

	out := map[ids.UUID]string{}
	for rows.Next() {
		var id ids.UUID
		var name string
		if err := rows.Scan(&id, &name); err != nil {
			return nil, fmt.Errorf("compose: scanning a movement's deal label: %w", err)
		}
		out[id] = name
	}
	return out, rows.Err()
}

// biggerMovementFirst orders two movements by magnitude, largest first.
//
// COMPARED rather than subtracted. Two int64 magnitudes can overflow when
// subtracted, and the result then has to be truncated to int for SortFunc —
// which on a 32-bit int would flip the sign of a large difference and quietly
// invert the order. Comparing sidesteps both.
func biggerMovementFirst(a, b int64) int {
	left, right := magnitude(a), magnitude(b)
	switch {
	case left > right:
		return -1
	case left < right:
		return 1
	default:
		return 0
	}
}

// magnitude is how far a movement moved, whichever way.
//
// math.MinInt64 has no positive counterpart, so negating it returns itself — a
// NEGATIVE magnitude that would sort as the smallest movement when it is the
// largest. Clamped instead: no real money reaches it, and a sort that inverted
// there would be a defect nobody could see.
func magnitude(v int64) int64 {
	if v == math.MinInt64 {
		return math.MaxInt64
	}
	if v < 0 {
		return -v
	}
	return v
}

// closingReadings are the figures a frozen outlook copies.
type closingReadings struct {
	landing, won, commit, bestCase, weighted int64
}

// freezeClosing takes the period-close snapshot and reports what it said.
func (w *WeeklyForecast) freezeClosing(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	at time.Time, baseCurrency string,
) (ids.UUID, closingReadings, forecasting.ForwardMeasure, error) {
	deals, _, _, err := ForecastDeals(ctx, tx, period, scope, at, baseCurrency)
	if err != nil {
		return ids.Nil, closingReadings{}, "", err
	}
	readings, err := forecasting.Compute(period, period.LocalDay(at), deals)
	if err != nil {
		return ids.Nil, closingReadings{}, "", err
	}
	measure, err := ForecastForwardMeasure(ctx, tx)
	if err != nil {
		return ids.Nil, closingReadings{}, "", err
	}
	// No manager call is consulted for a frozen weekly landing. A call is an
	// assertion about a period a contact is still working; a retrospective
	// reports what the PIPELINE said, and substituting somebody's number would
	// freeze an opinion as a measurement.
	landing, err := forecasting.ProjectLanding(readings, measure, nil)
	if err != nil {
		return ids.Nil, closingReadings{}, "", err
	}

	id, err := w.store.TakeSnapshot(ctx, tx, forecasting.NewSnapshot{
		Period: period, Scope: scope, Trigger: forecasting.TriggerPeriodClose,
		Readings: readings, BaseCurrency: baseCurrency, TakenAt: at,
	})
	if err != nil {
		return ids.Nil, closingReadings{}, "", err
	}
	// landing.Measure and NOT the requested one. A manager-call installation
	// with no call falls back to commit evidence, and freezing the request
	// would label a commit-evidence figure as somebody's authored call — the
	// wrong number under a right word, in a record nobody can correct later.
	return id, closingReadings{
		landing:  landing.AmountMinor,
		won:      readings.WonMinor,
		commit:   readings.EvidenceMinor,
		bestCase: readings.BestCaseMinor,
		weighted: readings.WeightedMinor,
	}, landing.Measure, nil
}

// openingSnapshot finds the snapshot this week started from.
func (w *WeeklyForecast) openingSnapshot(
	ctx context.Context, tx pgx.Tx, period forecasting.Period, scope forecasting.Scope,
	weekStart time.Time,
) (ids.UUID, bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM forecast_snapshot
		 WHERE period_start = $1 AND period_end = $2
		   AND scope_kind = $3 AND scope_id IS NOT DISTINCT FROM $4
		   AND local_day = $4
		 ORDER BY taken_at DESC
		 LIMIT 1`,
		period.StartDate, period.EndDate, string(scope.Kind), scope.ID, weekStart).Scan(&id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			// No snapshot on or before this week's Monday. A real state — an
			// installation in its first week, or one whose worker was down —
			// and the caller reports an absent opening rather than a zero.
			return ids.Nil, false, nil
		}
		return ids.Nil, false, fmt.Errorf("compose: finding the week's opening snapshot: %w", err)
	}
	return id, true, nil
}

// barsFrom sums the per-deal deltas into the bridge's bars.
//
// Pure, and separate from the driver read beside it: the fold is the part that
// decides what a waterfall says, and it is worth being able to test without a
// database or an authenticated caller.
func barsFrom(kind string, movement forecasting.Movement) ([]weekly.Movement, error) {
	byBar := map[string]int64{}
	for _, delta := range movement.Deals {
		bar, err := weekly.BarFor(delta.Bucket, delta.AmountMinor)
		if err != nil {
			return nil, err
		}
		byBar[bar] += delta.AmountMinor
	}
	var bars []weekly.Movement
	// Walked in the bridge's own order rather than over the map, so two reviews
	// of the same week write their bars in one order.
	for _, bar := range weekly.BridgeBars() {
		delta, moved := byBar[bar]
		if !moved {
			continue
		}
		bars = append(bars, weekly.Movement{PeriodKind: kind, Bar: bar, DeltaMinor: delta})
	}
	return bars, nil
}

// foldMovement turns the engine's twelve buckets into the bridge's six bars,
// and the deals behind them into the drivers a bar can be opened to.
func foldMovement(
	ctx context.Context, tx pgx.Tx, kind string, movement forecasting.Movement,
) ([]weekly.Movement, []weekly.Driver, error) {
	// Folded from the PER-DEAL deltas, never from movement.Buckets.
	//
	// A bucket total is already netted: the engine sums every deal into one
	// figure per cause, so a deal repriced up 100 and another down 50 arrive as
	// a single +50. Three of the twelve buckets pick their bar by SIGN, and
	// folding a netted total would put that whole 50 under "advanced" — the
	// slipped movement, and every driver behind it, silently gone.
	//
	// Per deal, the two land in different bars and both survive. The identity
	// still holds: the same deltas are summed, only grouped differently.
	bars, err := barsFrom(kind, movement)
	if err != nil {
		return nil, nil, err
	}
	drivers, err := topDrivers(ctx, tx, kind, movement.Deals)
	if err != nil {
		return nil, nil, err
	}
	return bars, drivers, nil
}
