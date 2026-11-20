// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package introductions

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Object is the grant every introduction read and write asks for. Table holds
// every introduction, asked or recorded.
const (
	Object = "introduction"
	Table  = "intro_request"
)

// ReadableSQL is the predicate over intro_request row alias for the rows the
// caller may read. An ask is its two colleagues' alone. A recorded
// introduction is anyone's who may open its contact. load holds one row it
// already read to the same rule. alias is a compile-time literal.
func ReadableSQL(ctx context.Context, alias string, arg func(any) int) (string, error) {
	row := pgx.Identifier{alias}.Sanitize()
	readable := "false"
	// Without the contact grant no recorded introduction is readable, and the
	// asks the caller is party to still are.
	if auth.ReadGranted(ctx, "contact") {
		contact, err := auth.ScopeClauseFor(ctx, "contact", "c", arg)
		if err != nil {
			return "", err
		}
		if contact == "" {
			contact = "true"
		}
		readable = fmt.Sprintf(`(%[1]s.origin = '%[2]s' AND EXISTS (SELECT 1 FROM contact c WHERE c.id = %[1]s.contact_id AND %[3]s))`, row, OriginRecorded, contact)
	}
	if actor, ok := principal.Actor(ctx); ok && actor.HumanOrItsAgent() && !actor.UserID.IsZero() {
		seat := fmt.Sprintf("$%d", arg(actor.UserID))
		readable = fmt.Sprintf(`(%[1]s.requester_user_id = %[2]s OR %[1]s.introducer_user_id = %[2]s OR %[3]s)`, row, seat, readable)
	}
	return "(" + row + ".archived_at IS NULL AND " + readable + ")", nil
}
