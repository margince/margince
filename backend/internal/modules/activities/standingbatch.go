// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// StandingActivities answers which of these activities still stand for THIS
// caller: live, and within the caller's own discover scope. Ids that are
// archived, outside that scope, or gone are simply absent from the set.
//
// It exists for advice that was STORED and is replayed later. A suggestion
// written days ago cites records that can be archived afterwards, and a
// finding whose record is gone is a card quoting something the reader can no
// longer open. Answering that needs a question about the ROW — is it still
// there — and nothing about its content.
//
// The gate is auth.ActivityDiscoverClause, deliberately, and this is the whole
// reason the probe is separate from EmailSummariesByIDBatch. That reader is
// narrower twice over: it admits only `kind = 'email'`, and it gates on
// ActivityContentClause because it prints a subject and a body preview. A
// caller asking "does this row still stand" through it would read a live
// MEETING as gone, and a live email whose CONTENT this reader may not see as
// gone too — retracting advice that is perfectly valid. Existence is the
// discover question, so it takes the discover gate.
func StandingActivities(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	return citedActivitiesAdmittedBy(
		ctx, tx, activityIDs, auth.ActivityDiscoverClause, "which cited activities still stand")
}

// ReadableActivityContent answers which of these activities THIS caller may
// read the WORDS of: live, and within the caller's own content scope. Ids that
// are archived, outside that scope, or gone are simply absent from the set.
//
// The third of the batch probes over one cited list, and the three differ only
// in which question they ask. StandingActivities asks whether the ROW still
// stands, under the discover gate. EmailSummariesByIDBatch asks for the email
// row itself, under the content gate and only for `kind = 'email'`. This asks
// the content question alone, of any kind.
//
// That last difference is the whole reason it exists. Advice that was STORED
// quotes a message verbatim, and an audience narrowed after the write leaves
// those words readable on every replay. The summary reader cannot answer for
// them: its silence means "not an email, or not yours", so blanking on it would
// also blank a meeting's own title from a reader entitled to it.
func ReadableActivityContent(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
) (map[ids.UUID]bool, error) {
	return citedActivitiesAdmittedBy(
		ctx, tx, activityIDs, auth.ActivityContentClause, "whose words a citation may still quote")
}

// activityScopeClause is the shape both gates share: render this caller's row
// predicate over the activity aliased `a`, or empty for an unbounded one.
type activityScopeClause func(context.Context, string, func(any) int) (string, error)

// citedActivitiesAdmittedBy is the ONE walk behind the probes above: the same
// id set, the same liveness term, the same silent narrowing — and one gate,
// supplied by the caller, which is the entire difference between the questions
// they ask.
//
// Shared rather than spelled twice because two copies of a scoped read drift in
// the half nobody is looking at: the liveness term, or the denied-permission
// branch. `asking` names the question in the caller's own words, so a failure
// says which probe could not answer rather than naming this helper.
//
// Never an error for an id the caller may not reach: the same rule
// EmailSummariesByIDBatch follows, because a page enriching or rechecking many
// rows must not fail over one the reader was never entitled to.
func citedActivitiesAdmittedBy(
	ctx context.Context, tx pgx.Tx, activityIDs []ids.UUID,
	scopeFor activityScopeClause, asking string,
) (map[ids.UUID]bool, error) {
	admitted := map[ids.UUID]bool{}
	if len(activityIDs) == 0 {
		return admitted, nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		// A seat with no activity.read reaches no activity, and the caller's
		// page still renders. Same silent narrowing the summary batch gives a
		// denied branch.
		return admitted, nil
	}
	args := []any{}
	arg := func(v any) int { args = append(args, v); return len(args) }

	scope, err := scopeFor(ctx, "a", arg)
	if err != nil {
		return nil, err
	}
	where := []string{fmt.Sprintf("a.id = ANY($%d)", arg(activityIDs)), activityLive}
	if scope != "" {
		where = append(where, scope)
	}

	rows, err := tx.Query(ctx,
		"SELECT a.id FROM activity a WHERE "+strings.Join(where, " AND "), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading %s: %w", asking, err)
	}
	defer rows.Close()
	for rows.Next() {
		var id ids.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("activities: scanning a row for %s: %w", asking, err)
		}
		admitted[id] = true
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("activities: reading %s: %w", asking, err)
	}
	return admitted, nil
}
