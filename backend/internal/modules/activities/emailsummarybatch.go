// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EmailSummariesByID is EmailSummariesByIDBatch for a caller with no
// transaction of its own to lend — the who-is-waiting seam, whose rows come
// back from a read that has already committed. Same reader, same gate; only
// the transaction differs.
func (s *Store) EmailSummariesByID(
	ctx context.Context, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	var out map[ids.UUID]crmcontracts.EmailSummary
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = EmailSummariesByIDBatch(ctx, tx, activityIDs)
		return readErr
	})
	return out, err
}

// EmailSummariesByIDBatch answers the email row of each of these activities,
// for THIS caller, in the caller's own transaction. Ids that are not emails,
// that the caller may not read, or that do not exist are simply absent from
// the map — an absent id is one this caller gets no summary for, never an
// error, because a batch enriching a page must not fail the page over one row
// the reader was never entitled to.
//
// It takes the whole page at once. A per-id read cost a round trip per hit,
// which a search page of 200 turns into 200 statements.
//
// The gate is auth.ActivityContentClause, not the timeline's discover clause.
// A summary carries the subject and a body preview, so admitting a row here
// is admitting its content: the discover gate would answer "you may know this
// row exists", and then this function would print what it says.
func EmailSummariesByIDBatch(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	// An empty map rather than a nil one on both early exits: a caller reads
	// this by key, and "no rows for you" and "no rows asked for" are the same
	// answer to that question. Returning nil would make each call site decide
	// again whether a nil map is safe to index.
	none := map[ids.UUID]crmcontracts.EmailSummary{}
	if len(activityIDs) == 0 {
		return none, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// A seat with no activity.read reads no email rows, and a page of
		// other records still renders. Same silent narrowing search itself
		// gives a denied branch.
		return none, nil
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }

	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	contentArm, err := auth.ActivityAudienceArm(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	where := []string{
		fmt.Sprintf("a.id = ANY($%d)", arg(activityIDs)),
		"a.kind = 'email'",
		activityLive,
	}
	if scope != "" {
		where = append(where, scope)
	}

	sql := "SELECT " + activityColumns(contentArm) + " FROM activity a WHERE " +
		strings.Join(where, " AND ")
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading email summaries for a page: %w", err)
	}
	defer rows.Close()

	admitted := make([]crmcontracts.Activity, 0, len(activityIDs))
	for rows.Next() {
		var scan activityScan
		if err := rows.Scan(activityScanTargets(&scan)...); err != nil {
			return nil, fmt.Errorf("activities: scanning an email summary: %w", err)
		}
		admitted = append(admitted, scan.record())
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading email summaries for a page: %w", err)
	}
	// Collected first: the count runs a second statement on this transaction,
	// which needs the cursor above already closed.
	rows.Close()
	if err := WithAttachmentCounts(ctx, tx, admitted); err != nil {
		return nil, err
	}

	out := make(map[ids.UUID]crmcontracts.EmailSummary, len(activityIDs))
	for _, activity := range admitted {
		// RowEmailSummary rather than a second assembler: the row a search hit
		// shows and the row a timeline shows are the same row, and two
		// projections of it would drift the moment either grew a field.
		// The summary the page carries is the row RECORD() already built,
		// counts and all — not a second call to RowEmailSummary, which would
		// rebuild it from the activity and drop the count just applied.
		if activity.EmailSummary != nil {
			out[ids.UUID(activity.Id)] = *activity.EmailSummary
		}
	}
	return out, nil
}
