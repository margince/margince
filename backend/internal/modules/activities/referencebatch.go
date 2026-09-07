// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ReferencesByID names each of these activities for a caller who holds a
// derived number and wants to see what it was computed from.
//
// TWO gates, because the row and its words are two questions. Discovery admits
// the row at all — an activity nobody may know exists is simply absent, the way
// it is absent from every other list. Content decides whether the SUBJECT
// travels: a reader may be entitled to know that an exchange happened on a day
// without being entitled to read what it said, and that is the case this
// projection exists to render honestly rather than by omission.
//
// It takes the whole set at once, like the email-summary batch beside it: a
// strength score cites a dozen activities and a per-id read would spend a round
// trip on each.
//
// An id that comes back absent is one this caller cannot discover. The caller
// keeps its own count from the id array it started with — a number that shrank
// to what one reader may open would tell two readers different things about one
// score.
func ReferencesByID(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.ActivityReference, error) {
	none := map[ids.UUID]crmcontracts.ActivityReference{}
	if len(activityIDs) == 0 {
		return none, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// A seat with no activity grant names no activities, and the surface
		// still renders its number. Same silent narrowing the summary batch
		// gives a denied branch.
		return none, nil
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }

	discover, err := auth.ActivityDiscoverClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	// The content question, asked as a COLUMN rather than as a filter. As a
	// filter it would drop the row, and a reader who may know the exchange
	// happened would see a shorter list with nothing saying why.
	content, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if content == "" {
		content = scopeUnbounded
	}
	where := []string{
		fmt.Sprintf("a.id = ANY($%d)", arg(activityIDs)),
		activityLive,
		discover,
	}

	sql := fmt.Sprintf(`
		SELECT a.id, a.kind, a.occurred_at, (%s) AS readable,
		       CASE WHEN (%s) THEN a.subject ELSE NULL END
		  FROM activity a
		 WHERE %s`, content, content, strings.Join(where, " AND "))
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: naming the activities behind a score: %w", err)
	}
	defer rows.Close()

	out := make(map[ids.UUID]crmcontracts.ActivityReference, len(activityIDs))
	for rows.Next() {
		var id ids.UUID
		var readable bool
		var ref crmcontracts.ActivityReference
		if err := rows.Scan(&id, &ref.Kind, &ref.OccurredAt, &readable, &ref.Subject); err != nil {
			return nil, fmt.Errorf("activities: scanning an activity reference: %w", err)
		}
		ref.ActivityId = openapi_types.UUID(id)
		ref.ContentState = crmcontracts.ActivityReferenceContentStateWithheld
		if readable {
			ref.ContentState = crmcontracts.ActivityReferenceContentStateAvailable
		}
		out[id] = ref
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: naming the activities behind a score: %w", err)
	}
	return out, nil
}
