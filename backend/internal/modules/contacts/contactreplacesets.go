// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// replaceSetsChange is what a save's replace-sets changed: anything at all,
// which bumps the version, and the identifiers, which re-run the duplicate check.
type replaceSetsChange struct {
	any, emails, phones bool
}

// applyContactReplaceSets lands the social, email and phone replace-sets in a
// savepoint and reports what they changed in what the contact holds.
//
// The forms send every set on every save, so a save of an untouched contact
// carries them all. A set equal to the stored rows is rolled back and leaves
// the version alone. The comparison is of the rows read back, so it needs no
// copy of the normalisation the replace applies.
func applyContactReplaceSets(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, current crmcontracts.Contact, in UpdateContactInput,
	active []fieldcatalog.Column,
) (replaceSetsChange, error) {
	var change replaceSetsChange
	if in.Social == nil && in.Emails == nil && in.Phones == nil {
		return change, nil
	}
	kept, err := storekit.Savepoint(ctx, tx, func(nested pgx.Tx) (bool, error) {
		var err error
		change, err = replaceContactSetsAndCompare(ctx, nested, id, current, in, active)
		return change.any, err
	})
	if err != nil || !kept {
		return replaceSetsChange{}, err
	}
	return change, nil
}

// replaceContactSetsAndCompare runs the replaces and reads the contact back.
func replaceContactSetsAndCompare(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, current crmcontracts.Contact, in UpdateContactInput,
	active []fieldcatalog.Column,
) (replaceSetsChange, error) {
	if err := replaceContactSets(ctx, tx, id, in); err != nil {
		return replaceSetsChange{}, err
	}
	after, err := readContactRow(ctx, tx, id, storekit.LiveOnly, active)
	if err != nil {
		return replaceSetsChange{}, fmt.Errorf("read the contact after its replace-sets: %w", err)
	}
	_, differs := storekit.ChangedColumns(contactSetsImage(current), contactSetsImage(after))
	_, emails := differs["emails"]
	_, phones := differs["phones"]
	return replaceSetsChange{any: len(differs) > 0, emails: emails, phones: phones}, nil
}

// replaceContactSets writes each set the update carries. It takes the write
// check itself, so the writes below it never run on a row the caller may not
// change.
func replaceContactSets(ctx context.Context, tx pgx.Tx, id ids.ContactID, in UpdateContactInput) error {
	if err := auth.EnsureChangeable(ctx, tx, "contact", id.UUID); err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	if in.Social != nil {
		if err := replaceContactSocial(ctx, tx, workspaceID(ctx), id, in.Social, in.converting); err != nil {
			return err
		}
	}
	if in.Emails == nil && in.Phones == nil {
		return nil
	}
	if err := replaceContactEmails(ctx, tx, workspaceID(ctx), id, in.Source, by, in.Emails, in.converting); err != nil {
		return err
	}
	return replaceContactPhones(ctx, tx, id, in.Source, by, in.Phones)
}

// contactSetsImage holds what a replace-set can change, without the row ids and
// stamps a retained row keeps or rewrites.
//
//craft:ignore naked-any the image is the schemaless column map storekit compares
func contactSetsImage(c crmcontracts.Contact) map[string]any {
	type address struct {
		Value   string
		Type    string
		Primary bool
	}
	var emails, phones []address
	if c.Emails != nil {
		for _, e := range *c.Emails {
			emails = append(emails, address{string(e.Email), string(e.EmailType), e.IsPrimary})
		}
	}
	if c.Phones != nil {
		for _, p := range *c.Phones {
			phones = append(phones, address{p.Phone, string(p.PhoneType), p.IsPrimary})
		}
	}
	return map[string]any{"emails": emails, "phones": phones, "social": c.Social}
}
