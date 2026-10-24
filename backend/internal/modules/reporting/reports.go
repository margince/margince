// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"errors"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

const (
	reportColumns = "d.id,d.owner_id,d.name,d.description,d.audience,d.audience_team_id,d.revision,d.version,d.created_at,d.archived_at,r.selection,d.draft_revision"
	reportFrom    = " FROM report_definition d JOIN report_definition_revision r ON r.report_id=d.id AND r.revision=d.revision"
)

func scanReport(row pgx.Row) (crmcontracts.ReportingReport, error) {
	var report crmcontracts.ReportingReport
	err := row.Scan(&report.Id, &report.OwnerId, &report.Name, &report.Description, &report.Audience, &report.AudienceTeamId, &report.Revision, &report.Version, &report.CreatedAt, &report.ArchivedAt, &report.Selection, &report.DraftRevision)
	return report, storedError(err)
}

func audienceClause(ctx context.Context, b *bindings) (string, error) {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return "", apperrors.ErrPermissionDenied
	}
	if actor.Type == principal.PrincipalSystem || (actor.Permissions.RowScope == principal.RowScopeAll && auth.Allows(ctx, "user_admin", principal.ActionUpdate)) {
		return "TRUE", nil
	}
	owner, err := actorID(ctx)
	if err != nil {
		return "", err
	}
	return "(d.owner_id=" + b.add(owner) + " OR d.audience='workspace' OR (d.audience='team' AND d.audience_team_id=ANY(" + b.add(actor.TeamIDs) + ")))", nil
}

func (s *Service) report(ctx context.Context, tx pgx.Tx, id ids.UUID, lock bool) (crmcontracts.ReportingReport, error) {
	var b bindings
	audience, err := audienceClause(ctx, &b)
	if err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	query := "SELECT " + reportColumns + reportFrom + " WHERE " + audience + " AND d.id=" + b.add(id)
	if lock {
		query += " FOR UPDATE OF d"
	}
	report, err := scanReport(tx.QueryRow(ctx, query, b.values...))
	if err != nil {
		return report, err
	}
	allowed := s.canEditReport(ctx, tx, report)
	if allowed != nil && !errors.Is(allowed, apperrors.ErrPermissionDenied) && !errors.Is(allowed, apperrors.ErrNotFound) {
		return report, allowed
	}
	manage := allowed == nil
	report.CanManage = &manage
	// A draft is the editors' work in progress, so a reader is not told of it.
	if !manage {
		report.DraftRevision = nil
	}
	return report, nil
}

// GetReport enforces the saved report’s current audience.
func (s *Service) GetReport(ctx context.Context, id ids.UUID) (crmcontracts.ReportingReport, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var out crmcontracts.ReportingReport
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error { var err error; out, err = s.report(ctx, tx, id, false); return err })
	return out, err
}

// ListReports filters the library by current audience and schedule state.
func (s *Service) ListReports(ctx context.Context, after *ids.UUID, limit int, scheduled bool) (crmcontracts.ReportingReportList, error) {
	if err := auth.Require(ctx, "report_definition", principal.ActionRead); err != nil {
		return crmcontracts.ReportingReportList{}, err
	}
	if scheduled {
		if err := auth.Require(ctx, "report_schedule", principal.ActionRead); err != nil {
			return crmcontracts.ReportingReportList{}, err
		}
	}
	limit = max(1, min(limit, 100))
	out := crmcontracts.ReportingReportList{Data: []crmcontracts.ReportingReport{}}
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		var b bindings
		where, err := audienceClause(ctx, &b)
		if err != nil {
			return err
		}
		where += " AND d.archived_at IS NULL"
		if scheduled {
			where += " AND EXISTS(SELECT 1 FROM report_schedule scheduled WHERE scheduled.report_id=d.id)"
		}
		if after != nil {
			where += " AND d.id>" + b.add(*after)
		}
		rows, err := tx.Query(ctx, "SELECT "+reportColumns+reportFrom+" WHERE "+where+" ORDER BY d.id LIMIT "+b.add(limit+1), b.values...)
		if err != nil {
			return err
		}
		out.Data, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingReport, error) {
			report, err := scanReport(row)
			report.DraftRevision = nil
			return report, err
		})
		if err != nil {
			return err
		}
		return populateReportSummaries(ctx, tx, out.Data)
	})
	if len(out.Data) > limit {
		cursor := out.Data[limit-1].Id.String()
		out.NextCursor = &cursor
		out.Data = out.Data[:limit]
	}
	return out, err
}

func (s *Service) validateReport(ctx context.Context, tx pgx.Tx, in *crmcontracts.ReportingReportInput) error {
	if !values.HasVisibleText(in.Name) || utf8.RuneCountInString(in.Name) > 160 {
		return refuse("name", "invalid", "name the report in at most 160 characters")
	}
	if in.Description != nil && utf8.RuneCountInString(*in.Description) > 2000 {
		return refuse("description", "invalid", "describe the report in at most 2000 characters")
	}
	if err := s.validateRevision(ctx, tx, &in.Selection); err != nil {
		return err
	}
	return s.validateReportAudience(ctx, tx, in)
}

