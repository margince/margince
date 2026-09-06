// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

// The week's forecast, frozen beside the review that reports it.
//
// The review already freezes its deals: a retrospective is a record of what a
// week WAS, and a line that changes because somebody cleaned up a deal is a
// record nobody can trust. A landing is the same kind of fact.
//
// Snapshot ids alone would not survive. forecast_snapshot is subject to
// retention, so a review holding only pointers reads as a blank outlook the day
// they age out — and blank is indistinguishable from a week nobody measured. So
// the ids say where the figures came from while they exist, and the copied
// figures are what the panel draws forever.

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Outlook is one horizon's landing, frozen.
type Outlook struct {
	PeriodKind string
	// The window's own local days. Carried as time.Time because the column is a
	// Postgres date: reading it as text and parsing it back would add a failure
	// mode the database cannot produce.
	PeriodStart time.Time
	PeriodEnd   time.Time
	// Opening is the landing at the start of the week, from that Monday's
	// snapshot. Absent when no Monday snapshot exists — which is NOT zero: a
	// zero opening draws a week that started from nothing and made everything.
	Opening OutlookSide
	// Closing is the landing at the end, and the figures the panel reports.
	Closing OutlookSide
	// ForwardMeasure is the measure the closing landing was built under,
	// frozen: the installation setting can change, and a past week must keep
	// saying what it was read under.
	ForwardMeasure string
	BaseCurrency   string
	// The readings behind the landing, copied for the same reason.
	WonMinor      int64
	CommitMinor   int64
	BestCaseMinor int64
	WeightedMinor int64
	// Movement is how this window got from its opening landing to its closing
	// one. Empty where there was no opening snapshot to move from.
	Movement []Movement
}

// OutlookSide is one end of the week: which snapshot, and what it said.
type OutlookSide struct {
	// Known is false when the side has no snapshot. Every other field is then
	// meaningless, and the writer stores nulls rather than zeros.
	Known        bool
	SnapshotID   ids.UUID
	LandingMinor int64
}

// Movement is one bar of the bridge, signed.
type Movement struct {
	PeriodKind string
	Bar        string
	DeltaMinor int64
	// Drivers are the deals worth naming under this bar, largest absolute
	// movement first. Empty is honest: a bar can move on deals the reader may
	// not see, and naming one would disclose a record they cannot open.
	Drivers []Driver
}

// Driver is one deal behind a bar, frozen with the label it carried.
type Driver struct {
	PeriodKind string
	Bar        string
	DealID     ids.UUID
	DealLabel  string
	DeltaMinor int64
}

// driversPerBar caps how many deals a bar remembers.
//
// Three, because the panel asks "which ones?" and a reader scanning a
// retrospective wants the ones that moved it, not a ledger. A cap rather than
// everything also bounds what one bad week can write.
const driversPerBar = 3

// DriversPerBar is that cap, for the seam that picks which deals to send.
//
// Exported so the picker and the table agree on one number: a seam choosing its
// own would write rows the panel does not expect, or fewer than a bar can show.
func DriversPerBar() int { return driversPerBar }

// writeOutlook freezes one review's horizons, bars and drivers.
//
// Idempotent on the review: a retried job finds the rows already there and
// writes nothing, because a second insert would double every bar in the bridge.
func writeOutlook(
	ctx context.Context, tx pgx.Tx, reviewID ids.UUID,
	outlooks []Outlook, movements []Movement, drivers []Driver,
) error {
	for _, outlook := range outlooks {
		if err := insertOutlook(ctx, tx, reviewID, outlook); err != nil {
			return err
		}
	}
	for _, movement := range movements {
		if err := insertMovement(ctx, tx, reviewID, movement); err != nil {
			return err
		}
	}
	for _, driver := range drivers {
		if err := insertDriver(ctx, tx, reviewID, driver); err != nil {
			return err
		}
	}
	// ONE audit row for the whole outlook rather than one per bar. The subject
	// is the review, and a bridge writing twenty audit rows would bury the
	// entry that says the review gained its forecast.
	//
	// AuditEvent and not Audit: freezing an outlook has NO prior state — these
	// rows did not exist a moment ago — and the ordinary door refuses an update
	// carrying no before-image, correctly. This is an occurrence, not an edit.
	//
	// "create" rather than a verb of its own. The action vocabulary is a
	// migration-controlled CHECK, and these rows are in fact being created;
	// extending a constrained list for one call site would be a schema change
	// bought for a word.
	if _, err := storekit.AuditEvent(ctx, tx, "create", "weekly_review", reviewID,
		map[string]any{
			"outlook_periods": len(outlooks),
			"movement_bars":   len(movements),
		}); err != nil {
		return err
	}
	return nil
}

