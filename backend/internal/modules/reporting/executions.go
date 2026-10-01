// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const executionColumns = "id,report_id,report_revision,status,attempt,intended_due_at,edition_id,reason"

func scanExecution(row pgx.Row) (crmcontracts.ReportingExecution, error) {
	var out crmcontracts.ReportingExecution
	err := row.Scan(&out.Id, &out.ReportId, &out.ReportRevision, &out.Status, &out.Attempt, &out.IntendedDueAt, &out.EditionId, &out.Reason)
	return out, storedError(err)
}

func (s *Service) execution(ctx context.Context, tx pgx.Tx, id ids.UUID) (crmcontracts.ReportingExecution, error) {
	var b bindings
	out, err := scanExecution(tx.QueryRow(ctx, "SELECT "+executionColumns+" FROM report_execution WHERE id="+b.add(id), b.values...))
	if err != nil {
		return out, err
	}
	_, err = s.report(ctx, tx, ids.UUID(out.ReportId), false)
	return out, err
}

// GetExecution applies report audience authority to execution status.
func (s *Service) GetExecution(ctx context.Context, id ids.UUID) (crmcontracts.ReportingExecution, error) {
	if err := auth.Require(ctx, "report_edition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingExecution{}, err
	}
	var out crmcontracts.ReportingExecution
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error { var err error; out, err = s.execution(ctx, tx, id); return err })
	return out, err
}

// ListExecutions exposes run history only through the owning report’s audience.
func (s *Service) ListExecutions(ctx context.Context, reportID ids.UUID, after *ids.UUID, limit int) (crmcontracts.ReportingExecutionList, error) {
	out := crmcontracts.ReportingExecutionList{Data: []crmcontracts.ReportingExecution{}}
	if err := auth.Require(ctx, "report_edition", principal.ActionRead); err != nil {
		return out, err
	}
	limit = max(1, min(100, limit))
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := s.report(ctx, tx, reportID, false); err != nil {
			return err
		}
		var b bindings
		where := "report_id=" + b.add(reportID)
		if after != nil {
			where += " AND id<" + b.add(*after)
		}
		rows, err := tx.Query(ctx, "SELECT "+executionColumns+" FROM report_execution WHERE "+where+" ORDER BY id DESC LIMIT "+b.add(limit+1), b.values...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			item, err := scanExecution(rows)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, item)
		}
		return rows.Err()
	})
	if len(out.Data) > limit {
		cursor := out.Data[limit-1].Id.String()
		out.NextCursor = &cursor
		out.Data = out.Data[:limit]
	}
	return out, err
}

// Freeze pins a report revision and records idempotent publication intent.
func (s *Service) Freeze(ctx context.Context, reportID ids.UUID, revision int64, key string) (crmcontracts.ReportingExecution, error) {
	if err := requireReadWrite(ctx, "report_edition", principal.ActionCreate); err != nil {
		return crmcontracts.ReportingExecution{}, err
	}
	if len(key) < 1 || len(key) > 128 {
		return crmcontracts.ReportingExecution{}, invalid("supply a stable idempotency key of 1–128 characters")
	}
	owner, err := actorID(ctx)
	if err != nil {
		return crmcontracts.ReportingExecution{}, err
	}
	requestedRevision := revision
	var out crmcontracts.ReportingExecution
	err = s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		report, err := s.report(ctx, tx, reportID, true)
		if err != nil {
			return err
		}
		if err := s.canEditReport(ctx, tx, report); err != nil {
			return err
		}
		if revision == 0 {
			revision = report.Revision
		}
		selection, _, err := s.reportRevision(ctx, tx, report, revision)
		if err != nil {
			return err
		}
		calendar, err := s.calendar(ctx, tx)
		if err != nil {
			return err
		}
		interval, err := Interval(selection, calendar, s.now())
		if err != nil {
			return err
		}
		identity := owner.String() + ":" + reportID.String() + ":" + key
		raw, err := json.Marshal(struct{ Revision int64 }{requestedRevision})
		if err != nil {
			return err
		}
		digest := sha256.Sum256(raw)
		hash := hex.EncodeToString(digest[:])
		out, err = s.insertExecution(ctx, tx, executionIntent{Report: report, Revision: revision, Owner: owner, Due: s.now(), Interval: interval, Key: identity, Hash: hash})
		return err
	})
	return out, err
}

