// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package person360

// The deliverability section: which of the person's addresses last refused a
// delivery, derived from the send ledger under the page's own snapshot.

import (
	"context"
	"sort"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// deadAddressesSection derives which of the person's addresses last refused a
// delivery, from the send ledger the page's transaction already sees. Absent
// addresses are simply not dead; the section reports the sorted survivors so
// the page and the identity card agree on order.
func (s *Service) deadAddressesSection(ctx context.Context, tx pgx.Tx, out *crmcontracts.Person360) error {
	// The store is asked even for a person with no addresses: its grant
	// check is what makes the withheld arm true, and a fast path around it
	// would show a grant-less caller an empty section instead of naming it.
	var addresses []string
	if out.Person.Emails != nil {
		addresses = make([]string, 0, len(*out.Person.Emails))
		for _, email := range *out.Person.Emails {
			addresses = append(addresses, string(email.Email))
		}
	}
	dead, err := s.comms.DeadAddressesTx(ctx, tx, addresses)
	if err != nil {
		return err
	}
	marked := make([]string, 0, len(dead))
	for address := range dead {
		marked = append(marked, address)
	}
	sort.Strings(marked)
	out.DeadAddresses = &marked
	return nil
}
