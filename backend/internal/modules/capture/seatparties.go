// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// workspaceParties is who counts as "us" when capture picks the party a
// message is about. That is the acting seat's own set, and every address
// storekit.SeatAddresses names as a seat's from this seat's mailbox.
//
// It decides only whom a record is about. A seat may sign in at a customer's
// address. So the acting seat's set and the trusted domains still decide
// whether mail is internal and who may open it.
type workspaceParties struct {
	self   SelfSet
	seats  storekit.SeatAddresses
	asking ids.UUID
}

func workspacePartiesTx(ctx context.Context, tx pgx.Tx) (workspaceParties, error) {
	self, err := ownerIdentitiesTx(ctx, tx)
	if err != nil {
		return workspaceParties{}, err
	}
	seats, err := storekit.WorkspaceSeatAddresses(ctx, tx)
	if err != nil {
		return workspaceParties{}, err
	}
	_, asking := capturePrincipal(ctx)
	return workspaceParties{self: self, seats: seats, asking: asking}, nil
}

// Covers reports whether address is the acting seat's own or a seat's.
func (w workspaceParties) Covers(address string) bool {
	return w.self.Covers(address) || w.seats.Covers(address, w.asking)
}

// Without is the addresses Covers refuses, in header order.
func (w workspaceParties) Without(addresses []string) []string {
	out := make([]string, 0, len(addresses))
	for _, address := range w.self.WithoutSelf(addresses) {
		if !w.seats.Covers(address, w.asking) {
			out = append(out, address)
		}
	}
	return out
}
