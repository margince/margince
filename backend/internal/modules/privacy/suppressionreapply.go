// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

// Erased stays erased, whatever brought it back.
//
// The suppression list guards the door mail comes through; a restore puts rows
// back underneath it. A resurrected subject is invisible because the record
// looks exactly like one that was never erased, so the list is re-applied
// blind to how the row arrived.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReapplySuppressionsReason is the reason the re-erasure records, so the
// second tombstone is distinguishable from the first: the trail should say a
// subject was erased twice and why, not appear to repeat itself.
const ReapplySuppressionsReason = "resurrected after erasure; suppression list reapplied"

// ReapplySuppressions re-erases every live subject whose address is on the
// suppression list, and reports how many it found.
//
// Zero is the answer on a healthy installation, and that is the point: the
// pass is cheap to run and its result is evidence either way. A non-zero
// answer is not a failure of this pass — it is this pass catching one.
//
// The hashing happens in Go, through the same SuppressionHash the eraser
// writes with. Doing it in SQL would mean lower()/btrim() standing in for
// Go's normalization, and the two agree on ASCII and not everywhere else —
// a second spelling of the hash silently forks the list, which is the one
// mistake suppression.go names in its own header.
func (e *Eraser) ReapplySuppressions(ctx context.Context) (int, error) {
	// The same gate the erasure itself asks for, asked once at the top. The
	// scan below reads every contact in the installation to decide which ones
	// the list already ended, and a caller who may not delete a contact has no
	// business being handed that reading either.
	if err := auth.Require(ctx, "contact", principal.ActionDelete); err != nil {
		return 0, err
	}
	suppressed, err := e.suppressedEmailHashes(ctx)
	if err != nil {
		return 0, err
	}
	// An empty list is the restore case the journal answers; keep scanning.
	resurrected, err := e.contactsMatchingSuppression(ctx, suppressed)
	if err != nil {
		return 0, err
	}
	var taken int
	var refused error
	for _, contactID := range resurrected {
		// One transaction each, through the ordinary erasure: a pass that
		// reached into the cascade's steps would be a second eraser to keep in
		// step with the first, and the thing most likely to drift is exactly
		// the part that decides what "erased" means.
		//
		// A subject the eraser refuses does NOT stop the others. A legal hold
		// is the refusal that matters here and it is per subject: one held
		// contact halting the pass would leave every resurrection after them
		// standing, and the hold — which is somebody else's obligation over
		// one record — would silently become a reason the whole installation
		// kept data it was told to erase. So the pass takes what it may, and
		// the refusals travel back joined rather than swallowed: the count
		// says what was done and the error says what could not be.
		if err := e.EraseContact(ctx, contactID, ReapplySuppressionsReason); err != nil {
			refused = errors.Join(refused,
				fmt.Errorf("privacy: re-erasing resurrected contact %s: %w", contactID, err))
			continue
		}
		taken++
	}
	return taken, refused
}

// suppressedEmailHashes reads the standing instruction.
func (e *Eraser) suppressedEmailHashes(ctx context.Context) (map[string]struct{}, error) {
	hashes := map[string]struct{}{}
	if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx,
			`SELECT value_hash FROM erasure_suppression WHERE kind = 'email'`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var hash string
			if err := rows.Scan(&hash); err != nil {
				return err
			}
			hashes[hash] = struct{}{}
		}
		return rows.Err()
	}); err != nil {
		return nil, fmt.Errorf("privacy: reading the suppression list: %w", err)
	}
	return hashes, nil
}

// contactsMatchingSuppression finds the live contacts an erasure should
// already have taken.
//
// Every live address is hashed rather than every suppressed hash being
// searched for: the list is the smaller side on a healthy installation, and
// the comparison has to happen in Go anyway.
func (e *Eraser) contactsMatchingSuppression(ctx context.Context, suppressed map[string]struct{}) ([]ids.UUID, error) {
	type candidate struct {
		contact ids.UUID
		hash    string
	}
	// The database read finishes before any of it is asked about. Probing the
	// journal inside the cursor would hold one transaction open across a
	// network round trip per address, so a slow store would keep a read
	// transaction alive for as long as it took and a failing one would roll
	// the whole scan back.
	var matched []ids.UUID
	var ask []candidate
	seen := map[ids.UUID]struct{}{}
	if err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		rows, err := tx.Query(ctx, `
			SELECT ce.contact_id, ce.email
			  FROM contact_email ce
			  JOIN contact c ON c.id = ce.contact_id AND c.archived_at IS NULL
			 ORDER BY ce.contact_id`)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var contactID ids.UUID
			var email string
			if err := rows.Scan(&contactID, &email); err != nil {
				return err
			}
			if _, already := seen[contactID]; already {
				continue
			}
			hash := storekit.SuppressionHash(email)
			if _, ok := suppressed[hash]; ok {
				seen[contactID] = struct{}{}
				matched = append(matched, contactID)
				continue
			}
			ask = append(ask, candidate{contact: contactID, hash: hash})
		}
		return rows.Err()
	}); err != nil {
		return nil, fmt.Errorf("privacy: looking for resurrected subjects: %w", err)
	}

	// Whatever the database's own list did not name, the exported journal
	// might: that is the restore case, where the rows came back and the list
	// that would have caught them rolled back with them.
	for _, c := range ask {
		if _, already := seen[c.contact]; already {
			continue
		}
		journaled, err := e.journalSuppressed(ctx, "email", c.hash)
		if err != nil {
			return nil, err
		}
		if !journaled {
			continue
		}
		seen[c.contact] = struct{}{}
		matched = append(matched, c.contact)
	}
	return matched, nil
}
