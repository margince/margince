// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/mailidentity"
	"github.com/margince/margince/backend/internal/platform/mailrole"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// ProvedMailboxLabels reads labels attested by a connected OAuth provider, never typed IMAP labels.
//
// The seat has to still be one. A departed member's connection stays connected
// until somebody takes it down. The label on it would otherwise go on
// attributing imports to a mailbox nobody holds.
func ProvedMailboxLabels(ctx context.Context, tx pgx.Tx) (map[ids.UUID][]string, error) {
	records, err := readMailSeatIdentities(ctx, tx)
	if err != nil {
		return nil, err
	}
	out := map[ids.UUID][]string{}
	for _, record := range records {
		if !record.provedMailbox {
			continue
		}
		seat, err := ids.Parse(record.seat)
		if err != nil {
			return nil, fmt.Errorf("reading proved mailbox seat: %w", err)
		}
		out[seat] = append(out[seat], record.label)
	}
	return out, nil
}

func provedMailboxClause(alias string) string {
	alias = pgx.Identifier{alias}.Sanitize()
	return fmt.Sprintf(`(%[1]s.archived_at IS NULL AND coalesce(%[1]s.account_label,'') <> ''
 AND %[1]s.status='connected' AND %[1]s.provider IN ('gmail','gcal','graph','graphcal'))`, alias)
}

// ProvedMailSeatIdentitiesSQL binds each seat's provider-attested mailbox
// addresses as a jsonb map from seat id to addresses. It reads
// ProvedMailboxLabels, the one set SeatProvedAddressTx trusts, so a declared
// alias or a delivered-to sighting proves nothing here either. An address two
// seats proved belongs to neither.
func ProvedMailSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	return provedSeatIdentitiesSQL(ctx, tx, bind, false)
}

// ProvedPersonalSeatIdentitiesSQL is ProvedMailSeatIdentitiesSQL without role
// mailboxes (sales@, info@, a helpdesk queue). A calendar roster binds a guest
// to a seat through it. An answer given for a role mailbox a member connected
// credits that member with nothing, because only their own answer proves they
// attended. A shared mailbox has no seat to bind.
func ProvedPersonalSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int) (string, error) {
	return provedSeatIdentitiesSQL(ctx, tx, bind, true)
}

func provedSeatIdentitiesSQL(ctx context.Context, tx pgx.Tx, bind func(any) int, withoutRoles bool) (string, error) {
	labels, err := ProvedMailboxLabels(ctx, tx)
	if err != nil {
		return "", err
	}
	holders := map[string]map[ids.UUID]bool{}
	for seat, values := range labels {
		for _, label := range values {
			if _, role := mailrole.Match(label); withoutRoles && role {
				continue
			}
			// Keyed by the folded address, so two seats proving one mailbox in two
			// spellings collide, as they do in UnambiguousMailSeatIdentities.
			address := mailidentity.SeatAddressKey(label, "")
			if address == "" {
				continue
			}
			if holders[address] == nil {
				holders[address] = map[ids.UUID]bool{}
			}
			holders[address][seat] = true
		}
	}
	out := map[string][]string{}
	for address, seats := range holders {
		if len(seats) != 1 {
			continue
		}
		for seat := range seats {
			out[seat.String()] = append(out[seat.String()], mailidentity.AddressSpellings(address)...)
		}
	}
	encoded, err := json.Marshal(out)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("$%d::jsonb", bind(encoded)), nil
}
