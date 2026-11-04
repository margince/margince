// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
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

// invitationCategory is what a calendar invitation to a booked meeting is: the
// guest agreed to meet, so it continues something they asked for. A marketing
// objection does not bind it; a bounce, a restriction or a general stop does.
const invitationCategory = commsauthz.CategoryRequestedFollowup

// InvitationSuppressed reports a live suppression recorded against the address
// itself, such as a hard bounce, that binds a calendar invitation. The rule is
// suppressionBinds, the one every send is judged by.
func (g *Gate) InvitationSuppressed(ctx context.Context, address string) (bool, error) {
	var stops []liveStop
	err := g.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var err error
		stops, err = liveSuppression(ctx, tx, ids.Nil.String(), connector.Recipient{Email: address})
		return err
	})
	return suppressionBindsAny(stops, invitationCategory, nil), err
}
