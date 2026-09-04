// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// Putting a name the record already knows onto the page.
//
// completePersonName beside this does the same thing as it LEARNS a name, under
// a predicate demanding both split columns be empty — it completes a record, and
// a record whose parts are already filled is not its business. This is for the
// contacts that learned their name before the display followed it: the parts are
// filled and correct, and only what a reader sees is stale.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// RefreshDisplayNameTx sets one contact's display name to the name its own
// columns already carry, and answers whether it moved.
//
// Only where no person has ever set that display name. It calls
// displayNameSetByHumanTx for that, which is the same function completePersonName
// calls, so today the two writers answer it identically — a shared helper rather
// than a claim that nothing could ever ask it differently.
//
// Asked again HERE rather than trusted to the caller's selector: a repair pass
// reads a page of ids and then writes them one at a time, and a colleague who
// renames one of those contacts in between must keep their name.
//
// It writes nothing when the display already agrees with the parts, so a pass
// over a repaired workspace costs one query and produces no audit noise.
func RefreshDisplayNameTx(ctx context.Context, tx pgx.Tx, personID ids.PersonID) (bool, error) {
	// Locked before it is read, for the reason completePersonName states: the
	// value recorded as the audit's before must be the one this write replaces,
	// and a human editing the name between an unlocked read and the write would
	// have their value recorded as the after of a change they did not make.
	var previous, first, last string
	err := tx.QueryRow(ctx, `
		SELECT full_name, coalesce(first_name, ''), coalesce(last_name, '')
		  FROM person WHERE id = $1 FOR UPDATE`, personID).Scan(&previous, &first, &last)
	if errors.Is(err, pgx.ErrNoRows) {
		// Erased or deleted. No name left to show.
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("people: reading the name person %s shows: %w", personID, err)
	}
	// Both halves, because a name is the pair. One half alone would put "Björn"
	// or "Welter" on the page in place of a label that at least identified
	// somebody, which is not an improvement.
	if first == "" || last == "" {
		return false, nil
	}
	humanNamed, err := displayNameSetByHumanTx(ctx, tx, personID)
	if err != nil {
		return false, err
	}
	if humanNamed {
		return false, nil
	}
	learned := strings.TrimSpace(first + " " + last)
	if learned == previous {
		return false, nil
	}
	tag, err := tx.Exec(ctx, `
		UPDATE person SET full_name = $2 WHERE id = $1 AND full_name = $3`,
		personID, learned, previous)
	if err != nil {
		return false, fmt.Errorf("people: showing the learned name of person %s: %w", personID, err)
	}
	if tag.RowsAffected() != 1 {
		// The display name changed under the lock, which means somebody else
		// wrote it. Auditing a change that did not happen would put a lie in the
		// trail.
		return false, nil
	}
	before, after := storekit.ChangedColumns(
		map[string]any{fieldFullName: previous},
		map[string]any{fieldFullName: learned},
	)
	auditID, err := storekit.Audit(ctx, tx, "update", entityPerson, personID.UUID, before, after)
	if err != nil {
		return false, err
	}
	if err := storekit.EmitEvent(ctx, tx, auditID, personID.UUID,
		crmcontracts.PublicEventPersonUpdated{
			ChangedFields: map[string]any{fieldFullName: learned},
		}); err != nil {
		return false, err
	}
	return true, nil
}
