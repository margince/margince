// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Whether a composed unit is handing the core records it cannot express.
//
// A unit moves its cursor past a record the grammar refuses — stopping on one
// malformed message parks the whole connection — so the drop leaves the unit's
// own logs and nothing else. Until this read existed, a provider format change
// that made EVERY record unrepresentable presented to the installation exactly
// like a healthy quiet feed.
//
// COUNTS AND A CLASS. The core's sentence about a refused record quotes the
// record back, so it is never stored and never served; the class is the core's
// own closed vocabulary and is what names the mapping to fix.

import (
	"context"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// extensionIngestWindowDays is how far back the counts reach. A week spans a
// full weekly cycle of a provider's traffic, so a connector that only carries
// weekday volume still reports as itself; a longer window would keep naming a
// mapping somebody already fixed.
const extensionIngestWindowDays = 7

type extensionIngestHealthHandlers struct {
	pool *pgxpool.Pool
	now  func() time.Time
}

// GetExtensionIngestHealth reports what the core has refused, per unit.
func (h extensionIngestHealthHandlers) GetExtensionIngestHealth(w http.ResponseWriter, r *http.Request) {
	if !admitHealthReader(w, r) {
		return
	}
	now := h.now()
	serveHealthReport(w, r, h.pool, "extension ingest health",
		func(ctx context.Context, tx pgx.Tx) (crmcontracts.ExtensionIngestHealth, error) {
			return readExtensionIngestHealth(ctx, tx, now)
		})
}

// readExtensionIngestHealth folds the daily rows into one row per unit.
//
// ONE statement, ordered so the fold needs no lookahead: unit, then the unit's
// classes largest first. A unit with nothing refused in the window has no rows
// and so is absent — the page answers "what is wrong", and a roll of healthy
// units is what a reader would have to scan past to find it.
func readExtensionIngestHealth(ctx context.Context, tx pgx.Tx, now time.Time) (crmcontracts.ExtensionIngestHealth, error) {
	out := crmcontracts.ExtensionIngestHealth{
		GeneratedAt: now,
		WindowDays:  extensionIngestWindowDays,
		Units:       []crmcontracts.ExtensionUnitIngestHealth{},
	}
	since := now.AddDate(0, 0, -(extensionIngestWindowDays - 1))
	rows, err := tx.Query(ctx, `
		SELECT unit, refusal, sum(refused)::bigint AS refused, max(last_at) AS last_at
		  FROM extension_ingest_refusal
		 WHERE day >= $1::date
		 GROUP BY unit, refusal
		 ORDER BY unit, refused DESC, refusal`, since)
	if err != nil {
		return crmcontracts.ExtensionIngestHealth{}, err
	}
	defer rows.Close()

	for rows.Next() {
		var unit, refusal string
		var refused int64
		var lastAt time.Time
		if err := rows.Scan(&unit, &refusal, &refused, &lastAt); err != nil {
			return crmcontracts.ExtensionIngestHealth{}, err
		}
		out.Units = appendRefusal(out.Units, unit, refusal, refused, lastAt)
	}
	if err := rows.Err(); err != nil {
		return crmcontracts.ExtensionIngestHealth{}, err
	}
	return out, nil
}

// appendRefusal folds one (unit, class) row onto the answer.
//
// The statement orders by unit, so a row either extends the last entry or opens
// a new one. The unit's own total and newest refusal are accumulated here
// rather than asked of the database a second time: they are sums of the rows
// already in hand, and a second statement could disagree with the first about
// a row written between them.
func appendRefusal(units []crmcontracts.ExtensionUnitIngestHealth, unit, refusal string,
	refused int64, lastAt time.Time,
) []crmcontracts.ExtensionUnitIngestHealth {
	if len(units) == 0 || units[len(units)-1].Unit != unit {
		units = append(units, crmcontracts.ExtensionUnitIngestHealth{
			Unit:     unit,
			Refusals: []crmcontracts.ExtensionIngestRefusal{},
		})
	}
	entry := &units[len(units)-1]
	entry.Refused += int(refused)
	if entry.LastRefusedAt == nil || entry.LastRefusedAt.Before(lastAt) {
		when := lastAt
		entry.LastRefusedAt = &when
	}
	when := lastAt
	entry.Refusals = append(entry.Refusals, crmcontracts.ExtensionIngestRefusal{
		Refusal:       crmcontracts.ExtensionIngestRefusalRefusal(refusal),
		Refused:       int(refused),
		LastRefusedAt: &when,
	})
	return units
}
