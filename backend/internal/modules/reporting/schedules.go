// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

const scheduleColumns = "id,report_id,owner_id,definition,timezone,version,next_due_at,last_status"

func scanSchedule(row pgx.Row) (crmcontracts.ReportingSchedule, error) {
	var out crmcontracts.ReportingSchedule
	err := row.Scan(&out.Id, &out.ReportId, &out.OwnerId, &out.Definition, &out.Timezone, &out.Version, &out.NextDueAt, &out.LastStatus)
	return out, storedError(err)
}

// ListSchedules checks the owning report before disclosing its schedules.
func (s *Service) ListSchedules(ctx context.Context, reportID ids.UUID) (crmcontracts.ReportingScheduleList, error) {
	if err := auth.Require(ctx, "report_schedule", principal.ActionRead); err != nil {
		return crmcontracts.ReportingScheduleList{}, err
	}
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingScheduleList{}, err
	}
	out := crmcontracts.ReportingScheduleList{Data: []crmcontracts.ReportingSchedule{}}
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if _, err := s.report(ctx, tx, reportID, false); err != nil {
			return err
		}
		var b bindings
		rows, err := tx.Query(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE report_id="+b.add(reportID)+" ORDER BY id", b.values...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			row, err := scanSchedule(rows)
			if err != nil {
				return err
			}
			out.Data = append(out.Data, row)
		}
		return rows.Err()
	})
	return out, err
}

func (s *Service) reportRevision(ctx context.Context, tx pgx.Tx, report crmcontracts.ReportingReport, revision int64) (crmcontracts.ReportingSelection, int64, error) {
	if revision < 1 || revision > report.Revision {
		return crmcontracts.ReportingSelection{}, 0, apperrors.ErrNotFound
	}
	var selection crmcontracts.ReportingSelection
	var framework int64
	var b bindings
	err := tx.QueryRow(ctx, "SELECT selection,framework_revision FROM report_definition_revision WHERE report_id="+b.add(report.Id)+" AND revision="+b.add(revision), b.values...).Scan(&selection, &framework)
	return selection, framework, storedError(err)
}

func (s *Service) validateSchedule(ctx context.Context, tx pgx.Tx, report crmcontracts.ReportingReport, in crmcontracts.ReportingScheduleInput) error {
	if in.Enabled {
		configured, err := scheduleReady(ctx, tx)
		if err != nil {
			return err
		}
		if !configured {
			return invalid("configure report edition retention before enabling a schedule")
		}
	}
	if report.ArchivedAt != nil {
		return apperrors.ErrNotFound
	}
	if err := s.canEditReport(ctx, tx, report); err != nil {
		return err
	}
	selection, _, err := s.reportRevision(ctx, tx, report, in.ReportRevision)
	if err != nil {
		return err
	}
	if selection.Scope.Kind == "managed_teams" {
		return invalid("choose one fixed team, owner or company for a schedule")
	}
	if _, err := s.authority.Scope(ctx, tx, selection.Scope, true); err != nil {
		return err
	}
	if err := s.validateSelection(ctx, selection); err != nil {
		return err
	}
	_, err = scheduleMinutes(in)
	return err
}

// CreateSchedule pins the report revision and computes its first local run.
func (s *Service) CreateSchedule(ctx context.Context, reportID ids.UUID, in crmcontracts.ReportingScheduleInput) (crmcontracts.ReportingSchedule, error) {
	if err := requireReadWrite(ctx, "report_schedule", principal.ActionCreate); err != nil {
		return crmcontracts.ReportingSchedule{}, err
	}
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingSchedule{}, err
	}
	owner, err := actorID(ctx)
	if err != nil {
		return crmcontracts.ReportingSchedule{}, err
	}
	out := crmcontracts.ReportingSchedule{Id: openapi_types.UUID(ids.NewV7()), ReportId: openapi_types.UUID(reportID), OwnerId: openapi_types.UUID(owner), Definition: in, Version: 1}
	err = s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		report, err := s.report(ctx, tx, reportID, true)
		if err != nil {
			return err
		}
		if err := s.validateSchedule(ctx, tx, report, in); err != nil {
			return err
		}
		var count int
		var b bindings
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM report_schedule WHERE report_id="+b.add(reportID), b.values...).Scan(&count); err != nil {
			return err
		}
		if count >= 20 {
			return invalid("a report supports at most twenty schedules")
		}
		calendar, err := s.calendar(ctx, tx)
		if err != nil {
			return err
		}
		out.Timezone = calendar.Timezone
		out.NextDueAt, err = NextDue(in, out.Timezone, s.now())
		if err != nil {
			return err
		}
		raw, err := encode(in)
		if err != nil {
			return err
		}
		b = bindings{}
		values := []string{b.add(out.Id), b.add(reportID), b.add(in.ReportRevision), b.add(owner), b.add(raw), b.add(out.Timezone), b.add(in.Enabled), b.add(1), b.add(out.NextDueAt)}
		if _, err := tx.Exec(ctx, "INSERT INTO report_schedule(id,report_id,report_revision,owner_id,definition,timezone,enabled,version,next_due_at) VALUES ("+strings.Join(values, ",")+")", b.values...); err != nil {
			return err
		}
		return recordChange(ctx, tx, "report_schedule", ids.UUID(out.Id), "create", nil, out)
	})
	return out, err
}

