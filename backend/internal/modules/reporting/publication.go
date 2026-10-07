// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var errPublicationLeaseLost = errors.New("report publication lease is no longer current")

type editionReceipt struct {
	ID       openapi_types.UUID `json:"id"`
	ReportID openapi_types.UUID `json:"report_id"`
	Revision int64              `json:"revision"`
}

type frozenRun struct {
	Execution       crmcontracts.ReportingExecution
	Interval        crmcontracts.ReportingWindow
	ScheduleID      *ids.UUID
	ScheduleVersion *int64
}

func (s *Service) executeClaim(ctx context.Context, claim executionClaim) error {
	return s.store.db.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		human, err := s.authority.PublicationHuman(ctx, tx, claim.Owner)
		if err != nil {
			return err
		}
		if err := requireReadWrite(human, "report_edition", principal.ActionCreate); err != nil {
			return err
		}

		run, report, err := s.publicationGuard(human, tx, claim)
		if err != nil {
			return err
		}
		selection, frameworkID, err := s.reportRevision(human, tx, report, run.Execution.ReportRevision)
		if err != nil {
			return err
		}
		selection.Period = "custom"
		selection.Interval = &run.Interval
		framework, err := s.frameworkRevision(human, tx, frameworkID)
		if err != nil {
			return err
		}
		evaluation, err := s.evaluateSelection(human, tx, selection, framework)
		if err != nil {
			return err
		}
		if !evaluation.Result.Context.Interval.StartAt.Equal(run.Interval.StartAt) || !evaluation.Result.Context.Interval.EndAt.Equal(run.Interval.EndAt) {
			return invalid("reporting cutoff precedes the requested capture interval")
		}
		edition := crmcontracts.ReportingEdition{Id: openapi_types.UUID(ids.NewV7()), ReportId: report.Id, ReportRevision: run.Execution.ReportRevision, Name: report.Name, CapturedAt: evaluation.Result.Context.EvaluatedAt, IntendedDueAt: run.Execution.IntendedDueAt, Evaluation: evaluation.Result}
		if err := s.writeEdition(human, tx, claim, report, edition, evaluation.Facts); err != nil {
			return err
		}
		return s.completeClaim(human, tx, claim, edition)
	})
}

func (s *Service) publicationGuard(ctx context.Context, tx pgx.Tx, claim executionClaim) (frozenRun, crmcontracts.ReportingReport, error) {
	var run frozenRun
	var b bindings
	execution, err := s.execution(ctx, tx, claim.ID)
	if err != nil {
		return run, crmcontracts.ReportingReport{}, err
	}
	run.Execution = execution
	report, err := s.report(ctx, tx, ids.UUID(execution.ReportId), true)
	if err != nil {
		return run, report, err
	}
	if report.ArchivedAt != nil {
		return run, report, apperrors.ErrConflict
	}
	if err := s.canEditReport(ctx, tx, report); err != nil {
		return run, report, err
	}
	err = tx.QueryRow(ctx, "SELECT interval_start,interval_end,schedule_id,schedule_version FROM report_execution WHERE id="+b.add(claim.ID)+" AND status='running' AND fence="+b.add(claim.Fence)+" AND lease_until>"+b.add(s.now()), b.values...).Scan(&run.Interval.StartAt, &run.Interval.EndAt, &run.ScheduleID, &run.ScheduleVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, report, errPublicationLeaseLost
	}
	if err != nil {
		return run, report, storedError(err)
	}
	if run.ScheduleID != nil {
		b = bindings{}
		var enabled bool
		var version int64
		err = tx.QueryRow(ctx, "SELECT enabled,version FROM report_schedule WHERE id="+b.add(*run.ScheduleID)+" FOR UPDATE", b.values...).Scan(&enabled, &version)
		if err != nil {
			return run, report, err
		}
		if !enabled || run.ScheduleVersion == nil || version != *run.ScheduleVersion {
			return run, report, apperrors.ErrConflict
		}
	}
	return run, report, nil
}