// validateRevision checks what one revision holds, and resolves its scope. A
// draft and a published revision pass the same checks.
func (s *Service) validateRevision(ctx context.Context, tx pgx.Tx, selection *crmcontracts.ReportingSelection) error {
	deriveLayoutBlocks(selection)
	if err := s.validateSelection(ctx, *selection); err != nil {
		return err
	}
	scope, err := s.authority.AssertedScope(ctx, tx, selection.Scope)
	if err != nil {
		return refusedField("selection.scope", err)
	}
	selection.Scope = scope
	if err := s.authority.Pipeline(ctx, tx, (*ids.UUID)(selection.PipelineId)); err != nil {
		return err
	}
	calendar, err := s.calendar(ctx, tx)
	if err != nil {
		return err
	}
	interval, err := Interval(*selection, calendar, s.now())
	if err != nil {
		if _, custom := errors.AsType[intervalError](err); custom {
			return refusedField("selection.interval", err)
		}
		return refusedField("selection.period", err)
	}
	if _, err := TargetWindow(interval, string(selection.TargetBasis), calendar); err != nil {
		return refusedField("selection.target_basis", err)
	}
	if selection.CloseWindow != "all_open" && selection.CloseWindow != reportingFiscalQuarter {
		return refuse("selection.close_window", "invalid", "choose the expected-close window")
	}
	return nil
}

func (s *Service) validateSelection(ctx context.Context, selection crmcontracts.ReportingSelection) error {
	catalog, err := s.evaluator.Catalog(ctx, textlang.English)
	if err != nil {
		return err
	}
	if len(selection.Metrics) == 0 || len(selection.Metrics) > 8 {
		return refuse("selection.metrics", "out_of_range", "choose between one and eight supported metrics")
	}
	if len(selection.Blocks) > 20 {
		return refuse("selection.blocks", "out_of_range", "choose at most twenty charts")
	}
	allowed := map[crmcontracts.ReportingBlockKind]bool{}
	metricBlocks := map[crmcontracts.ReportingMetricID][]crmcontracts.ReportingBlockKind{}
	for _, id := range selection.Metrics {
		if _, seen := metricBlocks[id]; seen {
			return refuse("selection.metrics", "duplicate", "a metric may appear only once")
		}
		index := slices.IndexFunc(catalog.Metrics, func(m crmcontracts.ReportingMetricDefinition) bool { return m.Id == id })
		if index < 0 {
			return refuse("selection.metrics", "unsupported", "a selected metric is unavailable")
		}
		metricBlocks[id] = catalog.Metrics[index].Blocks
		for _, block := range catalog.Metrics[index].Blocks {
			allowed[block] = true
		}
	}
	if selection.Layout != nil {
		if err := validateLayout(*selection.Layout, metricBlocks); err != nil {
			return err
		}
	}
	blocks := map[crmcontracts.ReportingBlockKind]bool{}
	for _, block := range selection.Blocks {
		if !allowed[block] || blocks[block] {
			return refuse("selection.blocks", "invalid", "choose distinct charts belonging to the selected metrics")
		}
		blocks[block] = true
	}
	return nil
}

// CreateReport stores the first immutable definition revision.
func (s *Service) CreateReport(ctx context.Context, in crmcontracts.ReportingReportInput) (crmcontracts.ReportingReport, error) {
	if err := requireReadWrite(ctx, "report_definition", principal.ActionCreate); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	owner, err := actorID(ctx)
	if err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	out := crmcontracts.ReportingReport{Id: openapi_types.UUID(ids.NewV7()), OwnerId: openapi_types.UUID(owner), Name: strings.TrimSpace(in.Name), Description: reportDescription(in.Description), Audience: crmcontracts.ReportingReportAudience(in.Audience), AudienceTeamId: in.AudienceTeamId, Selection: in.Selection, Revision: 1, Version: 1, CreatedAt: s.now()}
	err = s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := s.validateReport(ctx, tx, &in); err != nil {
			return err
		}
		out.Selection = in.Selection
		var b bindings
		values := []string{b.add(out.Id), b.add(owner), b.add(out.Name), b.add(out.Description), b.add(out.Audience), b.add(out.AudienceTeamId), b.add(1), b.add(1), b.add(out.CreatedAt)}
		_, err := tx.Exec(ctx, "INSERT INTO report_definition(id,owner_id,name,description,audience,audience_team_id,revision,version,created_at) VALUES ("+strings.Join(values, ",")+")", b.values...)
		if err != nil {
			return err
		}
		if err := s.saveReportRevision(ctx, tx, out); err != nil {
			return err
		}
		return recordChange(ctx, tx, "report_definition", ids.UUID(out.Id), "create", nil, out)
	})
	return out, err
}

// reportDescription trims what the author typed. A blank description is no
// description, so the report page shows nothing rather than an empty line.
func reportDescription(typed *string) *string {
	if typed == nil || !values.HasVisibleText(*typed) {
		return nil
	}
	trimmed := strings.TrimSpace(*typed)
	return &trimmed
}

