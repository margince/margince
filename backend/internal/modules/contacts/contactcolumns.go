// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/margince/margince/backend/internal/platform/auth"
)

// A function rather than a var: the created-from projection is gated by the
// reader's own activity grant and audience. Both need the context and a
// placeholder. The projection is named: a bare `src.id` subselect reaches the
// caller as `id`. The list's `ORDER BY "id"` is then ambiguous.
func contactColumns(ctx context.Context, arg func(any) int) (string, error) {
	source, err := createdFromSQL(ctx, arg)
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
	` + source + ` AS created_from_activity_id`, nil
}

// createdFromSQL is the source column a reader may see. It reads null to a
// reader without the activity read grant or outside the record's audience,
// rather than refusing the contact. An id would say the record exists.
func createdFromSQL(ctx context.Context, arg func(any) int) (string, error) {
	if !auth.ReadGranted(ctx, "activity") {
		return `NULL::uuid`, nil
	}
	audience, err := auth.ActivityContentClause(ctx, "src", arg)
	if err != nil {
		return "", err
	}
	return `(SELECT src.id FROM activity src
	  WHERE src.id = contact.created_from_activity_id
	    AND src.archived_at IS NULL AND ` + audience + `)`, nil
}
