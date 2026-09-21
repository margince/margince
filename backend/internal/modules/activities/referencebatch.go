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

	// The content clause is evaluated ONCE, in a subquery, and read twice from
	// there. Repeating its text would work — a $N placeholder may be referenced
	// as often as you like — but it would also mean the two copies could drift
	// apart under an edit, and the row's `content_state` would then disagree
	// with whether its own subject was blanked.
	sql := fmt.Sprintf(`
		SELECT id, kind, occurred_at, readable,
		       CASE WHEN readable THEN subject ELSE NULL END
		  FROM (
		    SELECT a.id, a.kind, a.occurred_at, a.subject, (%s) AS readable
		      FROM activity a
		     WHERE %s
		  ) admitted`, content, strings.Join(where, " AND "))
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
	// The canonical email row for the rows that are emails, from the reader
	// beside this one. Its own gate is the CONTENT gate, so it answers for
	// exactly the messages whose subject travelled above — a summary it
	// withholds is one this projection already marked withheld.
	//
	// Rows must be closed first: this runs a second statement on the same
	// transaction.
	rows.Close()
	if err := attachEmailSummaries(ctx, tx, out); err != nil {
		return nil, err
	}
	return out, nil
}

// attachEmailSummaries fills in the canonical row behind every reference that
// is an email, in one further read over the whole set.
//
// Asked only for the emails: a note has no email row, and asking would spend
// the statement on ids that can only come back absent — the same filtering the
// waiting lane does before its own call.
func attachEmailSummaries(
	ctx context.Context, tx pgx.Tx, refs map[ids.UUID]crmcontracts.ActivityReference,
) error {
	var emails []ids.UUID
	for id, ref := range refs {
		if ref.Kind == crmcontracts.ActivityReferenceKindEmail {
			emails = append(emails, id)
		}
	}
	if len(emails) == 0 {
		return nil
	}
	summaries, err := EmailSummariesByIDBatch(ctx, tx, emails)
	if err != nil {
		return err
	}
	for id, summary := range summaries {
		ref, ok := refs[id]
		if !ok {
			continue
		}
		// Copied into a local before its address is taken: the range variable
		// is reused, and every row would otherwise point at the last summary.
		held := summary
		ref.EmailSummary = &held
		refs[id] = ref
	}
	return nil
}
