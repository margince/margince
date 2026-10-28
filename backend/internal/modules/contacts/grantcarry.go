// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A merge joins two records of one real contact, company or lead, so a share
// given on the retired record keeps working on the survivor. identity owns
// record_grant and the audit shape of a grant change; contacts only asks,
// inside the merge's transaction.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
)

// GrantCarrier moves the grants on a retiring record onto its survivor, folding
// two grants of one grantee into one, and keeps a former owner's access there.
type GrantCarrier interface {
	CarryGrantsTx(ctx context.Context, tx pgx.Tx, recordType string, from, to ids.UUID) error
	CarryOwnerAccessTx(ctx context.Context, tx pgx.Tx, recordType string, record, owner, mergedFrom ids.UUID) (authz.OwnerAccessCarry, error)
}

// WithGrantCarrier wires the identity-side grant carry. Compose binds it.
func (s *Store) WithGrantCarrier(carrier GrantCarrier) *Store {
	s.grantCarrier = carrier
	return s
}

// carryGrantsTx refuses only when an unwired seam would strand a grant, the
// rule carryIntrosTx spells for introduction asks.
func (s *Store) carryGrantsTx(ctx context.Context, tx pgx.Tx, recordType string, from, to ids.UUID) error {
	if s.grantCarrier != nil {
		return s.grantCarrier.CarryGrantsTx(ctx, tx, recordType, from, to)
	}
	return refuseUnwiredGrantCarry(ctx, tx, recordType, from)
}

// carryOwnerAccessTx keeps the access of a contact's former owner on the
// record that survives it. The survivor's own owner needs no grant. Unwired, it
// refuses: the merge would otherwise take the record away from its owner.
func (s *Store) carryOwnerAccessTx(ctx context.Context, tx pgx.Tx, survivor ids.ContactID, survivorOwner, formerOwner *ids.UUID, mergedFrom ids.UUID) (*authz.OwnerAccessCarry, error) {
	if formerOwner == nil {
		return nil, nil
	}
	if sameOwner(survivorOwner, formerOwner) {
		return &authz.OwnerAccessCarry{Owner: *formerOwner, Skipped: authz.OwnerCarryAlreadyOwner}, nil
	}
	if s.grantCarrier == nil {
		return nil, &SatelliteCarrierNotWiredError{Holding: "an owner other than the survivor's"}
	}
	carry, err := s.grantCarrier.CarryOwnerAccessTx(ctx, tx, contactEntity, survivor.UUID, *formerOwner, mergedFrom)
	if err != nil {
		return nil, fmt.Errorf("keep the former owner's access: %w", err)
	}
	return &carry, nil
}

func refuseUnwiredGrantCarry(ctx context.Context, tx pgx.Tx, recordType string, from ids.UUID) error {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM record_grant WHERE record_type = $1 AND record_id = $2)`,
		recordType, from).Scan(&held); err != nil {
		return fmt.Errorf("contacts: checking whether the retiring %s is shared with anyone: %w", recordType, err)
	}
	if held {
		return &SatelliteCarrierNotWiredError{Holding: "shares with colleagues"}
	}
	return nil
}

type grantCarryPreviewer interface {
	PreviewGrantCarryTx(context.Context, pgx.Tx, string, ids.UUID, ids.UUID) ([]authz.CarriedRecordGrant, error)
}

func (s *Store) previewGrantCarry(ctx context.Context, tx pgx.Tx, kind string, source, target ids.UUID) ([]authz.CarriedRecordGrant, error) {
	if reader, ok := s.grantCarrier.(grantCarryPreviewer); ok {
		return reader.PreviewGrantCarryTx(ctx, tx, kind, source, target)
	}
	return nil, refuseUnwiredGrantCarry(ctx, tx, kind, source)
}
