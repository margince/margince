// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// A function rather than a var: the created-from projection is gated by the
// reader's own activity audience, which needs the context and a placeholder.
// The seat-name subselect was already built by a function.
//
// The projection is named, because a bare subselect of `src.id` reaches the
// caller as a column called `id`, and the list's `ORDER BY "id"` is then
// ambiguous against the contact's own.
//
// The source reads null to a reader who may not open that record, rather than
// refusing the contact. A bare id would say a record exists that the reader
// cannot reach, which is the disclosure the audience exists to prevent.
func contactColumns(ctx context.Context, arg func(any) int) (string, error) {
	source, err := auth.ActivityContentClause(ctx, "src", arg)
	if err != nil {
		return "", err
	}
	return `id, full_name, first_name, last_name, title, owner_id, visibility,
	address_line1, address_line2, address_city, address_region, address_postal_code, address_country,
	` + addressCountriesSQL("contact") + `,
	merged_into_id, converted_from_lead_id, source, captured_by,
	source_system, source_author_id, source_author_name,
	` + sourceAuthorSeatNameSQL("contact") + `,
	version, created_at, updated_at, archived_at, last_activity_at, legal_hold, relationship_mark,
	(SELECT src.id FROM activity src
	  WHERE src.id = contact.created_from_activity_id
	    AND src.archived_at IS NULL AND ` + source + `)
	  AS created_from_activity_id`, nil
}
