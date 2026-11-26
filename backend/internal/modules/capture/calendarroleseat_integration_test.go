// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package capture

import (
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// A calendar answer binds its guest to a seat through the seat's own address or
// a personal mailbox the provider attested for it. A role mailbox a member
// connected (sales@, support@) is attested too, and its answer is the team's,
// not that member's, so it binds nobody.
func TestARoleMailboxAnswerOnAMeetingBindsNoSeat(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
		seat, login := seedSeat(ctx, t, tx, "active")
		colleague, _ := seedSeat(ctx, t, tx, "active")
		role := "sales-" + seat.String() + "@example.test"
		alias := "alias-" + colleague.String() + "@example.test"
		for owner, label := range map[ids.UUID]string{seat: role, colleague: alias} {
			if _, err := tx.Exec(ctx, `
				INSERT INTO capture_connection (id, provider, user_id, status, account_label)
				VALUES ($1, 'gcal', $2, 'connected', $3)`, ids.NewV7(), owner, label); err != nil {
				return err
			}
		}
		for _, answer := range []struct {
			name, address string
			seat          ids.UUID
			bound         bool
		}{
			{"the member's own address", login, seat, true},
			{"a personal mailbox the provider attested", alias, colleague, true},
			{"a role mailbox the member connected", role, seat, false},
		} {
			args := []any{answer.address, answer.seat}
			match, err := calendarSeatMatch(ctx, tx, meetingKind, true, &args)
			if err != nil {
				return err
			}
			var bound bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user u CROSS JOIN
 (SELECT $1::text AS address, 'example.test'::text AS domain_folded) inp WHERE u.id = $2 AND `+match+`)`,
				args...).Scan(&bound); err != nil {
				return err
			}
			if bound != answer.bound {
				t.Errorf("%s: bound = %v, want %v", answer.name, bound, answer.bound)
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}
