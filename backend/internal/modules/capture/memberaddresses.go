// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MemberAddresses is every address proven to belong to one of the workspace's
// members, read across all seats. Only proven addresses count: a declared
// identity or a typed IMAP label is a claim, and one seat claiming an outsider
// would silently hold every colleague's mail with that outsider.
type MemberAddresses struct{ known SelfSet }

// MemberAddressesTx reads the set inside the workspace's transaction: each
// seat's login and each mailbox address a provider attested (provedAddressesTx,
// the rule SeatProvedAddressTx trusts). An archived seat no longer counts. An
// invited or departed one does, as in colleagueTx: their mail was still the
// workspace's mail.
func MemberAddressesTx(ctx context.Context, tx pgx.Tx) (MemberAddresses, error) {
	rows, err := tx.Query(ctx, `SELECT id, email FROM app_user WHERE archived_at IS NULL`)
	if err != nil {
		return MemberAddresses{}, fmt.Errorf("capture: reading the members' logins: %w", err)
	}
	defer rows.Close()
	seats := map[ids.UUID]bool{}
	var addresses []string
	for rows.Next() {
		var seat ids.UUID
		var login string
		if err := rows.Scan(&seat, &login); err != nil {
			return MemberAddresses{}, fmt.Errorf("capture: reading the members' logins: %w", err)
		}
		seats[seat] = true
		addresses = append(addresses, login)
	}
	if err := rows.Err(); err != nil {
		return MemberAddresses{}, fmt.Errorf("capture: reading the members' logins: %w", err)
	}
	proved, err := provedAddressesTx(ctx, tx)
	if err != nil {
		return MemberAddresses{}, err
	}
	for _, mailbox := range proved {
		if seats[mailbox.Seat] {
			addresses = append(addresses, mailbox.Address)
		}
	}
	own, err := TrustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return MemberAddresses{}, err
	}
	// The company's own domains, subdomains included: a shared mailbox, a list
	// or a colleague without a seat there is still internal mail.
	return MemberAddresses{known: NewSelfSet(addresses, own.Domains())}, nil
}

// CoverEveryParty answers whether every party to a message is a member: the
// one test behind the members_only hold, asked at capture and by the repair
// of stored mail. A message naming nobody is not members-only, for the reason
// internalOnlyTx gives: an empty list means the parties could not be read.
func (m MemberAddresses) CoverEveryParty(parties []string) bool {
	named := false
	for _, party := range parties {
		if strings.TrimSpace(party) == "" {
			continue
		}
		if !m.known.Covers(party) {
			return false
		}
		named = true
	}
	return named
}
