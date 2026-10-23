// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// calendarSeatMatch is how an attested meeting party's address `inp.address`
// finds its seat `u`: the seat's account address or one of its proven,
// unambiguous identities. An alias a seat only declared itself never binds
// it to another seat's event. Any other record keeps the account-address match.
func calendarSeatMatch(ctx context.Context, tx pgx.Tx, kind string, attested bool, args *[]any) (string, error) {
	if kind != meetingKind || !attested {
		return "lower(u.email) = inp.address", nil
	}
	bind := func(v any) int { *args = append(*args, v); return len(*args) }
	identities, err := storekit.ProvenMailSeatIdentitiesSQL(ctx, tx, bind)
	if err != nil {
		return "", err
	}
	return "(lower(u.email) = inp.address OR (" + identities + " -> u.id::text) ? inp.address)", nil
}