func insertOutlook(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, out Outlook) error {
	cols, args := insertColumns{}, []any(nil)
	add := func(name string, value any) {
		cols = append(cols, name)
		args = append(args, value)
	}
	add("weekly_review_id", reviewID)
	add("period_kind", out.PeriodKind)
	add("period_start", out.PeriodStart)
	add("period_end", out.PeriodEnd)
	add("base_currency", out.BaseCurrency)
	add("won_minor", out.WonMinor)
	add("commit_minor", out.CommitMinor)
	add("best_case_minor", out.BestCaseMinor)
	add("weighted_minor", out.WeightedMinor)
	// The opening side travels whole or not at all, which is what the table's
	// own CHECK holds. An absent Monday writes nulls, and the panel says "no
	// Monday snapshot" rather than drawing a zero it would read as a fact.
	if out.Opening.Known {
		add("opening_snapshot_id", out.Opening.SnapshotID)
		add("opening_landing_minor", out.Opening.LandingMinor)
	}
	if out.Closing.Known {
		add("closing_snapshot_id", out.Closing.SnapshotID)
		add("closing_landing_minor", out.Closing.LandingMinor)
		add("forward_measure", out.ForwardMeasure)
	}

	_, err := tx.Exec(ctx, fmt.Sprintf(`
		INSERT INTO weekly_review_outlook (%s) VALUES (%s)
		ON CONFLICT (weekly_review_id, period_kind) DO NOTHING`,
		cols.names(), cols.placeholders()), args...)
	if err != nil {
		return fmt.Errorf("weekly: freezing the %s outlook: %w", out.PeriodKind, err)
	}
	return nil
}

