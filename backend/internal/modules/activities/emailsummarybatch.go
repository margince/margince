// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EmailSummariesByID reads for a caller without a transaction, including brief
// evidence. Its store clock supplies the instant for request-time readers.
func (s *Store) EmailSummariesByID(
	ctx context.Context, activityIDs []ids.UUID,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	return s.EmailSummariesByIDAt(ctx, activityIDs, s.now())
}

// EmailSummariesByIDAt judges each row at the caller's read instant.
func (s *Store) EmailSummariesByIDAt(
	ctx context.Context, activityIDs []ids.UUID, asOf time.Time,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	var out map[ids.UUID]crmcontracts.EmailSummary
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = EmailSummariesByIDBatch(ctx, tx, activityIDs, asOf)
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
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, asOf time.Time,
) (map[ids.UUID]crmcontracts.EmailSummary, error) {
	// An empty map rather than a nil one on both early exits: a caller reads
	// this by key, and "no rows for you" and "no rows asked for" are the same
	// answer to that question. Returning nil would make each call site decide
	// again whether a nil map is safe to index.
	none := map[ids.UUID]crmcontracts.EmailSummary{}
	admitted, err := readAdmittedByID(ctx, tx, activityIDs, asOf, "a.kind = 'email'")
	if err != nil {
		return nil, err
	}
	if admitted == nil {
		return none, nil
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

// VoicesByIDBatch answers who wrote each of these activities, for THIS caller.
// A row the caller may not read, that is gone, or that records no author is
// absent: who wrote a row is part of its content. Brief inputs read it so a
// quoted line is credited to its writer and not to the page's contact.
func VoicesByIDBatch(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, asOf time.Time,
) (map[ids.UUID]*crmcontracts.ActivityVoice, error) {
	out := map[ids.UUID]*crmcontracts.ActivityVoice{}
	admitted, err := readAdmittedByID(ctx, tx, activityIDs, asOf, sqlTrue)
	if err != nil {
		return nil, err
	}
	for _, activity := range admitted {
		if activity.Voice != nil && activity.Voice.Author != nil {
			out[ids.UUID(activity.Id)] = activity.Voice
		}
	}
	return out, nil
}

// readAdmittedByID reads the rows of these ids that the caller may read in
// full, with their mail facts and voices filled. It returns nil when there is
// nothing to read or the seat has no activity.read: a batch enriching a page
// must not fail the page over a row the reader was never entitled to.
func readAdmittedByID(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID, asOf time.Time, kindClause string,
) ([]crmcontracts.Activity, error) {
	if len(activityIDs) == 0 {
		return nil, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// Same silent narrowing search itself gives a denied branch.
		return nil, nil
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }

	scope, err := auth.ActivityContentClause(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = sqlTrue
	}
	where := []string{
		fmt.Sprintf("a.id = ANY($%d)", arg(activityIDs)),
		kindClause,
		activityLive,
		scope,
	}
	// The content gate holds the audience arm, so every row it admits has its
	// content available. A second copy would only cost planning.
	from, contentArm := ActivityContentFrom("activity a", sqlTrue)

	columns, err := activityColumns(ctx, contentArm, arg)
	if err != nil {
		return nil, err
	}
	sql := "SELECT " + columns + " FROM " + from + " WHERE " +
		strings.Join(where, " AND ")
	rows, err := tx.Query(ctx, sql, args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading rows by id for a page: %w", err)
	}
	defer rows.Close()

	admitted := make([]crmcontracts.Activity, 0, len(activityIDs))
	for rows.Next() {
		var scan activityScan
		if err := rows.Scan(activityScanTargets(&scan)...); err != nil {
			return nil, fmt.Errorf("activities: scanning a row by id: %w", err)
		}
		admitted = append(admitted, scan.record(ctx))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading rows by id for a page: %w", err)
	}
	// Collected first: the facts run further statements on this transaction,
	// which needs the cursor above already closed.
	rows.Close()
	if err := WithEmailRowFacts(ctx, tx, admitted, asOf); err != nil {
		return nil, err
	}
	return admitted, nil
}
