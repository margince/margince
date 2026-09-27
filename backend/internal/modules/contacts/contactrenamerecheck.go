// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The contact rename re-check: the company re-check (renamerecheck.go) applied
// to contacts.
//
// A contact is checked for duplicates once, when it is created, with the name
// it has then. A contact captured from mail or imported without a name starts
// as its address ("sahner"), and the real name arrives later by an edit
// ("Gerhard Sahner"). That edit is the moment the duplicate becomes visible —
// the same contact already sat under another address — and nothing looked.
//
// Every write that changes full_name re-runs the fuzzy tier and the
// name-collision lane and files what they find. Unlike the company re-check it
// also runs for a human's edit: a human correcting a name is exactly who
// reveals the twin. It never merges and never overrules the rename; the pair
// goes on the review queue.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// contactRenameRecheckSource names this detector on the queue rows it files.
const contactRenameRecheckSource = "contact_rename_recheck"

// renamesAContact reports whether an update moves the contact's name. A re-sent
// spelling of the same name is no rename: it costs a scan and reveals nothing.
func renamesAContact(currentName string, in UpdateContactInput) bool {
	return in.FullName != nil && NormalizeContactName(*in.FullName) != NormalizeContactName(currentName)
}

// recheckIfRenamed runs the re-check when an update moved the contact's name.
//
// The name-lane lock is taken here, after the row lock the update already
// holds. That order cannot deadlock against a create: a create takes the name
// lock and then inserts a NEW row, never waiting on an existing one, and the
// re-check below only reads other contacts.
func recheckIfRenamed(ctx context.Context, tx pgx.Tx, id ids.ContactID, currentName string, in UpdateContactInput) error {
	if !renamesAContact(currentName, in) {
		return nil
	}
	if err := lockNameLane(ctx, tx, *in.FullName); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	return recheckContactNameForDuplicates(ctx, tx, id, by)
}

// recheckContactNameForDuplicates scores a renamed contact against the rest of
// the workspace and files the pair the fuzzy tier or the name-collision lane
// names. The caller holds the name-lane lock for the new name (lockNameLane), so
// two renames converging on one name cannot both read no incumbent and land
// unfiled.
//
// The exact lanes are skipped: the contact holds its own addresses and phones,
// so they would name itself. Its addresses still ride the candidate, because
// the fuzzy tier's employer term reads their domains.
func recheckContactNameForDuplicates(ctx context.Context, tx pgx.Tx, id ids.ContactID, by string) error {
	var name string
	var emails []string
	err := tx.QueryRow(ctx, `
		SELECT p.full_name,
		       coalesce((SELECT array_agg(e.email) FROM contact_email e
		                  WHERE e.contact_id = p.id AND e.archived_at IS NULL), '{}')
		  FROM contact p
		 WHERE p.id = $1 AND p.archived_at IS NULL`, id).Scan(&name, &emails)
	if err != nil {
		return fmt.Errorf("contacts: reading the renamed contact: %w", err)
	}
	if NormalizeContactName(name) == "" {
		return nil
	}
	match, err := fuzzyContact(ctx, tx, ContactCandidate{
		FullName: name, Emails: emails, QueueNameCollisions: true, ExcludeID: &id,
	})
	if err != nil {
		return err
	}
	return match.recordIfReview(ctx, tx, id, name, contactRenameRecheckSource, by)
}
