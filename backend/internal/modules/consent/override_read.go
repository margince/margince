// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// The read half of a rep's override: a standing row that outranks the
// machine's own reading for one subject and one category, and nothing else.
//
// The write door does not exist yet — communication_override is seeded
// directly by whatever writes it, and this file only answers "is there a live
// one that applies here". decideOne (authorizetransmit.go) is the only caller,
// and it asks ONLY when the decision is already known to be CanBeOverruled —
// a machine-level, non-absolute refusal — so this never has to re-derive that.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// liveOverride finds a standing override that outranks the machine for this
// subject and category. It is consulted ONLY when the decision is
// CanBeOverruled, so it never has to re-check the reason: a subject act or an
// absolute machine fact never reaches here.
//
// The level filter derives from LevelsWeakestFirst rather than a retyped rank,
// so "outranks machine" here reads the same ladder CanOverrule elsewhere
// compares against, instead of a second ranking that could fall out of step
// with it. Every seat level (user, admin) outranks machine; the door writes no
// other, but the query states the rule rather than trusting the writer.
func liveOverride(
	ctx context.Context, tx pgx.Tx, contactID string, category commsauthz.Category,
) (ids.UUID, bool, error) {
	var id ids.UUID
	err := tx.QueryRow(ctx, `
		SELECT id FROM communication_override
		 WHERE revoked_at IS NULL
		   AND category = $2
		   AND decided_by_level = ANY($3)
		   AND (contact_id = $1 OR lead_id = $1)
		 ORDER BY recorded_at DESC
		 LIMIT 1`,
		contactID, string(category), levelsAboveMachine).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ids.UUID{}, false, nil
	}
	if err != nil {
		return ids.UUID{}, false, fmt.Errorf("consent: reading a live override: %w", err)
	}
	return id, true, nil
}

// levelsAboveMachine is the authority levels strictly above LevelMachine,
// computed once from the same ladder AuthorityLevel.CanOverrule compares
// against. A standing override is written only at a seat level, but the query
// states the rule rather than trusting the writer.
var levelsAboveMachine = computeLevelsAboveMachine()

func computeLevelsAboveMachine() []string {
	var above []string
	for _, l := range commsauthz.LevelsWeakestFirst() {
		if l.CanOverrule(commsauthz.LevelMachine) {
			above = append(above, string(l))
		}
	}
	return above
}
