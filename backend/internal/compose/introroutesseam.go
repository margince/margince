// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// One assembler for the person-graph read surface.
//
// The Network tab ranks routes and has no idea which favours have already been
// asked; the introductions module holds that and refuses a second open ask on
// one route. Joining them is compose's job, and this is the whole of it — the
// store already answers what network's AskedRoutes asks for, so there is no
// adapter here, only the wiring.
//
// Exported so a test drives the SAME assembly the process serves. Built by
// hand in a test, the introductions reader is the piece most easily left out —
// and left out it stamps nothing, so every route reads `available` exactly as
// it did before this seam existed and the test passes over the gap.
// Held by: TestPersonGraphMarksARouteThatAlreadyHasAnOpenAsk
// (backend/internal/compose/integration/persongraph_integration_test.go)

import (
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/compose/network"
	"github.com/margince/margince/backend/internal/modules/introductions"
	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
)

// NewPersonGraphReads assembles the network read surface the way the server
// does, including the introductions reader that stamps route availability.
func NewPersonGraphReads(pool *pgxpool.Pool, db *database.DB) network.Reads {
	return network.NewReads(pool, people.NewStore(db)).
		WithAskedRoutes(introductions.NewStore(db, time.Now))
}
