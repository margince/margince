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

func TestRetiredSeatsLeaveNewMailClassificationButKeepCalendarAttendance(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	for _, removed := range []bool{false, true} {
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			seat := ids.NewV7()
			email := seat.String() + "@seat.example"
			if _, err := tx.Exec(ctx, `INSERT INTO app_user (id,email,display_name,status,removed_at)
 VALUES (@seat,@email,'Colleague','deactivated',CASE WHEN @removed THEN now() END)`,
				pgx.StrictNamedArgs{"seat": seat, "email": email, "removed": removed}); err != nil {
				return err
			}
			members, err := MemberAddressesTx(ctx, tx)
			if err != nil {
				return err
			}
			if members.CoverEveryParty([]string{email}) == removed {
				t.Errorf("removed=%v: wrong member classification", removed)
			}
			recipient, err := recipientColleagueTx(ctx, tx, email, ids.NewV7())
			if err != nil {
				return err
			}
			if recipient.IsZero() != removed {
				t.Errorf("removed=%v: recipient bound to %v", removed, recipient)
			}
			var args []any
			match, err := calendarSeatMatch(ctx, tx, meetingKind, false, &args)
			if err != nil {
				return err
			}
			var bound bool
			if err := tx.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM app_user u CROSS JOIN
 (SELECT @email::text AS address, 'seat.example'::text AS domain_folded) inp WHERE `+match+`)`,
				pgx.StrictNamedArgs{"email": email}).Scan(&bound); err != nil {
				return err
			}
			if !bound {
				t.Errorf("removed=%v: calendar attendance lost", removed)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
