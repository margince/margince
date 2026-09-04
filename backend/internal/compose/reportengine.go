// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The report engine's own shape: what it holds and how a surface builds one.
// The vocabulary it executes (specs, selects, aggregates) lives in report.go,
// and the drill-through in derivation.go — this file is only the seam set,
// which differs per surface and is the thing a caller has to get right.

import (
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/attention"
)

type reportEngine struct {
	pool *pgxpool.Pool
	// names labels the drill-through's source rows for a human reader
	// (derivationlabels.go). Absent on the agent surface, which answers in
	// ids, and absent means unlabelled rather than an error.
	names attention.Names
}

func newReportEngine(pool *pgxpool.Pool) *reportEngine {
	return &reportEngine{pool: pool}
}

// WithNames wires the display-name seam the drill-through reads through, so
// "explain this number" answers with the records a reader recognises. The
// seam carries the READER's grants, so this adds a presentation lane and no
// authority: a name the caller may not read stays absent.
func (e *reportEngine) WithNames(names attention.Names) *reportEngine {
	e.names = names
	return e
}
