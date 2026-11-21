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

// On stored mail a colleague is whoever holds a seat that is neither retired
// nor erased: an invited one and a departed one count. A retired seat leaves
// mail classification, and keeps the meetings it attended.
func TestRetiredSeatsLeaveNewMailClassificationButKeepCalendarAttendance(t *testing.T) {
	ctx, pool := meetingTenureEnv(t)
	for _, seatCase := range []struct {
		name, status string
		removed      bool
	}{
		{"an invited colleague", "invited", false},
		{"an active colleague", "active", false},
		{"a departed colleague", "deactivated", false},
		{"a retired seat", "deactivated", true},
	} {
		removed := seatCase.removed
		if err := database.WithWorkspaceTx(ctx, pool, func(tx pgx.Tx) error {
			seat := ids.NewV7()
			email := seat.String() + "@seat.example"
			if _, err := tx.Exec(ctx, `INSERT INTO app_user (id,email,display_name,status,removed_at)
 VALUES (@seat,@email,'Colleague',@status,CASE WHEN @removed THEN now() END)`,
				pgx.StrictNamedArgs{"seat": seat, "email": email, "status": seatCase.status, "removed": removed}); err != nil {
				return err
			}
			members, err := MemberAddressesTx(ctx, tx)
			if err != nil {
				return err
			}
			if members.CoverEveryParty([]string{email}) == removed {
				t.Errorf("%s: wrong member classification", seatCase.name)
			}
			recipient, err := recipientColleagueTx(ctx, tx, email, ids.NewV7())
			if err != nil {
				return err
			}
			if recipient.IsZero() != removed {
				t.Errorf("%s: recipient bound to %v", seatCase.name, recipient)
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
				t.Errorf("%s: calendar attendance lost", seatCase.name)
			}
			return nil
		}); err != nil {
			t.Fatal(err)
		}
	}
}
