// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Putting the name we learned onto the page, for the records that learned it
// before the display followed.
//
// A contact minted from a calendar invitation is named the way the ORGANIZER had
// them saved — "Bw" for Björn Welter, "Juan" for Judith Andresen. A signature or
// a fuller invitation later teaches the record the real name, which lands in
// first_name and last_name; until now the display name stayed on the label. The
// fill has been widened to move it, and this is the pass for the rows that
// already went through the fill under the old rule.
//
// A companion to the attendee-name recovery beside it, not an arm of it: that
// pass drains on activity_participant.display_name IS NULL, which these rows no
// longer are — their attendee names were recovered, and it is the PERSON's
// display that stayed behind. Same originals, different question, so a separate
// selector that drains on its own answer.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/margince/margince/backend/internal/modules/people"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// repairStaleDisplayNamesBatch puts the learned name on the page for up to limit
// people whose display still shows what a machine first guessed.
func repairStaleDisplayNamesBatch(ctx context.Context, pool *pgxpool.Pool, limit int, log *slog.Logger) (int, error) {
	if limit <= 0 {
		return 0, nil
	}
	// storekit.EmitEvent REFUSES to publish without a correlation id, so a pass
	// that omitted one would repair nothing at all: every person would fail on
	// their own event, the batch would roll back, and the backlog would sit there
	// looking as though the job had simply not run yet.
	ctx = principal.WithCorrelationID(ctx, ids.NewV7())
	var repaired int
	err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		stale, err := selectStaleDisplayNames(ctx, tx, limit)
		if err != nil {
			return err
		}
		for _, personID := range stale {
			// Through the module that owns the write, under its own guards: the
			// human-precedence check runs again here, so a name somebody typed
			// between the select and this call is still theirs.
			moved, err := people.RefreshDisplayNameTx(ctx, tx, personID)
			if err != nil {
				return err
			}
			if moved {
				repaired++
			}
		}
		return nil
	})
	if err != nil {
		return 0, err
	}
	if repaired > 0 {
		log.InfoContext(ctx, "display name repair: contacts now show the name we learned",
			"people", repaired)
	}
	return repaired, nil
}

// selectStaleDisplayNames finds people whose split columns name them and whose
// display name says something else, where no person ever set that display.
//
// The two human tests are the same pair people.RefreshDisplayNameTx applies, and
// they are restated here for the reason every drain restates its writer's
// predicate: a selector that offered rows the write refuses would return the
// same page every tick and never empty.
func selectStaleDisplayNames(ctx context.Context, tx pgx.Tx, limit int) ([]ids.PersonID, error) {
	rows, err := tx.Query(ctx, `
		SELECT p.id
		  FROM person p
		 WHERE p.first_name IS NOT NULL
		   AND p.last_name IS NOT NULL
		   AND p.full_name IS DISTINCT FROM (p.first_name || ' ' || p.last_name)
		   AND p.captured_by NOT LIKE 'human:%'
		   AND NOT EXISTS (
		       SELECT 1 FROM audit_log a
		        WHERE a.entity_type = 'person' AND a.entity_id = p.id
		          AND a.actor_type = 'human' AND a.action = 'update'
		          AND (a.after ? 'full_name' OR a.before ? 'full_name'))
		 ORDER BY p.id
		 LIMIT $1`, limit)
	if err != nil {
		return nil, fmt.Errorf("compose: selecting the contacts whose display name is stale: %w", err)
	}
	defer rows.Close()
	var out []ids.PersonID
	for rows.Next() {
		var id ids.PersonID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("compose: reading a contact whose display name is stale: %w", err)
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("compose: selecting the contacts whose display name is stale: %w", err)
	}
	return out, nil
}
