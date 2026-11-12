// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SettledCitations answers whether each cited message's thread settled it by
// asOf, by the Worklist's rules. Their ask: a settled verdict, a done request
// task, or an answer when nobody confirmed the ask. Our promise: the follow-up
// the Worklist offers as "Done?", to one of recipients. Only evidence the
// caller may read settles. Other kinds never settle. An id the caller may not
// read is absent.
func SettledCitations(
	ctx context.Context, tx pgx.Tx, cited []ids.UUID, recipients []ids.ContactID, asOf time.Time,
) (map[ids.UUID]bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return nil, err
	}
	out := make(map[ids.UUID]bool, len(cited))
	if len(cited) == 0 {
		return out, nil
	}
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	citedPos := arg(cited)
	recipientsPos := arg(recipients)
	until := fmt.Sprintf("$%d::timestamptz", arg(asOf))
	// The cited messages, their answers and their follow-ups share one content
	// gate. Evidence the reader may not read settles nothing.
	readable, err := auth.ContentReadableActivitiesSQL(ctx,
		"SELECT id FROM cited UNION ALL SELECT id FROM cited_answer UNION ALL SELECT id FROM follow_up", arg)
	if err != nil {
		return nil, err
	}
	// An unjudged message has a NULL verdict, and unknown never settles.
	rows, err := tx.Query(ctx, fmt.Sprintf(`WITH cited AS MATERIALIZED (
  SELECT a.* FROM activity a WHERE a.id = ANY($%[1]d) AND a.archived_at IS NULL),
cited_answer AS MATERIALIZED (
  SELECT a.id AS cited_id, answer_found.id
    FROM cited a
   CROSS JOIN LATERAL (%[2]s) answer_found
    JOIN activity readable_answer ON readable_answer.id = answer_found.id
   WHERE a.direction = 'inbound' AND a.kind IN ('email', 'message')
     AND readable_answer.restricted_at IS NULL),
follow_up AS MATERIALIZED (
  SELECT a.id AS cited_id, kept.id
    FROM cited a
    JOIN activity kept ON %[3]s
   WHERE a.direction = 'outbound' AND a.kind = 'email'
     AND EXISTS (SELECT 1 FROM activity_participant promised
           WHERE promised.activity_id = a.id AND promised.role IN ('to', 'cc')
             AND promised.contact_id = ANY($%[4]d) AND %[5]s)),
readable_activity AS MATERIALIZED (%[6]s)
SELECT a.id, coalesce(
  CASE WHEN a.direction = 'inbound' AND a.kind IN ('email', 'message')
         THEN NOT (%[7]s AND (coalesce(%[8]s, false) OR NOT EXISTS (
                SELECT 1 FROM cited_answer answer
                 WHERE answer.cited_id = a.id AND answer.id IN (SELECT id FROM readable_activity))))
       WHEN a.direction = 'outbound' AND a.kind = 'email'
         THEN EXISTS (SELECT 1 FROM follow_up
                WHERE follow_up.cited_id = a.id AND follow_up.id IN (SELECT id FROM readable_activity))
       ELSE false END, false)
  FROM cited a
 WHERE a.id IN (SELECT id FROM readable_activity)`,
		citedPos, answerIDsSQL("a", until), promiseFollowUpSQL("kept", "a", until), recipientsPos,
		promiseRecipientAddressedSQL("kept", "promised.contact_id"), readable,
		requestOpenSQL, confirmedRequestSQL), args...)
	if err != nil {
		return nil, fmt.Errorf("activities: reading whether cited threads settled: %w", err)
	}
	var id ids.UUID
	var settled bool
	if _, err := pgx.ForEachRow(rows, []any{&id, &settled}, func() error {
		out[id] = settled
		return nil
	}); err != nil {
		return nil, fmt.Errorf("activities: reading whether cited threads settled: %w", err)
	}
	return out, nil
}
