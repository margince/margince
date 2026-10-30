// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// RequestSettlementActor is the principal the settlement pass writes as. A
// reminder it completed is told apart from one a human completed by this name
// on the audit row.
const RequestSettlementActor = "system:request_settlement"

// settlementEvidenceSQL is the request and every answer a verdict may be
// judged over, as sorted id:version pairs. It reads alias a with $1 as the
// bound. Every UPDATE on activity bumps version, so a body edit, an audience
// change or a narrowing changes the value. A narrowed answer leaves the set.
// The request sits inside the `IN` list: an `OR` beside it scans every row.
func settlementEvidenceSQL() string {
	return `SELECT string_agg(seen.id::text || ':' || seen.version, ',' ORDER BY seen.id)
   FROM activity seen
  WHERE seen.id IN (SELECT a.id UNION ALL SELECT answered.id FROM (` + settlementAnswersSQL() + `) answered)`
}

// reArmedSettlementSQL is a settled request whose verdict recorded the
// evidence it read. The watermark arm beside it offers the request again once
// a newer answer exists. An answer that landed during the verdict is judged,
// not lost. A verdict that recorded no evidence is
// judged again only once the reviewed repair asks for it.
var reArmedSettlementSQL = `a.kind IN ('email', 'message') AND a.direction = 'inbound'
 AND a.archived_at IS NULL AND a.restricted_at IS NULL AND ` + notDismissedSQL + `
 AND (` + confirmedRequestSQL + `)
 AND EXISTS (SELECT 1 FROM activity_request_settlement rearmed
   WHERE rearmed.request_activity_id = a.id AND rearmed.verdict = '` + RequestSettled + `'
     AND (rearmed.judged_evidence IS NOT NULL OR rearmed.revalidation_requested_at IS NOT NULL))`

// settlementCompletedTaskSQL is a reminder the settlement pass completed and
// no human ever wrote to. Only such a reminder may be reopened by a later
// verdict; human-completed or human-edited work is never reopened.
func settlementCompletedTaskSQL(alias string) string {
	return storekit.SQLf(`(%[1]s.is_done AND %[1]s.archived_at IS NULL
 AND EXISTS (SELECT 1 FROM audit_log completed WHERE completed.entity_type = 'activity'
   AND completed.entity_id = %[1]s.id AND completed.actor_type = 'system'
   AND completed.actor_id = '`+RequestSettlementActor+`' AND completed.after->>'is_done' = 'true')
 AND NOT %s)`, alias, storekit.HumanWroteActivityClause(alias))
}

// judgedThroughNewest is whether the verdict's answer is still the request's
// newest one the model may be shown. It reads under the request's row lock.
// An answer that landed or was narrowed during the model call makes the
// verdict stale. The verdict is dropped, and a later pass reads the request.
func (s *Store) judgedThroughNewest(ctx context.Context, tx pgx.Tx, request RepliedRequest) (bool, error) {
	bound := request.AsOf
	if now := s.now(); now.After(bound) {
		bound = now
	}
	var newest *ids.UUID
	if err := tx.QueryRow(ctx, `SELECT (`+newestAnswerSince+`) FROM activity a WHERE a.id = $2`,
		bound, request.RequestID).Scan(&newest); err != nil {
		return false, fmt.Errorf("activities: re-reading the newest answer before settling: %w", err)
	}
	return newest != nil && *newest == request.NewestAnswerID, nil
}

// evidenceUnchanged is whether the request and its answers are the ones the
// verdict read. It reads under holdSettlementEvidenceTx, so the value holds.
func evidenceUnchanged(ctx context.Context, tx pgx.Tx, request RepliedRequest) (bool, error) {
	var now *string
	if err := tx.QueryRow(ctx, `SELECT (`+settlementEvidenceSQL()+`) FROM activity a WHERE a.id = $2`,
		request.AsOf, request.RequestID).Scan(&now); err != nil {
		return false, fmt.Errorf("activities: re-reading the evidence before settling: %w", err)
	}
	return now != nil && *now == request.Evidence, nil
}

// reopenSettledReminder returns a reminder the pass itself completed to work
// when a later verdict says something is still owed. An unsure verdict is not
// evidence of that and reopens nothing. It does so only while
// the request is current at its original age; an older one stays done and the
// verdict stays as record information.
func reopenSettledReminder(ctx context.Context, tx pgx.Tx, in RequestSettlementInput, asOf time.Time) error {
	if in.Verdict != RequestStillOwed || in.Request.TaskID != nil || in.Request.DoneTaskID == nil {
		return nil
	}
	cutoff, err := requestReminderCutoff(ctx, tx, asOf)
	if err != nil {
		return err
	}
	var current bool
	if err := tx.QueryRow(ctx, `SELECT `+occurredday.FromSQL("a", "$2")+` FROM activity a WHERE a.id = $1`,
		in.Request.RequestID, cutoff).Scan(&current); err != nil {
		return fmt.Errorf("activities: reading the request's age before reopening: %w", err)
	}
	if !current {
		return nil
	}
	open, version := false, in.Request.DoneTaskVersion
	_, err = updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](*in.Request.DoneTaskID), UpdateActivityInput{IsDone: &open, IfVersion: &version})
	if errors.Is(err, apperrors.ErrVersionSkew) || errors.Is(err, apperrors.ErrNotFound) {
		return nil
	}
	return err
}
