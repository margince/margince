// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// sameAddressSQL is the gate's one spelling of "column holds this address":
// case-insensitive on the whole address. Every lookup of an address here uses
// it, so the gate and the questions asked of it cannot match differently.
func sameAddressSQL(column, param string) string {
	return "lower(" + column + ") = lower(" + param + ")"
}

// AddressHasSubject reports whether any contact or lead holds the address,
// archived or promoted ones included. Such an address must go to the gate,
// which answers only for live subjects and refuses the rest.
func (g *Gate) AddressHasSubject(ctx context.Context, address string) (bool, error) {
	var held bool
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM contact_email pe WHERE `+sameAddressSQL("pe.email", "$1")+`)
			    OR EXISTS (SELECT 1 FROM lead l WHERE `+sameAddressSQL("l.email", "$1")+`)`, address).Scan(&held)
	})
	if err != nil {
		return false, fmt.Errorf("consent: read whether a record holds an address: %w", err)
	}
	return held, nil
}

// AddressSuppressed reports a live suppression recorded against the address
// itself, such as a hard bounce. It is the stop that still binds an address no
// contact or lead holds.
func (g *Gate) AddressSuppressed(ctx context.Context, address string) (bool, error) {
	var stops []liveStop
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		stops, err = liveSuppression(ctx, tx, ids.Nil.String(), connector.Recipient{Email: address})
		return err
	})
	return len(stops) > 0, err
}
