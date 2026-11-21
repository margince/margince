// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// recordEdit is what one update did: the row as it stood before it, and
// whether the update changed anything. The read for the caller is not in it.
type recordEdit[T any] struct {
	current T
	changed bool
}

// ownerAfterEdit is the owner an applied patch left the record with. Only the
// update's own owner_id sets or clears it, as the patch builders spell it.
func ownerAfterEdit(p *storekit.Patch, before *ids.UUID, set *ids.UserID) *ids.UUID {
	if _, touched := p.After()[ownerIDColumn]; !touched {
		return before
	}
	if set == nil {
		return nil
	}
	return &set.UUID
}

// ApplyContactUpdate is UpdateContact for a caller that does not read the
// contact back, so it costs no read for the caller.
func (s *Store) ApplyContactUpdate(ctx context.Context, id ids.ContactID, in UpdateContactInput) error {
	if err := auth.Require(ctx, "contact", principal.ActionUpdate); err != nil {
		return err
	}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		active, err := s.activeColumnsIn(ctx, tx, "contact")
		if err != nil {
			return err
		}
		_, err = s.writeContactInTx(ctx, tx, id, in, active)
		return err
	})
	return s.AskHolderToShare(ctx, err)
}

// ApplyCompanyUpdate is UpdateCompany for a caller that does not read the
// company back, so it costs no read for the caller.
func (s *Store) ApplyCompanyUpdate(ctx context.Context, id ids.CompanyID, in UpdateCompanyInput) error {
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		active, err := s.activeColumnsIn(ctx, tx, "company")
		if err != nil {
			return err
		}
		_, err = s.writeCompanyInTx(ctx, tx, id, in, active)
		return err
	})
}

// ApplyLeadUpdate is UpdateLead for a caller that does not read the lead back,
// so it costs no read for the caller.
func (s *Store) ApplyLeadUpdate(ctx context.Context, id ids.LeadID, in UpdateLeadInput) error {
	if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
		return err
	}
	return s.tx(ctx, func(tx pgx.Tx) error {
		active, err := s.activeColumnsIn(ctx, tx, "lead")
		if err != nil {
			return err
		}
		_, err = s.lockAndWriteLeadInTx(ctx, tx, id, in, active)
		return err
	})
}

func (s *Store) lockAndWriteLeadInTx(
	ctx context.Context, tx pgx.Tx, id ids.LeadID, in UpdateLeadInput, active []fieldcatalog.Column,
) (recordEdit[crmcontracts.Lead], error) {
	if err := ensureLeadUpdateAuthority(ctx, tx, id, in); err != nil {
		return recordEdit[crmcontracts.Lead]{}, err
	}
	// The lock comes before the read, so the before image is what the write
	// replaces. A recompute landing in between would otherwise move
	// score_computed under a clear that already captured the old value.
	lock, err := storekit.LockRow(ctx, tx, "lead", id.UUID, storekit.LiveOnly)
	if err != nil {
		return recordEdit[crmcontracts.Lead]{}, err
	}
	return s.writeLeadInTx(ctx, tx, lock, in, active)
}