func (s *Service) writeEdition(ctx context.Context, tx pgx.Tx, claim executionClaim, report crmcontracts.ReportingReport, edition crmcontracts.ReportingEdition, facts []Fact) error {
	raw, err := encode(edition)
	if err != nil {
		return err
	}
	var b bindings
	values := []string{b.add(edition.Id), b.add(claim.ID), b.add(report.Id), b.add(edition.ReportRevision), b.add(report.Audience), b.add(report.AudienceTeamId), b.add(report.OwnerId), b.add(edition.CapturedAt), b.add(edition.IntendedDueAt), b.add(raw)}
	if _, err := tx.Exec(ctx, "INSERT INTO report_edition(id,execution_id,report_id,report_revision,publication_audience,publication_team_id,owner_id,captured_at,intended_due_at,manifest) VALUES ("+strings.Join(values, ",")+")", b.values...); err != nil {
		return err
	}
	if err := writeEditionContributions(ctx, tx, edition.Id, facts); err != nil {
		return err
	}

	return recordChange(ctx, tx, "report_edition", ids.UUID(edition.Id), "create", nil, editionReceipt{edition.Id, edition.ReportId, edition.ReportRevision})
}

func (s *Service) completeClaim(ctx context.Context, tx pgx.Tx, claim executionClaim, edition crmcontracts.ReportingEdition) error {
	before, err := executionState(ctx, tx, claim.ID)
	if err != nil {
		return err
	}
	status := "succeeded"
	for _, metric := range edition.Evaluation.Metrics {
		if metric.Coverage.Status != "ok" && metric.Coverage.Status != "no_data" {
			status = "partial"
		}
	}
	for _, chart := range edition.Evaluation.Charts {
		if chart.Coverage.Status != "ok" && chart.Coverage.Status != "no_data" {
			status = "partial"
		}
	}
	var b bindings
	command, err := tx.Exec(ctx, "UPDATE report_execution SET status="+b.add(status)+",edition_id="+b.add(edition.Id)+",lease_until=NULL,reason=NULL WHERE id="+b.add(claim.ID)+" AND status='running' AND fence="+b.add(claim.Fence)+" AND lease_until>"+b.add(s.now()), b.values...)
	if err != nil {
		return err
	}
	if command.RowsAffected() != 1 {
		return apperrors.ErrConflict
	}
	if err := recordExecutionChange(ctx, tx, claim.ID, before); err != nil {
		return err
	}
	return s.recordScheduleResult(ctx, tx, claim, status, "")
}

func (s *Service) failClaim(ctx context.Context, claim executionClaim, cause error) error {
	status := "failed"
	reason := "Report generation failed. Retry the run."
	var retry *time.Time
	switch {
	case errors.Is(cause, apperrors.ErrPermissionDenied), errors.Is(cause, apperrors.ErrNotFound):
		status = reportingSuspended
		reason = "The accountable owner can no longer generate this report. Review their access and schedule."
	case errors.Is(cause, apperrors.ErrConflict):
		status = "skipped"
		reason = "The report or schedule changed before publication."
	case claim.Attempt-claim.RetryBase < 3:
		status = reportingPending
		delay := 5 * time.Minute
		if claim.Attempt-claim.RetryBase > 1 {
			delay = 15 * time.Minute
		}
		at := s.now().Add(delay)
		retry = &at
	}
	return s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := executionState(ctx, tx, claim.ID)
		if err != nil {
			return err
		}
		var b bindings
		command, err := tx.Exec(ctx, "UPDATE report_execution SET status="+b.add(status)+",reason="+b.add(reason)+",retry_at="+b.add(retry)+",lease_until=NULL WHERE id="+b.add(claim.ID)+" AND status='running' AND fence="+b.add(claim.Fence), b.values...)
		if err != nil {
			return err
		}
		if command.RowsAffected() == 0 {
			return nil
		}
		if err := recordExecutionChange(ctx, tx, claim.ID, before); err != nil {
			return err
		}
		return s.recordScheduleResult(ctx, tx, claim, status, reason)
	})
}
