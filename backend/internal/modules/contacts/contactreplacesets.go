// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// applyContactReplaceSets lands the social, email and phone replace-sets in a
// savepoint and reports whether they changed what the contact holds.
//
// The forms send every set on every save, so a save of an untouched contact
// carries them all. A set equal to the stored rows is rolled back and leaves
// the version alone. The comparison is of the rows read back, so it needs no
// copy of the normalisation the replace applies.
func applyContactReplaceSets(
	ctx context.Context, tx pgx.Tx, id ids.ContactID, current crmcontracts.Contact, in UpdateContactInput,
	active []fieldcatalog.Column,
) (bool, error) {
	if in.Social == nil && in.Emails == nil && in.Phones == nil {
		return false, nil
	}
	if err := auth.EnsureChangeable(ctx, tx, "contact", id.UUID); err != nil {
		return false, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return false, err
	}
	nested, err := tx.Begin(ctx)
	if err != nil {
		return false, fmt.Errorf("open the contact replace-sets: %w", err)
	}
	moved, err := func() (bool, error) {
		if in.Social != nil {
			if err := replaceContactSocial(ctx, nested, workspaceID(ctx), id, in.Social); err != nil {
				return false, err
			}
		}
		if in.Emails != nil || in.Phones != nil {
			if err := replaceContactEmails(ctx, nested, workspaceID(ctx), id, in.Source, by, in.Emails); err != nil {
				return false, err
			}
			if err := replaceContactPhones(ctx, nested, id, in.Source, by, in.Phones); err != nil {
				return false, err
			}
		}
		after, err := readContact(ctx, nested, id, storekit.LiveOnly, active)
		if err != nil {
			return false, fmt.Errorf("read the contact after its replace-sets: %w", err)
		}
		_, differs := storekit.ChangedColumns(contactSetsImage(current), contactSetsImage(after))
		return len(differs) > 0, nil
	}()
	if err != nil {
		if rbErr := nested.Rollback(ctx); rbErr != nil {
			return false, errors.Join(err, rbErr)
		}
		return false, err
	}
	if !moved {
		if err := nested.Rollback(ctx); err != nil {
			return false, fmt.Errorf("withdraw the replace-sets that changed nothing: %w", err)
		}
		return false, nil
	}
	if err := nested.Commit(ctx); err != nil {
		return false, fmt.Errorf("commit the contact replace-sets: %w", err)
	}
	return true, nil
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