func insertMovement(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, move Movement) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO weekly_review_movement
		    (weekly_review_id, period_kind, bar, delta_minor)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (weekly_review_id, period_kind, bar) DO NOTHING`,
		reviewID, move.PeriodKind, move.Bar, move.DeltaMinor)
	if err != nil {
		return fmt.Errorf("weekly: freezing the %s bar: %w", move.Bar, err)
	}
	return nil
}

func insertDriver(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, driver Driver) error {
	_, err := tx.Exec(ctx, `
		INSERT INTO weekly_review_driver
		    (weekly_review_id, period_kind, bar, deal_id, deal_label, delta_minor)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (weekly_review_id, period_kind, bar, deal_id) DO NOTHING`,
		reviewID, driver.PeriodKind, driver.Bar, driver.DealID,
		driver.DealLabel, driver.DeltaMinor)
	if err != nil {
		return fmt.Errorf("weekly: freezing a %s driver: %w", driver.Bar, err)
	}
	return nil
}

// Bars is the movement attached to one horizon, and Drivers the deals under a
// bar. Held on Outlook rather than read separately by the handler, so a caller
// that has an outlook has the whole story rather than three queries' worth of
// it.
//
// readMovement below fills them in one pass over each table, because a query
// per bar would be one round trip per row of a panel.

// readOutlook reads back one review's frozen horizons.
//
// The figures come from THIS table and never from forecast_snapshot, even where
// the snapshot still exists. Reading through the id would make a retrospective
// answer differently before and after retention ran.
func readOutlook(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) ([]Outlook, error) {
	rows, err := tx.Query(ctx, `
		SELECT period_kind, period_start, period_end, base_currency,
		       won_minor, commit_minor, best_case_minor, weighted_minor,
		       opening_snapshot_id, opening_landing_minor,
		       closing_snapshot_id, closing_landing_minor, forward_measure
		  FROM weekly_review_outlook
		 WHERE weekly_review_id = $1
		 ORDER BY period_kind`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the frozen outlook: %w", err)
	}
	defer rows.Close()

	out, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Outlook, error) {
		var o Outlook
		var openID, closeID *ids.UUID
		var openLanding, closeLanding *int64
		var measure *string
		err := row.Scan(&o.PeriodKind, &o.PeriodStart, &o.PeriodEnd, &o.BaseCurrency,
			&o.WonMinor, &o.CommitMinor, &o.BestCaseMinor, &o.WeightedMinor,
			&openID, &openLanding, &closeID, &closeLanding, &measure)
		if openID != nil && openLanding != nil {
			o.Opening = OutlookSide{Known: true, SnapshotID: *openID, LandingMinor: *openLanding}
		}
		if closeID != nil && closeLanding != nil {
			o.Closing = OutlookSide{Known: true, SnapshotID: *closeID, LandingMinor: *closeLanding}
		}
		if measure != nil {
			o.ForwardMeasure = *measure
		}
		return o, err
	})
	if err != nil {
		return nil, fmt.Errorf("weekly: collecting the frozen outlook: %w", err)
	}
	if err := readMovement(ctx, tx, reviewID, out); err != nil {
		return nil, err
	}
	return out, nil
}

// readMovement attaches the bars and their drivers to the horizons they belong
// to, in one pass over each table.
func readMovement(ctx context.Context, tx pgx.Tx, reviewID ids.UUID, horizons []Outlook) error {
	if len(horizons) == 0 {
		return nil
	}
	drivers, err := readDrivers(ctx, tx, reviewID)
	if err != nil {
		return err
	}

	rows, err := tx.Query(ctx, `
		SELECT period_kind, bar, delta_minor
		  FROM weekly_review_movement
		 WHERE weekly_review_id = $1`, reviewID)
	if err != nil {
		return fmt.Errorf("weekly: reading the frozen bridge: %w", err)
	}
	defer rows.Close()

	bars := map[string][]Movement{}
	for rows.Next() {
		var m Movement
		if err := rows.Scan(&m.PeriodKind, &m.Bar, &m.DeltaMinor); err != nil {
			return fmt.Errorf("weekly: scanning a frozen bar: %w", err)
		}
		m.Drivers = drivers[m.PeriodKind+"/"+m.Bar]
		bars[m.PeriodKind] = append(bars[m.PeriodKind], m)
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("weekly: reading the frozen bridge: %w", err)
	}

	for i := range horizons {
		// Ordered by the bridge's own drawing order rather than by whatever the
		// scan returned: a waterfall read left to right is a story, and two
		// readers of one week must get the same one.
		horizons[i].Movement = inBarOrder(bars[horizons[i].PeriodKind])
	}
	return nil
}

// readDrivers reads every driver on a review, keyed by horizon and bar.
func readDrivers(ctx context.Context, tx pgx.Tx, reviewID ids.UUID) (map[string][]Driver, error) {
	rows, err := tx.Query(ctx, `
		SELECT period_kind, bar, deal_id, deal_label, delta_minor
		  FROM weekly_review_driver
		 WHERE weekly_review_id = $1
		 ORDER BY abs(delta_minor) DESC`, reviewID)
	if err != nil {
		return nil, fmt.Errorf("weekly: reading the frozen drivers: %w", err)
	}
	defer rows.Close()

	out := map[string][]Driver{}
	for rows.Next() {
		var d Driver
		if err := rows.Scan(&d.PeriodKind, &d.Bar, &d.DealID, &d.DealLabel, &d.DeltaMinor); err != nil {
			return nil, fmt.Errorf("weekly: scanning a frozen driver: %w", err)
		}
		out[d.PeriodKind+"/"+d.Bar] = append(out[d.PeriodKind+"/"+d.Bar], d)
	}
	return out, rows.Err()
}

// inBarOrder puts a horizon's bars in the bridge's drawing order.
func inBarOrder(bars []Movement) []Movement {
	if len(bars) == 0 {
		return nil
	}
	byBar := map[string]Movement{}
	for _, bar := range bars {
		byBar[bar.Bar] = bar
	}
	out := make([]Movement, 0, len(bars))
	for _, name := range BridgeBars() {
		if bar, moved := byBar[name]; moved {
			out = append(out, bar)
		}
	}
	return out
}
