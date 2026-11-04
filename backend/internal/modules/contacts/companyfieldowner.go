// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Who last answered one company field, asked once.
//
// Two columns arbitrate the same way — the description and the logo — and both
// carried their own copy of this read. Two spellings of "a user owns this
// field" is two answers to one question in a product that shows the answer
// beside the field.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// companyFieldHeldByAUser reports whether the last writer of one company field
// was a user or their agent, read from the layer the provenance display reads.
//
// No provenance row means nobody has claimed the field, and an automated writer
// may replace what another automated writer put there. Every user's writer
// stamps, so the absence is informative rather than merely unknown.
func companyFieldHeldByAUser(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID, field string) (bool, error) {
	var held bool
	err := tx.QueryRow(ctx, `
		SELECT `+storekit.StampForAUserSQL("captured_by")+`
		FROM field_provenance
		WHERE object_type = 'company' AND object_id = $1 AND field_name = $2
		ORDER BY captured_at DESC, id DESC
		LIMIT 1`, companyID, field).Scan(&held)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read company %s provenance: %w", field, err)
	}
	return held, nil
}
