// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/consent"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/datatask"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func subjectMailDataTask(now func() time.Time) datatask.Task {
	return datatask.Task{
		ID: "core/1793502625_direct_sender_notice_duties", Version: 1,
		Title: "Closing notice duties for direct senders", Requires: []string{"core/1792474140"},
		Steps: []datatask.Step{{
			Name: "restore_bulk_sender_proof", Limit: 50, MinLimit: 1,
			Audit:    datatask.AuditPolicy{Reason: "capture stamps the same derived sender authentication on new mail without a separate audit"},
			Estimate: estimateSenderAuthOriginals, Batch: restoreSenderAuthBatch,
		}, {
			Name: "settle_direct_senders", Limit: 200, MinLimit: 10, EmptyWhenDone: true,
			Audit: datatask.AuditPolicy{PerRecord: true}, Estimate: estimateSubjectMailDuties,
			Batch: func(ctx context.Context, tx pgx.Tx, in datatask.BatchIn) (datatask.BatchOut, error) {
				return settleSubjectMailBatch(ctx, tx, in, now())
			},
		}},
	}
}

func subjectMailScopeSQL() string {
	return `FROM (SELECT DISTINCT e.contact_id AS id FROM contact_acquisition_evidence e
 WHERE e.kind = 'unknown_legacy' AND ` + capturedAcquisition + `
 AND EXISTS (SELECT 1 FROM privacy_notice_case n WHERE n.acquisition_id = e.id
 AND n.state = ANY(@states))) c WHERE true
 AND EXISTS (SELECT 1 FROM contact_email ce
 JOIN activity_participant p ON lower(p.address) = ce.email AND p.role = 'from'
 JOIN activity a ON a.id = p.activity_id
 WHERE ce.contact_id = c.id AND ce.archived_at IS NULL AND ` + correspondence.SubjectMailSQL("a") + `)`
}

func estimateSubjectMailDuties(ctx context.Context, q datatask.Querier, maximum int64) (int64, bool, error) {
	if err := auth.Require(ctx, "data_task", principal.ActionRead); err != nil {
		return 0, false, err
	}
	var count int64
	err := q.QueryRow(ctx, `SELECT count(*) FROM (SELECT c.id `+subjectMailScopeSQL()+` LIMIT @subject_limit) candidates`, pgx.NamedArgs{"subject_limit": maximum + 1, "states": consent.SettleableNoticeStates()}).Scan(&count)
	return min(count, maximum), count > maximum, err
}

func settleSubjectMailBatch(ctx context.Context, tx pgx.Tx, in datatask.BatchIn, now time.Time) (datatask.BatchOut, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return datatask.BatchOut{}, err
	}
	after, err := datatask.UUIDCursor(in.Cursor)
	if err != nil {
		return datatask.BatchOut{}, err
	}
	rows, err := tx.Query(ctx, `SELECT c.id `+subjectMailScopeSQL()+` AND c.id > @subject_after ORDER BY c.id LIMIT @subject_limit`, pgx.NamedArgs{"subject_after": after, "subject_limit": in.Limit, "states": consent.SettleableNoticeStates()})
	if err != nil {
		return datatask.BatchOut{}, err
	}
	contacts, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return datatask.BatchOut{}, err
	}
	changed := 0
	for _, contact := range contacts {
		var mail ids.UUID
		var sent time.Time
		err := tx.QueryRow(ctx, `SELECT a.id, a.occurred_at FROM contact_email ce
   JOIN activity_participant p ON lower(p.address) = ce.email AND p.role = 'from'
   JOIN activity a ON a.id = p.activity_id
   WHERE ce.contact_id = @subject_contact AND ce.archived_at IS NULL AND `+correspondence.SubjectMailSQL("a")+`
   ORDER BY a.occurred_at, a.id LIMIT 1`, pgx.NamedArgs{"subject_contact": contact}).Scan(&mail, &sent)
		if errors.Is(err, pgx.ErrNoRows) {
			continue
		}
		if err != nil {
			return datatask.BatchOut{}, err
		}
		closed, err := settleSenderCasesTx(ctx, tx, contact, mail, sent, now)
		if err != nil {
			return datatask.BatchOut{}, err
		}
		if closed > 0 {
			changed++
		}
	}
	return datatask.AdvanceUUID(in, contacts, changed)
}
