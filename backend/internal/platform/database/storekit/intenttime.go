// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// IntentInstantSQL resolves a chosen clock using current database timezone rules.
func IntentInstantSQL(alias, field string) string {
	return IntentInstantColumnSQL(alias, field, field+"_at")
}

func IntentInstantColumnSQL(alias, field, instant string) string {
	column := func(name string) string { return pgx.Identifier{alias, name}.Sanitize() }
	return fmt.Sprintf("COALESCE(public.resolve_intent_time(%s,%s),%s)", column(field+"_wall"), column(field+"_zone"), column(instant))
}

// IntentWireSQL leaves older instants without a guessed wall clock or zone.
func IntentWireSQL(alias, field string) string {
	wall, zone := pgx.Identifier{alias, field + "_wall"}.Sanitize(), pgx.Identifier{alias, field + "_zone"}.Sanitize()
	return fmt.Sprintf(`CASE WHEN %s IS NOT NULL THEN jsonb_build_object('wall_clock',to_char(%s,'YYYY-MM-DD"T"HH24:MI:SS.US'),'timezone',%s) END`, wall, wall, zone)
}

func ResolveIntentTime(field string, wire *crmcontracts.IntentTime) (*time.Time, error) {
	if wire == nil {
		return nil, nil
	}
	zone, err := values.ParseTimezone(wire.Timezone)
	// The name is stored as sent, so a padded spelling is refused here and
	// never reaches the database check.
	if err != nil || zone.String() != wire.Timezone {
		return nil, &values.ParseError{Field: field + ".timezone", Code: "invalid", Message: "choose an IANA timezone"}
	}
	intent, err := values.ParseIntentTime(wire.WallClock, zone)
	if err != nil {
		return nil, &values.ParseError{Field: field + ".wall_clock", Code: "invalid", Message: "choose a valid local date and time"}
	}
	instant, err := intent.Resolve()
	if err != nil {
		return nil, err
	}
	return &instant, nil
}
