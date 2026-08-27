// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The forecast's own history: what a deal's amount and close date were, when
// they moved, and who moved them. It lives beside the stage history rather than
// inside it — see recordForecastMovement and the 0215 migration for why the two
// tables stay apart.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// forecastColumns are the fields a forecast reconstruction reads. A move in any
// of them is what deal_forecast_history exists to record.
var forecastColumns = []string{amountField, currencyField, closeDateField}

// recordForecastMovement snapshots the forecast fields when an update moves one
// of them without moving the stage.
//
// deal_stage_history answers "what did this deal look like when it entered this
// stage", and it is written on creation and on a move — nowhere else. So an
// amount revised in place and a close date slipped leave no trace at all, and
// the second of those is the single most common reason a real forecast moves. A
// reconstruction built from stage history alone reconciles over stage movement
// and omits the rest while presenting itself as the whole answer: a partial sum
// wearing the label of a total, which is what finance/formulas.go already
// refuses by hand elsewhere.
//
// It writes to its own table rather than adding an arm to deal_stage_history
// because five readers hold that table to its stated meaning — health.go reads
// max(changed_at) as "when did this deal last move", the automation previews
// COUNT rows as movements — and a row that is not a transition would make every
// edited deal look freshly moved. See the migration for the full argument.
//
// The values are the deal's state AFTER the change, which is what a
// reconstruction asking "what was the forecast as of date T" needs from the
// latest row at or before T. They are read back off the ROW, inside the write's
// own transaction, rather than folded together from a before-image and the
// caller's input: the writers that reach this hold the deal in different shapes
// — a wire record, a candidate row, a set of scanned columns — and each of them
// assembling the resulting state itself is each of them able to assemble it
// wrongly. The row already knows.
//
// The patch decides WHETHER to write, by which columns it assigns: a write that
// assigned no forecast column records nothing, or the table would answer "the
// forecast moved" for every edit anyone makes to a deal. A close date cleared or
// set for the first time is an assignment like any other, and counts as movement
// exactly as a revision does.
//
// Assigned, not changed — storekit.Patch records an assignment without comparing
// it to what the row held, so "the patch decided" means the writer decided. A
// writer that would otherwise re-assign a value the row already carries compares
// first, so that a pass which moved nothing records nothing.
//
// Call it only after the patch has landed, because it reads the row it is
// recording. applyDealPatchGuarded and applyDealPatchLocked are the two seams
// that guarantee that order, and every write this module makes to the deal row
// goes through one of them.
func recordForecastMovement(ctx context.Context, tx pgx.Tx, id ids.DealID, moved map[string]any) error {
	if !movesForecast(moved) {
		return nil
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx,
		`INSERT INTO deal_forecast_history
		   (deal_id, changed_by, amount_minor_at_change, currency_at_change, close_date_at_change)
		 SELECT d.id, $2, d.amount_minor, d.currency, d.expected_close_date
		   FROM deal d WHERE d.id = $1`,
		id, by)
	if err != nil {
		return fmt.Errorf("record forecast history: %w", err)
	}
	if tag.RowsAffected() != 1 {
		// The caller just wrote this row in this transaction, so a SELECT that
		// resolves nothing is a programming error. Insert-from-select reports it
		// as zero rows and no error, which is the shape that would leave the
		// history quietly short — the failure this whole file exists to prevent.
		return fmt.Errorf("record forecast history: deal %s did not resolve after its own write", id)
	}
	return nil
}

// movesForecast answers whether a patch's changed columns include one a forecast
// reconstruction reads.
func movesForecast(moved map[string]any) bool {
	for _, column := range forecastColumns {
		if _, ok := moved[column]; ok {
			return true
		}
	}
	return false
}
