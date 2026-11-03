// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The lead object's own create path.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/migration"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// errImportReplayed aborts a landing that wrote nothing: the store answered with
// a record that already existed under its natural key. Rolling back keeps the
// identity row out of the map. Adopting a record this run did not create would
// make the next attempt resolve it as already imported.
var errImportReplayed = errors.New("import: the record replayed under its natural key")

// leadImportOrigin is where a contact's owner reads a refused import row came from.
const leadImportOrigin = "a CSV import"

func (w *csvWriters) createLead(ctx context.Context, row migration.Row, typed map[string]any) (migration.EnsureResult, error) {
	in := leadCreateFrom(textFields(row.Fields), typed, w.sourceSystem, row.ExternalID, w.provenanceOf(row.ExternalID))
	err := w.land(ctx, row, func(tx pgx.Tx) (ids.UUID, error) {
		if err := w.authors.resolve(ctx, tx, &in.Author); err != nil {
			return ids.UUID{}, err
		}
		lead, created, err := w.contacts.CreateLeadTx(ctx, tx, in, w.cols.store)
		if err != nil {
			return ids.UUID{}, fmt.Errorf("import: creating lead %s: %w", row.ExternalID, err)
		}
		if !created {
			return ids.UUID{}, errImportReplayed
		}
		return ids.UUID(lead.Id), nil
	})
	// The row's own transaction has ended, so the owner of a contact the row
	// names is told, as on every other lead door.
	err = w.contacts.TellContactOwnerOfLead(ctx, err, in, leadImportOrigin)
	if _, held := errors.AsType[*contacts.ContactHoldsIdentityError](err); held {
		return skippedFor(contactHoldsLeadWhy), nil
	}
	if _, held := errors.AsType[*contacts.DuplicateLeadError](err); held || errors.Is(err, errImportReplayed) {
		return skippedFor(naturalKeyTakenWhy), nil
	}
	if linkedInClaimed(err) {
		return skippedFor(linkedInClaimedWhy), nil
	}
	if refusedAsErased(err) {
		return skippedFor(addressErasedWhy), nil
	}
	if err != nil {
		return migration.EnsureResult{}, err
	}
	return migration.EnsureResult{Created: true}, nil
}

// leadEmailAlreadyHeld is the refusal CreateLeadTx would make, asked before the
// write so the preview skips the row the commit will. Like a contact's address
// it is estate-wide and not visibility-filtered: the outcome is the same
// whoever holds the address.
func (w *csvWriters) leadEmailAlreadyHeld(ctx context.Context, row migration.Row) (bool, error) {
	if w.object != migration.ObjectLead {
		return false, nil
	}
	email := strings.TrimSpace(textFields(row.Fields)[fieldEmail])
	var held bool
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		var err error
		_, held, err = storekit.LiveLeadByEmail(ctx, tx, email, nil)
		return err
	})
	if err != nil {
		return false, fmt.Errorf("import: checking whether %q is already held: %w", email, err)
	}
	return held, nil
}

// leadRowContactHeld is the contact-held refusal CreateLeadTx would make, asked
// before the write so the preview skips the row the commit will. Like the lead
// probes beside it, it is not visibility-filtered: the outcome is the same
// whoever holds the key.
func (w *csvWriters) leadRowContactHeld(ctx context.Context, row migration.Row, typed map[string]any) (bool, error) {
	if w.object != migration.ObjectLead {
		return false, nil
	}
	var keys storekit.IdentityKeys
	if email := strings.TrimSpace(textFields(row.Fields)[fieldEmail]); email != "" {
		keys.Emails = []string{email}
	}
	if url, ok := typed[fieldLinkedIn].(string); ok && url != "" {
		keys.LinkedInURLs = []string{url}
	}
	var held bool
	err := database.WithWorkspaceTx(ctx, w.pool, func(tx pgx.Tx) error {
		holders, err := storekit.IdentityHoldersOf(ctx, tx, keys)
		held = len(holders.Contacts) > 0
		return err
	})
	if err != nil {
		return false, fmt.Errorf("import: checking whether a contact holds this lead row: %w", err)
	}
	return held, nil
}
