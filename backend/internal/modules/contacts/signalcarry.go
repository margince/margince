// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SignalCarrier moves the signals module's resolutions of a merged-away contact
// or company onto the survivor, in the merge's transaction. Compose binds it.
type SignalCarrier func(ctx context.Context, tx pgx.Tx, kind string, from, to ids.UUID) error

// WithSignalCarrier wires the signals-side resolution carry.
func (s *Store) WithSignalCarrier(carry SignalCarrier) *Store {
	s.signalCarrier = carry
	return s
}

// WithSignalCarrier wires the signals-side resolution carry.
func (h Handlers) WithSignalCarrier(carry SignalCarrier) Handlers {
	h.store.WithSignalCarrier(carry)
	return h
}

// WithSignalCarrier wires the signals-side resolution carry.
func (p *Provider) WithSignalCarrier(carry SignalCarrier) *Provider {
	p.store.WithSignalCarrier(carry)
	return p
}

// carrySignalResolutionsTx refuses only when an unwired seam would strand a
// resolution, the rule carryStopsTx spells for stops.
func (s *Store) carrySignalResolutionsTx(ctx context.Context, tx pgx.Tx, kind string, from, to ids.UUID) error {
	if s.signalCarrier != nil {
		return s.signalCarrier(ctx, tx, kind, from, to)
	}
	return refuseUnwiredSignalCarry(ctx, tx, from)
}

// refuseUnwiredSignalCarry reads the tables the signals carry moves, for the
// record ids alone, so one query answers for either kind.
func refuseUnwiredSignalCarry(ctx context.Context, tx pgx.Tx, from ids.UUID) error {
	var held bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM signal WHERE resolved_contact_id = $1 OR resolved_company_id = $1)
		    OR EXISTS (SELECT 1 FROM signal_resolution WHERE matched_company_id = $1)`, from).Scan(&held); err != nil {
		return fmt.Errorf("contacts: checking whether signals resolve to the retiring record: %w", err)
	}
	if held {
		return &SatelliteCarrierNotWiredError{Holding: "signals resolved to it"}
	}
	return nil
}
