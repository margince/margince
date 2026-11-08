// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Subject lines for a SET of activities, in one query.
//
// The attention feed names every subject on the page. Read one at a time that
// is a round trip per card; read together it is one query over the page.
//
// An activity is gated twice and this read carries both, because a subject
// line is CONTENT: the discover clause decides which rows the caller may know
// about at all, and the audience arm decides, per row, whether what was said
// comes along. A row the caller may see but not read answers no label — the
// same withholding readActivity performs when it nulls Subject on a withheld
// row — rather than a name from a message that is none of their business.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivityLabels answers each named activity's subject line, under the
// caller's own grants. A message with no subject, and one whose content is
// withheld from this caller, are both absent.
func (s *Store) ActivityLabels(ctx context.Context, want []ids.UUID) (map[ids.UUID]string, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	labels := map[ids.UUID]string{}
	if len(want) == 0 {
		return labels, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	idsPos := arg(want)
	discover, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if discover == "" {
		discover = scopeUnbounded
	}
	content, err := auth.ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// The subject is selected THROUGH the audience arm rather than
		// filtered by it: a withheld row still answers, with no name, which
		// is the same answer a reader gets for a record that is simply gone.
		// Filtering on it instead would make "withheld" and "absent"
		// indistinguishable here, which they are — deliberately — everywhere
		// else this feed speaks.
		found, err := storekit.LabelsByID(ctx, tx, fmt.Sprintf(`
			SELECT a.id, coalesce(CASE WHEN (%s) THEN a.subject END, '')
			  FROM activity a
			 WHERE a.id = ANY($%d) AND a.archived_at IS NULL AND (%s)`,
			content, idsPos, discover), args...)
		labels = found
		return err
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading activity subjects: %w", err)
	}
	return labels, nil
}
