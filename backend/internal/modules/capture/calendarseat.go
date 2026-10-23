// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

func calendarSeatMatch(ctx context.Context, tx pgx.Tx, kind string, attested bool, args *[]any) (string, error) {
	if kind != meetingKind || !attested {
		return "lower(u.email)=inp.address", nil
	}
	bind := func(v any) int { *args = append(*args, v); return len(*args) }
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, bind)
	if err != nil {
		return "", err
	}
	return "(" + identities + " -> u.id::text) ? inp.address", nil
}
