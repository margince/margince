// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"github.com/jackc/pgx/v5"
)

// IntentInstantSQL resolves a chosen clock using current database timezone rules.
func IntentInstantSQL(alias, field string) string {
	column := func(name string) string { return pgx.Identifier{alias, name}.Sanitize() }
	return fmt.Sprintf("COALESCE(public.resolve_intent_time(%s,%s),%s)", column(field+"_wall"), column(field+"_zone"), column(field+"_at"))
}

// IntentWireSQL leaves older instants without a guessed wall clock or zone.
func IntentWireSQL(alias, field string) string {
	wall, zone := pgx.Identifier{alias, field + "_wall"}.Sanitize(), pgx.Identifier{alias, field + "_zone"}.Sanitize()
	return fmt.Sprintf(`CASE WHEN %s IS NOT NULL THEN jsonb_build_object('wall_clock',to_char(%s,'YYYY-MM-DD"T"HH24:MI:SS.US'),'timezone',%s) END`, wall, wall, zone)
}
