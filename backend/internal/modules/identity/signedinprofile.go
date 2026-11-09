// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// readSeatProfile fills what /me carries beyond who the member is, for an Identity a sign-in
// builds without a session read. It reads the columns Authenticate's session read does, so
// the answer to a sign-in and the next /me draw the same seat.
func readSeatProfile(ctx context.Context, tx pgx.Tx, id *Identity) error {
	var locale, zone *string
	if err := tx.QueryRow(ctx,
		`SELECT locale, timezone, timezone_chosen, timezone_set_by, greeting_name, avatar_object_key
		   FROM app_user WHERE id = $1`, id.UserID).
		Scan(&locale, &zone, &id.TimezoneChosen, &id.TimezoneSetBy, &id.GreetingName, &id.AvatarKey); err != nil {
		return fmt.Errorf("identity: reading the signed-in seat's profile: %w", err)
	}
	id.Locale, id.Timezone = optionalValue(locale), optionalValue(zone)
	return nil
}
