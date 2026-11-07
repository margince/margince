// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package forecasting

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Assembly is one forecast result: the readings with their per-deal rows, the
// scope they resolved to, and the basis the chances stood on.
type Assembly struct {
	Readings Readings
	// Scope is the population RESOLVED, which an unset request leaves to the
	// caller's lens; the answer always names it.
	Scope Scope
	// Limited says deals the caller cannot read were left out. A boolean: a
	// count of what a caller may not read says how much of it there is.
	Limited bool
	// SelfCorrecting is whether the chances moved with recorded outcomes, and
	// EvidenceCutoff the instant no later outcome counted.
	SelfCorrecting bool
	EvidenceCutoff time.Time
	// RiskSignalsVersion names the signal rules on the rows; empty while
	// self-correction is off and no signal was assessed.
	RiskSignalsVersion string
}

// AssembleFunc builds the forecast result for a period, row-scoped. Injected
// rather than imported: the deals, stages and outcomes live in other modules,
// and the caller's authority sits in the composition layer.
type AssembleFunc func(ctx context.Context, tx pgx.Tx, period Period, scope Scope, asOf time.Time, baseCurrency string) (Assembly, error)

// ReaderRowsFunc narrows the per-deal rows of a reading to what the reader may
// see one by one. Injected, because the installation's individual-performance
// rule lives in the composition layer.
type ReaderRowsFunc func(ctx context.Context, tx pgx.Tx, rows []Contribution) ([]Contribution, bool, error)

// WithReaderRows narrows the rows GET /forecast?contributions=true returns.
// Unbound, the door refuses to return rows at all.
func (h Handlers) WithReaderRows(narrow ReaderRowsFunc) Handlers {
	h.readerRows = narrow
	return h
}

// contributionRows renders the per-deal rows this reader may see onto out, and
// says when they were narrowed. Unbound, the door refuses rather than return
// rows nobody narrowed.
func (h Handlers) contributionRows(ctx context.Context, tx pgx.Tx, readings Readings, out *crmcontracts.ForecastReadings) error {
	if h.readerRows == nil {
		return errors.New("forecasting: no reader lens is bound, so per-deal rows cannot be narrowed")
	}
	visible, limited, err := h.readerRows(ctx, tx, readings.Contributions)
	if err != nil {
		return err
	}
	rows := ContributionsToWire(visible)
	out.Contributions, out.ContributionsLimited = &rows, &limited
	return nil
}
