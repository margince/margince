// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MarkerAuditPageSize keeps the global hold-lock window to one correction.
const MarkerAuditPageSize = 1

const (
	weeklyCommitmentEntity = "weekly_plan_commitment"
	auditKindParameter     = "kind"
)

const markedWeeklySQL = `record.label=@marker AND record.linked_record_type IS NULL AND record.linked_record_id IS NULL
 AND record.help_requested='' AND record.manager_response='' AND record.manager_user_id IS NULL AND record.responded_at IS NULL`

const markedRequestSQL = `record.subject_ref='erased+'||record.id::text AND record.contact_id IS NULL
 AND record.status IN ('fulfilled','rejected') AND (record.resolution IS NULL OR record.resolution='erased') AND NOT record.dispute_open`

const markerPendingSQL = `(EXISTS(SELECT 1 FROM audit_log history WHERE history.entity_type=@kind AND history.entity_id=record.id AND ` + storekit.AuditRedactionNeededSQL + `)
 OR EXISTS(SELECT 1 FROM event_outbox WHERE envelope->'entity'->>'type'=@kind AND envelope->'entity'->>'id'=record.id::text))`

type markedAuditSource struct{ kind, selectSQL, lockSQL string }

var weeklyAuditSource = markedAuditSource{
	kind:      weeklyCommitmentEntity,
	selectSQL: `SELECT record.id FROM weekly_plan_commitment record WHERE ` + markedWeeklySQL + ` AND ` + markerPendingSQL + ` AND record.id>@after AND record.id<@until ORDER BY record.id LIMIT @limit`,
	lockSQL:   `SELECT ` + markedWeeklySQL + ` FROM weekly_plan_commitment record WHERE record.id=@id FOR UPDATE NOWAIT`,
}

var retiredRequestAuditSource = markedAuditSource{
	kind:      rightsRequestEntity,
	selectSQL: `SELECT record.id FROM data_subject_request record WHERE ` + markedRequestSQL + ` AND ` + markerPendingSQL + ` AND record.id>@after AND record.id<@until ORDER BY record.id LIMIT @limit`,
	lockSQL:   `SELECT ` + markedRequestSQL + ` FROM data_subject_request record WHERE record.id=@id FOR UPDATE NOWAIT`,
}

// RedactMarkedWeeklyAuditPageTx uses the approved editable erasure marker.
// Later working text disqualifies a weekly item from this correction.
func RedactMarkedWeeklyAuditPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, _ CountryReader) (HistoricalAuditPage, error) {
	return redactMarkedAuditPage(ctx, tx, weeklyAuditSource, after, until, limit)
}

// RedactRetiredRequestAuditPageTx clears copies of machine-retired closed cases.
// Disputed cases and later answers remain outside the correction.
func RedactRetiredRequestAuditPageTx(ctx context.Context, tx pgx.Tx, after, until ids.UUID, limit int, _ CountryReader) (HistoricalAuditPage, error) {
	return redactMarkedAuditPage(ctx, tx, retiredRequestAuditSource, after, until, limit)
}

func markerArgs(source markedAuditSource) pgx.StrictNamedArgs {
	args := pgx.StrictNamedArgs{auditKindParameter: source.kind}
	if source.kind == weeklyCommitmentEntity {
		args["marker"] = erasedMention
	}
	return args
}

func redactMarkedAuditPage(ctx context.Context, tx pgx.Tx, source markedAuditSource, after, until ids.UUID, limit int) (HistoricalAuditPage, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return HistoricalAuditPage{}, err
	}
	args := markerArgs(source)
	args["after"], args["until"], args[auditPageLimitParameter] = after, until, limit
	rows, err := tx.Query(ctx, source.selectSQL, args)
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	idsToCheck, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return HistoricalAuditPage{}, err
	}
	page := HistoricalAuditPage{Scanned: idsToCheck}
	if len(idsToCheck) == 0 {
		return page, nil
	}
	held, err := markerAuditUnderHold(ctx, tx)
	if err != nil || held {
		return page, err
	}
	for _, id := range idsToCheck {
		cleared, err := lockMarkedAuditRecord(ctx, tx, source, id)
		if err != nil {
			return HistoricalAuditPage{}, err
		}
		if !cleared {
			continue
		}
		if err := certifyMarkedAudit(ctx, tx, source.kind, id); err != nil {
			return HistoricalAuditPage{}, err
		}
		page.Changed++
	}
	return page, nil
}

func markerAuditUnderHold(ctx context.Context, tx pgx.Tx) (bool, error) {
	if err := auth.RequireSystem(ctx); err != nil {
		return false, err
	}
	var held bool
	if err := tx.QueryRow(ctx, markerHoldSQL).Scan(&held); err != nil || held {
		return held, err
	}
	locked, err := storekit.Savepoint(ctx, tx, func(attempt pgx.Tx) (bool, error) {
		_, err := attempt.Exec(ctx, markerHoldLockSQL)
		if storekit.IsLockTimeout(err) {
			return false, nil
		}
		return err == nil, err
	})
	if err != nil || !locked {
		return !locked, err
	}
	err = tx.QueryRow(ctx, markerHoldSQL).Scan(&held)
	return held, err
}

func certifyMarkedAudit(ctx context.Context, tx pgx.Tx, kind string, id ids.UUID) error {
	var err error
	switch kind {
	case weeklyCommitmentEntity:
		_, err = certifyCollateralScrub(ctx, tx, "weekly_plan_commitment", id, subjectRedactionReason, subjectRedactionCause)
	case rightsRequestEntity:
		_, err = certifyCollateralScrub(ctx, tx, "data_subject_request", id, subjectRedactionReason, subjectRedactionCause)
	}
	return err
}

func lockMarkedAuditRecord(ctx context.Context, tx pgx.Tx, source markedAuditSource, id ids.UUID) (bool, error) {
	args := pgx.StrictNamedArgs{"id": id}
	if source.kind == weeklyCommitmentEntity {
		args["marker"] = erasedMention
	}
	return storekit.Savepoint(ctx, tx, func(attempt pgx.Tx) (bool, error) {
		var cleared bool
		err := attempt.QueryRow(ctx, source.lockSQL, args).Scan(&cleared)
		if err == pgx.ErrNoRows || storekit.IsLockTimeout(err) {
			return false, nil
		}
		return cleared, err
	})
}