// UpdateSchedule uses version checks to preserve concurrent scheduling intent.
func (s *Service) UpdateSchedule(ctx context.Context, id ids.UUID, version int64, in crmcontracts.ReportingScheduleInput) (crmcontracts.ReportingSchedule, error) {
	if err := requireReadWrite(ctx, "report_schedule", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingSchedule{}, err
	}
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingSchedule{}, err
	}
	var out crmcontracts.ReportingSchedule
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var b bindings
		before, err := scanSchedule(tx.QueryRow(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE id="+b.add(id), b.values...))
		if err != nil {
			return err
		}
		report, err := s.report(ctx, tx, ids.UUID(before.ReportId), true)
		if err != nil {
			return err
		}
		b = bindings{}
		before, err = scanSchedule(tx.QueryRow(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE id="+b.add(id)+" FOR UPDATE", b.values...))
		if err != nil {
			return err
		}
		if err := s.validateSchedule(ctx, tx, report, in); err != nil {
			return err
		}
		if before.Version != version {
			return apperrors.ErrVersionSkew
		}
		calendar, err := s.calendar(ctx, tx)
		if err != nil {
			return err
		}
		out = before
		out.Definition = in
		out.Version++
		out.Timezone = calendar.Timezone
		out.NextDueAt, err = NextDue(in, out.Timezone, s.now())
		if err != nil {
			return err
		}
		if err := s.writeSchedule(ctx, tx, out); err != nil {
			return err
		}
		return recordChange(ctx, tx, "report_schedule", id, "update", &before, out)
	})
	return out, err
}

func (s *Service) writeSchedule(ctx context.Context, tx pgx.Tx, schedule crmcontracts.ReportingSchedule) error {
	raw, err := encode(schedule.Definition)
	if err != nil {
		return err
	}
	var b bindings
	query := "UPDATE report_schedule SET definition=" + b.add(raw) + ",report_revision=" + b.add(schedule.Definition.ReportRevision) + ",timezone=" + b.add(schedule.Timezone) + ",enabled=" + b.add(schedule.Definition.Enabled) + ",version=" + b.add(schedule.Version) + ",next_due_at=" + b.add(schedule.NextDueAt) + ",last_status=" + b.add(schedule.LastStatus) + " WHERE id=" + b.add(schedule.Id)
	_, err = tx.Exec(ctx, query, b.values...)
	return err
}

func (s *Service) pauseReportSchedules(ctx context.Context, tx pgx.Tx, reportID ids.UUID) error {
	var b bindings
	rows, err := tx.Query(ctx, "SELECT "+scheduleColumns+" FROM report_schedule WHERE report_id="+b.add(reportID)+" AND enabled FOR UPDATE", b.values...)
	if err != nil {
		return err
	}
	schedules, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingSchedule, error) { return scanSchedule(row) })
	if err != nil {
		return err
	}
	for _, before := range schedules {
		after := before
		after.Definition.Enabled = false
		after.Version++
		if err := s.writeSchedule(ctx, tx, after); err != nil {
			return err
		}
		if err := recordChange(ctx, tx, "report_schedule", ids.UUID(after.Id), "update", &before, after); err != nil {
			return err
		}
	}
	return nil
}

func scheduleReady(ctx context.Context, tx pgx.Tx) (bool, error) {
	var ready bool
	err := tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM retention_policy WHERE object_type='report_edition' AND action='erase' AND enabled)").Scan(&ready)
	return ready, err
}