func (s *Service) saveReportRevision(ctx context.Context, tx pgx.Tx, report crmcontracts.ReportingReport) error {
	framework, err := s.framework(ctx, tx)
	if err != nil {
		return err
	}
	author, err := actorID(ctx)
	if err != nil {
		return err
	}
	selection, err := encode(report.Selection)
	if err != nil {
		return err
	}
	var b bindings
	values := []string{b.add(report.Id), b.add(report.Revision), b.add(selection), b.add(framework.Revision), b.add(s.now()), b.add(author)}
	_, err = tx.Exec(ctx, "INSERT INTO report_definition_revision(report_id,revision,selection,framework_revision,created_at,created_by) VALUES ("+strings.Join(values, ",")+")", b.values...)
	return err
}

func (s *Service) canEditReport(ctx context.Context, tx pgx.Tx, report crmcontracts.ReportingReport) error {
	actor, err := actorID(ctx)
	if err != nil {
		return err
	}
	if actor == ids.UUID(report.OwnerId) {
		return nil
	}
	scope := crmcontracts.ReportingScope{Kind: reportingWorkspace}
	if report.Audience == reportingTeam {
		scope = crmcontracts.ReportingScope{Kind: reportingTeam, Id: report.AudienceTeamId}
	}
	if report.Audience == "private" {
		return apperrors.ErrNotFound
	}
	_, err = s.authority.Scope(ctx, tx, scope, true)
	return err
}

// UpdateReport appends a revision under optimistic concurrency control.
func (s *Service) UpdateReport(ctx context.Context, id ids.UUID, version int64, in crmcontracts.ReportingReportInput) (crmcontracts.ReportingReport, error) {
	if err := requireReadWrite(ctx, "report_definition", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var out crmcontracts.ReportingReport
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.editableReport(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if err := refuseOpenDraft(before); err != nil {
			return err
		}
		if err := s.validateReport(ctx, tx, &in); err != nil {
			return err
		}
		out = before
		out.Name = strings.TrimSpace(in.Name)
		// Omitted keeps the stored purpose: a client that predates the field
		// must not erase it. Blank text is how an author clears it.
		if in.Description != nil {
			out.Description = reportDescription(in.Description)
		}
		out.Audience = crmcontracts.ReportingReportAudience(in.Audience)
		out.AudienceTeamId = in.AudienceTeamId
		out.Selection = in.Selection
		out.Revision++
		out.Version++
		var b bindings
		query := "UPDATE report_definition SET name=" + b.add(out.Name) + ",description=" + b.add(out.Description) + ",audience=" + b.add(out.Audience) + ",audience_team_id=" + b.add(out.AudienceTeamId) + ",revision=" + b.add(out.Revision) + ",version=" + b.add(out.Version) + " WHERE id=" + b.add(id)
		if _, err := tx.Exec(ctx, query, b.values...); err != nil {
			return err
		}
		if err := s.saveReportRevision(ctx, tx, out); err != nil {
			return err
		}
		return recordChange(ctx, tx, "report_definition", id, "update", &before, out)
	})
	return out, err
}

// ArchiveReport pauses scheduling while preserving captured editions.
func (s *Service) ArchiveReport(ctx context.Context, id ids.UUID) (crmcontracts.ReportingReport, error) {
	if err := requireReadWrite(ctx, "report_definition", principal.ActionDelete); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var out crmcontracts.ReportingReport
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.report(ctx, tx, id, true)
		if err != nil {
			return err
		}
		if err := s.canEditReport(ctx, tx, before); err != nil {
			return err
		}
		if before.ArchivedAt != nil {
			out = before
			return nil
		}
		at := s.now()
		out = before
		out.ArchivedAt = &at
		out.Version++
		var b bindings
		query := "UPDATE report_definition SET archived_at=" + b.add(at) + ",version=" + b.add(out.Version) + " WHERE id=" + b.add(id)
		if _, err := tx.Exec(ctx, query, b.values...); err != nil {
			return err
		}
		if err := s.pauseReportSchedules(ctx, tx, id); err != nil {
			return err
		}
		return recordChange(ctx, tx, "report_definition", id, "archive", &before, out)
	})
	return out, err
}

func (s *Service) validateReportAudience(ctx context.Context, tx pgx.Tx, in *crmcontracts.ReportingReportInput) error {
	switch in.Audience {
	case "private":
		if in.AudienceTeamId != nil {
			return refuse("audience_team_id", "not_allowed", "a private report has no audience team")
		}
	case reportingWorkspace:
		if in.AudienceTeamId != nil {
			return refuse("audience_team_id", "not_allowed", "a company report has no audience team")
		}
		_, err := s.authority.Scope(ctx, tx, crmcontracts.ReportingScope{Kind: reportingWorkspace}, true)
		return err
	case reportingTeam:
		if in.AudienceTeamId == nil {
			return refuse("audience_team_id", "required", "choose an audience team")
		}
		_, err := s.authority.Scope(ctx, tx, crmcontracts.ReportingScope{Kind: reportingTeam, Id: in.AudienceTeamId}, true)
		return err
	default:
		return refuse("audience", "invalid", "choose private, team or company availability")
	}
	return nil
}
