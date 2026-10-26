// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ConnectorSeatSQL is the seat, as text, that the connector stamp
// `connector:<provider>:<seat>` on row alias names, and NULL for any other
// stamp. A bare `connector:<provider>` names no mailbox, so it proves no owner,
// however few connections the provider has.
func ConnectorSeatSQL(alias string) string {
	column := pgx.Identifier{alias, "captured_by"}.Sanitize()
	return fmt.Sprintf(`(CASE WHEN split_part(%[1]s, ':', 1) = 'connector' THEN nullif(split_part(%[1]s, ':', 3), '') END)`, column)
}

// ParticipantStandsSQL holds when participant row participant of activity act
// may stand for its seat being on the message. One row does not stand: it names
// only a seat, on mail whose bare connector stamp names no mailbox. The
// exception is a connector that created the message acting for that seat.
// Otherwise the row records an inferred mailbox owner and grants nothing.
// The SQL carries no '%' because callers splice it into format strings.
func ParticipantStandsSQL(participant, act string) string {
	p := pgx.Identifier{participant}.Sanitize()
	a := pgx.Identifier{act}.Sanitize()
	return fmt.Sprintf(`NOT (%[1]s.address IS NULL AND %[1]s.contact_id IS NULL AND %[1]s.channel_user_id IS NULL
	   AND split_part(%[2]s.captured_by, ':', 1) = 'connector' AND %[3]s IS NULL
	   AND NOT EXISTS (SELECT 1 FROM audit_log made
	        WHERE made.entity_type = 'activity' AND made.entity_id = %[2]s.id AND made.action = 'create'
	          AND made.actor_type = '%[4]s' AND made.on_behalf_of = %[1]s.user_id))`,
		p, a, ConnectorSeatSQL(act), principal.PrincipalConnector)
}
