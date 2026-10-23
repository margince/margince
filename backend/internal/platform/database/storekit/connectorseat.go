// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"

	"github.com/jackc/pgx/v5"
)

// ConnectorSeatSQL is the seat, as text, that the connector stamp
// `connector:<provider>:<seat>` on row alias names, and NULL for any other
// stamp. A bare `connector:<provider>` names no mailbox, so it proves no owner,
// however few connections the provider has.
func ConnectorSeatSQL(alias string) string {
	column := pgx.Identifier{alias, "captured_by"}.Sanitize()
	return fmt.Sprintf(`(CASE WHEN split_part(%[1]s, ':', 1) = 'connector' THEN nullif(split_part(%[1]s, ':', 3), '') END)`, column)
}