type executionIntent struct {
	Report          crmcontracts.ReportingReport
	Revision        int64
	Owner           ids.UUID
	Due             time.Time
	Interval        crmcontracts.ReportingWindow
	Key, Hash       string
	Status, Reason  string
	ScheduleID      *ids.UUID
	ScheduleVersion *int64
}

func (s *Service) insertExecution(ctx context.Context, tx pgx.Tx, in executionIntent) (crmcontracts.ReportingExecution, error) {
	id := ids.NewV7()
	if in.Status == "" {
		in.Status = reportingPending
	}
	var b bindings
	values := []string{b.add(id), b.add(in.Report.Id), b.add(in.Revision), b.add(in.Owner), b.add(in.Due), b.add(in.Interval.StartAt), b.add(in.Interval.EndAt), b.add(in.Status), b.add(in.Key), b.add(in.Hash), b.add(in.ScheduleID), b.add(in.ScheduleVersion), b.add(in.Reason)}
	command, err := tx.Exec(ctx, "INSERT INTO report_execution(id,report_id,report_revision,owner_id,intended_due_at,interval_start,interval_end,status,execution_key,request_hash,schedule_id,schedule_version,reason) VALUES ("+strings.Join(values, ",")+") ON CONFLICT (execution_key) DO NOTHING", b.values...)
	if err != nil {
		return crmcontracts.ReportingExecution{}, err
	}
	b = bindings{}
	var existingHash string
	if err := tx.QueryRow(ctx, "SELECT id,request_hash FROM report_execution WHERE execution_key="+b.add(in.Key), b.values...).Scan(&id, &existingHash); err != nil {
		return crmcontracts.ReportingExecution{}, err
	}
	if existingHash != in.Hash {
		return crmcontracts.ReportingExecution{}, apperrors.ErrConflict
	}
	b = bindings{}
	out, err := scanExecution(tx.QueryRow(ctx, "SELECT "+executionColumns+" FROM report_execution WHERE id="+b.add(id), b.values...))
	if err != nil {
		return out, err
	}
	if command.RowsAffected() == 1 {
		err = recordChange(ctx, tx, "report_edition", id, "create", nil, out)
	}
	return out, err
}

// RetryExecution preserves publication intent while granting a fresh bounded retry budget.
func (s *Service) RetryExecution(ctx context.Context, id ids.UUID) (crmcontracts.ReportingExecution, error) {
	if err := requireReadWrite(ctx, "report_edition", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingExecution{}, err
	}
	var out crmcontracts.ReportingExecution
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.execution(ctx, tx, id)
		if err != nil {
			return err
		}
		report, err := s.report(ctx, tx, ids.UUID(before.ReportId), true)
		if err != nil {
			return err
		}
		if err := s.canEditReport(ctx, tx, report); err != nil {
			return err
		}
		if before.Status != "failed" && before.Status != reportingSuspended {
			return apperrors.ErrConflict
		}
		var b bindings
		command, err := tx.Exec(ctx, "UPDATE report_execution SET status='pending',retry_base_attempt=attempt,retry_at=NULL,reason=NULL WHERE id="+b.add(id)+" AND status="+b.add(before.Status), b.values...)
		if err != nil {
			return err
		}
		if command.RowsAffected() != 1 {
			return apperrors.ErrConflict
		}
		out = before
		out.Status = reportingPending
		out.Reason = nil
		return recordChange(ctx, tx, "report_edition", id, "update", &before, out)
	})
	return out, err
}
