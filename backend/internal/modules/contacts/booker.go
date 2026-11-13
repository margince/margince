// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EnsureBookerByEmail is EnsureContactByEmail for the public booking page. A
// live lead holding the address is the booker's record, and no contact is made.
//
// host is the booking page's host, who owns a contact the booking creates.
// created says the contact is new. An existing one is answered as it stands:
// FillBookerGaps fills it, once the caller has confirmed the host may book it.
func (s *Store) EnsureBookerByEmail(ctx context.Context, host ids.UserID, fullName, email, source string, erasure storekit.ErasureAdmission) (contact ids.UUID, lead ids.LeadID, created bool, err error) {
	contact, created, err = s.ensureContactByEmail(ctx, email, CreateContactInput{
		FullName:    fullName,
		OwnerID:     &host,
		Source:      source,
		Acquisition: Acquisition{Kind: AcquiredEventOrForm},
		erasure:     erasure,
	})
	if _, held := errors.AsType[*LeadHoldsIdentityError](err); !held {
		return contact, ids.LeadID{}, created, err
	}
	lead, err = s.CounterpartyLead(ctx, email)
	if err == nil && lead.IsZero() {
		// The lead the refusal named is gone, or a contact took the address:
		// the race ensureContactByEmail's claimed-email retry answers, met the
		// other way.
		err = apperrors.ErrConflict
	}
	return ids.Nil, lead, false, err
}

// FillBookerGaps gives a booker the host as owner and the domain's company as
// employer, each only where the contact has none. The caller carries the host's
// own authority. A contact the host may not change is left as it is.
//
// The employer goes through the inferred-employer resolution that a created
// contact and mail capture use, so all three treat a domain alike.
func (s *Store) FillBookerGaps(ctx context.Context, contact ids.UUID, host ids.UserID, source string) error {
	if refusedChange(auth.Require(ctx, entityContact, principal.ActionUpdate)) {
		return nil
	}
	cols, err := s.activeCustomColumns(ctx, entityContact, principal.ActionUpdate)
	if err != nil {
		return err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return err
	}
	id := ids.From[ids.ContactKind](contact)
	return s.txRetryingLockCycles(ctx, func(tx pgx.Tx) error {
		// The row lock comes first (lockorder.go). The write check and the owner
		// read run under it, so no owner or visibility change lands in between.
		if err := lockContactForAttach(ctx, tx, id); err != nil {
			return err
		}
		if err := auth.EnsureChangeable(ctx, tx, entityContact, contact); err != nil {
			if refusedChange(err) {
				return nil
			}
			return err
		}
		var owner *ids.UUID
		if err := tx.QueryRow(ctx, `SELECT owner_id FROM contact WHERE id = $1`, id).Scan(&owner); err != nil {
			return fmt.Errorf("contacts: reading a booker's owner: %w", err)
		}
		if owner == nil {
			if _, err := s.UpdateContactTx(ctx, tx, id, UpdateContactInput{OwnerID: &host, Source: source}, cols); err != nil {
				return err
			}
			owner = &host.UUID
		}
		return s.resolveCreatedEmployer(ctx, tx, id, ContactSpec{Source: source, CapturedBy: by, OwnerID: ownerFromUUID(owner)})
	})
}

// refusedChange reports a refusal that means the actor may not change the
// record, as opposed to a fault.
func refusedChange(err error) bool {
	return errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) ||
		errors.Is(err, apperrors.ErrSeatTierInsufficient)
}
