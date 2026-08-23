// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/gradionhq/margince/backend/internal/platform/database"
	"github.com/gradionhq/margince/backend/internal/shared/ports/fieldcatalog"
)

// liveRowsClause narrows a statement to rows that have not been archived.
const liveRowsClause = " AND archived_at IS NULL"

// Store owns this module's tables (data-seam ownership, ADR-0014 Am.1); every
// write rides the storekit audit+outbox shape in one transaction.
type Store struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
	// catalog is the fieldcatalog seam (custom-field columns); nil means no
	// catalog is wired and every read/write runs core-columns-only.
	catalog fieldcatalog.Reader
	// clock is the "today" a phase transition is stamped against; injected so
	// duration folding is deterministic in tests.
	clock func() time.Time
}

// NewStore binds the store to the pool every tenant query runs through.
func NewStore(db *database.DB) *Store {
	return &Store{db: db, clock: time.Now}
}

// WithFieldCatalog wires the custom-field seam. Without it the store runs
// core-columns-only, which is the honest answer for an installation that has
// declared no custom fields.
func (s *Store) WithFieldCatalog(catalog fieldcatalog.Reader) *Store {
	s.catalog = catalog
	return s
}

// WithClock replaces the "today" source, so a test can state the date a phase
// transition is measured against instead of racing the wall clock.
func (s *Store) WithClock(clock func() time.Time) *Store {
	s.clock = clock
	return s
}

// Tx opens the transaction every read and write in this module runs inside,
// bound to the workspace the store holds. Exported because storekit's list
// helper takes the opener rather than a database handle of its own.
func (s *Store) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.db.Tx(ctx, fn)
}

// catalogColumns answers which custom-field columns a project carries, with no
// gate of its own: it is the raw catalog read, called by the store-opened entry
// points that have ALREADY taken their grant, and by ActiveProjectColumns, which
// is the gated spelling a caller outside this package gets.
func (s *Store) catalogColumns(ctx context.Context) ([]fieldcatalog.Column, error) {
	if s.catalog == nil {
		return nil, nil
	}
	return s.catalog.ActiveColumns(ctx, projectObject)
}

// CustomColumns is the catalog's answer, carried from a caller that had to
// fetch it before it opened its transaction to the seam that runs inside that
// transaction.
//
// The columns are unexported deliberately. They become quoted identifiers in a
// SELECT list and in an UPDATE's SET clause (storekit's customcolumns helpers),
// so a caller able to name its own could widen a read to any column of the same
// table, or write a core column past the typed input this store validates. Only
// this package can populate one, so that is unrepresentable rather than
// forbidden by comment. The zero value is the honest empty answer: core columns
// only.
type CustomColumns struct {
	cols []fieldcatalog.Column
}
