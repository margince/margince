// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package finance

// The store: reads of the mirror, and nothing else.
//
// There is no create, update or delete on a finance RECORD anywhere in this
// file, and that is the read-only posture (FIN-DDL-N-1) expressed where a
// contributor would look for the write rather than as a runtime refusal. Rows
// arrive through the sync pass, which is the connector's own path.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
)

// Store reads the finance mirror under the caller's own gates.
type Store struct {
	// db binds the workspace this store runs for (ADR-0091 §9 step 3).
	db *database.DB
	// now is injected so a summary's staleness and its trailing window are
	// testable without waiting for the clock to move.
	now func() time.Time
	// rates resolves the installation's reporting currency, the rate in force
	// on a day, and the conversion. The constructor requires
	// it, because the mirror freezes a rate onto rows it will not revisit.
	rates Rates
	// billingContacts answers who handles this customer's invoices, bound by
	// compose because the relationship table belongs to contacts. Nil in an
	// installation that has not wired it, which serves the card without the
	// panel rather than failing.
	billingContacts BillingContactReader
	// sources resolves the providers Connect can open; nil offers none.
	sources Sources
}

// BaseCurrencyFunc resolves the installation's reporting currency inside a
// transaction the caller already holds. Compose supplies the one real
// implementation; a function rather than a settings handle keeps finance free
// of a registry it has no other use for.
type BaseCurrencyFunc func(context.Context, pgx.Tx) (string, error)

// NewStore binds the store to the pool every tenant read runs through.
// NewStore opens this module's store on a handle already bound to the
// workspace it serves.
func NewStore(db *database.DB, rates Rates) *Store {
	return &Store{db: db, now: time.Now, rates: rates}
}

// SetClock replaces the store's clock, for tests. Whether a summary reads
// "stale" depends on the clock, and a test must not wait for it.
func (s *Store) SetClock(now func() time.Time) *Store {
	s.now = now
	return s
}

func (s *Store) tx(ctx context.Context, fn func(pgx.Tx) error) error {
	return s.db.Tx(ctx, fn)
}

// staleAfter is how long a successful sync stays current.
//
// The sweep runs every six hours, so a mirror that has not synced in a full
// day has missed four passes — long enough that the reader should see the date
// beside the figure rather than take it as today's.
const staleAfter = 24 * time.Hour
