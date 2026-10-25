// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/capture"
	"github.com/margince/margince/backend/internal/modules/capture/mailmap"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// mailBodyCompletenessDataTask restores mail bodies an older parser or cap left
// short, from their stored originals, then assesses the rest of the stored mail.
// Two later steps drop labels read from a short body and decode legacy charsets.
// All use the parser capture uses today. Mail without its stored original is
// never guessed at: it stays unassessed, and automatic work does not read it.
func mailBodyCompletenessDataTask() datatask.Task {
	return datatask.Task{
		ID: "core/1792776165_mail_body_completeness", Version: 2,
		Title: "Restore and assess the text of stored mail", Requires: []string{"core/1792776166"},
		Steps: []datatask.Step{{
			Name: "restore_stored_mail_bodies", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateRecoverableMailBodies, Batch: restoreMailBodiesBatch,
		}, {
			Name: "assess_stored_mail_bodies", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{Reason: "capture stamps the same parsed completeness on the row it inserts, without a separate audit"},
			Estimate: estimateUnassessedMailBodies, Batch: assessMailBodiesBatch,
		}, {
			Name: "reset_labels_read_from_short_bodies", Limit: 100, MinLimit: 1,
			Audit:    datatask.AuditPolicy{Reason: "activities.SetCaptureLabel writes and the classify pass rewrites the label without an audit"},
			Estimate: estimateStaleMailLabels, Batch: resetRestoredMailLabelsBatch,
		}, {
			Name: "repair_mail_charsets", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{PerRecord: true},
			Estimate: estimateCharsetDamagedMail, Batch: repairMailCharsetsBatch,
		}},
	}
}

type unassessedMailBody struct {
	ID        ids.UUID
	Body      *string
	Connector bool
	Payload   []byte
}

func assessMailBodiesBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT a.id, a.body, `+capturedByMailConnector+`, rc.payload `+unassessedMailBodyScope+`
		AND a.id > @unassessed_after ORDER BY a.id LIMIT @batch_limit FOR UPDATE OF a`, pgx.NamedArgs{"unassessed_after": after, "batch_limit": in.Limit})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	candidates, err := pgx.CollectRows(rows, pgx.RowToStructByPos[unassessedMailBody])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	scanned := make([]ids.UUID, 0, len(candidates))
	assessments := make([]activities.MailBodyAssessment, 0, len(candidates))
	for _, candidate := range candidates {
		scanned = append(scanned, candidate.ID)
		if completeness, ok := assessStoredMailBody(candidate); ok {
			assessments = append(assessments, activities.MailBodyAssessment{
				Activity: candidate.ID, Completeness: string(completeness), Body: *candidate.Body,
			})
		}
	}
	changed, err := activities.AssessMailBodiesTx(ctx, tx, assessments)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	return datatask.AdvanceUUID(in, scanned, changed)
}

// assessStoredMailBody never guesses. Mail no connector parsed is text a member
// or an import wrote, whole as stored. Captured mail is judged from its stored
// original; without one, or with one that no longer reads, it stays unassessed.
func assessStoredMailBody(candidate unassessedMailBody) (capture.BodyCompleteness, bool) {
	if candidate.Body == nil {
		return "", false
	}
	if !candidate.Connector {
		return capture.BodyComplete, true
	}
	if candidate.Payload == nil {
		return "", false
	}
	raw, readable := decodedMailOriginal(candidate.Payload)
	if !readable {
		return "", false
	}
	return mailmap.AssessStoredBody(raw, *candidate.Body)
}

// decodedMailOriginal decodes a stored original. One that no longer decodes is no
// evidence of anything, so the row it belongs to is left as it is.
func decodedMailOriginal(payload []byte) ([]byte, bool) {
	raw, err := capture.DecodeStoredOriginal(payload)
	return raw, err == nil
}

// capturedByMailConnector names the connectors whose mail mailmap parsed.
const capturedByMailConnector = `(split_part(a.captured_by, ':', 1) = 'connector'
 AND split_part(a.captured_by, ':', 2) IN ('gmail', 'imap', 'graph'))`

// A restricted row refuses every update, so it is not offered.
const unassessedMailBodyScope = `FROM activity a ` + mailOriginalJoin + `
 WHERE a.kind = 'email' AND a.body_completeness = 'unassessed' AND a.restricted_at IS NULL`

// mailOriginalJoin is the stored original the row itself links, and only that.
// A Message-ID is typed by the sender, so an original found by it may be a
// planted one. A row with no link has no original here, and is never guessed at.
const mailOriginalJoin = `LEFT JOIN raw_capture rc ON rc.id = a.raw_capture_id`

func estimateUnassessedMailBodies(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, "SELECT count(*) FROM (SELECT a.id "+unassessedMailBodyScope+" LIMIT @cap) candidates", pgx.NamedArgs{"cap": maximum + 1}).Scan(&count)
	return min(count, maximum), count > maximum, err
}
