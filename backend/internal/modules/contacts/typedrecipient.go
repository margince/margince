// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// typedRecipientSource is contact.source for a contact a rep made by writing to
// a new address: a human typed it, as on the create form.
const typedRecipientSource = "manual"

// FileTypedRecipientsTx gives each address no live contact or lead holds a
// minimal contact owned by the caller. It returns the folded addresses filed.
//
// A held address is left alone: no second record, no lead conversion, no new
// owner. The match is the exact folded address every identity writer locks. A
// plus-tag or Gmail-dot variant gets its own contact, and the create files the
// pair for review.
//
// Some addresses are never filed: an erased one, a colleague's, a sending
// system's, or any address when the caller lacks contact:create. The send's
// recipient check then refuses them.
func (s *Store) FileTypedRecipientsTx(ctx context.Context, tx pgx.Tx, addresses []string) ([]string, error) {
	err := auth.Require(ctx, contactEntity, principal.ActionCreate)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// Sorted, so two sends naming the same new addresses take their locks in
	// one order.
	folded := make([]string, 0, len(addresses))
	for _, address := range addresses {
		if key := correspondence.Fold(address); key != "" && refuseMachineAddress(key) == nil {
			folded = append(folded, key)
		}
	}
	slices.Sort(folded)
	folded = slices.Compact(folded)
	var filed []string
	for _, address := range folded {
		fileable, err := typedRecipientFileable(ctx, tx, address)
		if err != nil {
			return nil, err
		}
		if !fileable {
			continue
		}
		name := ParseContactName("", address)
		in := CreateContactInput{
			FullName:    name.Full,
			sourceNames: true,
			Emails:      []ContactEmailInput{{Email: address, EmailType: emailTypeWork, IsPrimary: true}},
			Source:      typedRecipientSource,
		}
		if name.Confident {
			in.FirstName, in.LastName = nameColumn(name.First), nameColumn(name.Last)
		}
		if _, err := s.CreateContactTx(ctx, tx, in, CustomColumns{}); err != nil {
			return nil, err
		}
		filed = append(filed, address)
	}
	return filed, nil
}

// typedRecipientFileable takes the locks a contact create of this address
// takes, in capture's order, and reports whether the address is free to file.
func typedRecipientFileable(ctx context.Context, tx pgx.Tx, address string) (bool, error) {
	erased, err := storekit.EmailSuppressed(ctx, tx, address)
	if err != nil || erased {
		return false, err
	}
	seats, err := storekit.SeatAddressesFor(ctx, tx, address)
	if err != nil || seats.Covers(address, ids.Nil) {
		return false, err
	}
	holders, err := storekit.LockIdentityHolders(ctx, tx, storekit.IdentityKeys{Emails: []string{address}})
	if err != nil {
		return false, err
	}
	return !holders.ContactHoldsEmail(address) && !holders.LeadHoldsEmail(address), nil
}
