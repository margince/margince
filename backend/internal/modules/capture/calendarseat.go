// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// calendarSeatMatch is how an attested meeting party's address `inp.address`
// finds its seat `u`: the seat's account address or a mailbox the provider
// attested for it (storekit.ProvedMailSeatIdentitiesSQL). A declared alias or
// a delivered-to sighting never binds a seat to an event. Any other record
// keeps the account-address match.
func calendarSeatMatch(ctx context.Context, tx pgx.Tx, kind string, attested bool, args *[]any) (string, error) {
	// An erased seat is nobody to bind. Both callers join app_user on this
	// fragment and add no liveness test, so an archived account's address wrote
	// an attendance row naming it.
	//
	// It asks only whether the account is erased. A seat that merely left still
	// attended the meeting.
	// Factored out front rather than written into one arm.
	//
	// `AND` binds tighter than `OR`, so a liveness test beside the login match
	// leaves the attested arm unguarded.
	alive := "u.archived_at IS NULL AND "
	login := storekit.SeatLoginIsSQL("u", "inp.address", "inp.domain_folded")
	if kind != meetingKind || !attested {
		return alive + login, nil
	}
	bind := func(v any) int { *args = append(*args, v); return len(*args) }
	identities, err := storekit.ProvedMailSeatIdentitiesSQL(ctx, tx, bind)
	if err != nil {
		return "", err
	}
	return alive + "(" + login +
		" OR (" + identities + " -> u.id::text) ? inp.address)", nil
}
