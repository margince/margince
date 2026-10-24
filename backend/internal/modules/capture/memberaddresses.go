// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// MemberAddresses is every address the workspace knows belongs to one of its
// members, read across all seats. It only ever narrows an audience and never
// reaches a reader, which is why a seat's unproved declaration may count here
// while it may not count as proof of delivery (SeatProvedAddressTx).
//
// Exact addresses only, plus the domains an administrator vouched for. A
// seat's own domain claim is left out: one seat could otherwise hold every
// colleague's mail with a whole outside domain.
type MemberAddresses struct{ known SelfSet }

// MemberAddressesTx reads the set inside the workspace's transaction: each
// seat's login, its declared and discovered addresses, and its live mailbox
// labels. An archived seat no longer counts. An invited or departed one does,
// as in colleagueTx: their mail was still the workspace's mail.
func MemberAddressesTx(ctx context.Context, tx pgx.Tx) (MemberAddresses, error) {
	own, err := TrustedOwnDomainsTx(ctx, tx)
	if err != nil {
		return MemberAddresses{}, err
	}
	rows, err := tx.Query(ctx, `
		SELECT u.email, 'login' FROM app_user u WHERE u.archived_at IS NULL
		 UNION ALL
		SELECT i.value, i.source FROM capture_owner_identity i
		  JOIN app_user u ON u.id = i.user_id
		 WHERE i.kind = 'address' AND u.archived_at IS NULL
		 UNION ALL
		SELECT c.account_label, '' FROM capture_connection c
		  JOIN app_user u ON u.id = c.user_id
		 WHERE c.archived_at IS NULL AND coalesce(c.account_label, '') <> '' AND u.archived_at IS NULL`)
	if err != nil {
		return MemberAddresses{}, fmt.Errorf("capture: reading the members' addresses: %w", err)
	}
	defer rows.Close()
	var addresses []string
	for rows.Next() {
		var value, source string
		if err := rows.Scan(&value, &source); err != nil {
			return MemberAddresses{}, fmt.Errorf("capture: reading the members' addresses: %w", err)
		}
		if discoveredMachineAddress(source, value) {
			continue
		}
		addresses = append(addresses, bareAddress(value))
	}
	if err := rows.Err(); err != nil {
		return MemberAddresses{}, fmt.Errorf("capture: reading the members' addresses: %w", err)
	}
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
