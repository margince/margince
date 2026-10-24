// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
)

// SeatAddressSet holds every address a live seat entered or connected, folded
// to lower case. An address two seats both claim still counts, and an address
// counts before it is verified.
//
// The set only decides. Never return its members to a client, because one
// seat's private aliases are not a fact for the rest of the workspace.
type SeatAddressSet map[string]struct{}

// WorkspaceSeatAddresses reads the set from the same records MailSeatIdentities
// folds. A domain-kind identity is not read, so it covers no address.
func WorkspaceSeatAddresses(ctx context.Context, tx pgx.Tx) (SeatAddressSet, error) {
	bySeat, err := MailSeatIdentities(ctx, tx)
	if err != nil {
		return nil, err
	}
	set := SeatAddressSet{}
	for _, addresses := range bySeat {
		for _, address := range addresses {
			set[address] = struct{}{}
		}
	}
	return set, nil
}

// Covers reports whether address, in any case, is a seat address.
func (s SeatAddressSet) Covers(address string) bool {
	_, ok := s[mailidentity.ReaderAddress(address, "")]
	return ok
}

// Sorted lists the addresses for binding as one SQL array argument.
func (s SeatAddressSet) Sorted() []string {
	out := make([]string, 0, len(s))
	for address := range s {
		out = append(out, address)
	}
	sort.Strings(out)
	return out
}
