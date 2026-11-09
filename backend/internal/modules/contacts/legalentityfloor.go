// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// A member's mail is kept under the floor of the legal entity they work for.
// Moving a member, or changing an entity's country, can shorten that floor, so
// the write carries the old one for mail already held. privacy owns the carry
// and this module never imports it, so the composition root injects it.

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// EntityJurisdiction is what decides an entity's floor: whether it is the
// default entity, which follows the installation, and its own country.
type EntityJurisdiction struct {
	Default bool
	Country string
}

// FloorCarry keeps the floor that moving members from one jurisdiction to the
// other would shorten, in the caller's transaction.
type FloorCarry func(ctx context.Context, tx pgx.Tx, from, to EntityJurisdiction) error

// WithFloorCarry wires the carry every entity move and country change owes.
func (s *Store) WithFloorCarry(carry FloorCarry) *Store {
	s.floorCarry = carry
	return s
}

// WithFloorCarry wires the carry into the transport's own store.
func (h Handlers) WithFloorCarry(carry FloorCarry) Handlers {
	h.store = h.store.WithFloorCarry(carry)
	return h
}

var errFloorCarryUnwired = errors.New("contacts: nothing keeps the retention floor when a legal entity change shortens it")

// carryFloor runs the carry for a change between two jurisdictions. A change
// that keeps the jurisdiction asks nothing. Nor does one from the default
// entity, because no member's mail keeps less than the default floor.
func (s *Store) carryFloor(ctx context.Context, tx pgx.Tx, from, to EntityJurisdiction) error {
	if from == to || from.Default {
		return nil
	}
	if s.floorCarry == nil {
		return errFloorCarryUnwired
	}
	return s.floorCarry(ctx, tx, from, to)
}

// entityJurisdiction reads what decides one entity's floor.
func entityJurisdiction(ctx context.Context, tx pgx.Tx, id ids.LegalEntityID) (EntityJurisdiction, error) {
	entity, err := readLegalEntity(ctx, tx, id)
	if err != nil {
		return EntityJurisdiction{}, err
	}
	return jurisdictionOf(entity), nil
}

func jurisdictionOf(entity LegalEntity) EntityJurisdiction {
	out := EntityJurisdiction{Default: entity.IsDefault}
	if entity.Country != nil {
		out.Country = *entity.Country
	}
	return out
}
