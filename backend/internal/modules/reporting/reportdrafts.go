// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package reporting

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// A report's revisions form one line. Published ones run from 1 to
// report_definition.revision, and an open draft is the one number after it.
// Evaluation serves revisions up to the published one, so a reader never
// reaches a draft, and editions and schedules only ever pin a published one.

// editableReport locks a live report the caller may edit, at the version the
// caller last saw.
func (s *Service) editableReport(ctx context.Context, tx pgx.Tx, id ids.UUID, version int64) (crmcontracts.ReportingReport, error) {
	report, err := s.report(ctx, tx, id, true)
	if err != nil {
		return report, err
	}
	if err := s.canEditReport(ctx, tx, report); err != nil {
		return report, err
	}
	if report.ArchivedAt != nil {
		return report, apperrors.ErrNotFound
	}
	if report.Version != version {
		return report, apperrors.ErrVersionSkew
	}
	return report, nil
}

// refuseOpenDraft keeps a direct publish from passing over a draft. Publishing
// past it would leave the draft's number among the published ones.
func refuseOpenDraft(report crmcontracts.ReportingReport) error {
	if report.DraftRevision != nil {
		return invalid("this report has an unpublished draft; publish or discard it first")
	}
	return nil
}

// requireDraftWrite admits a human whose seat may write, or an agent acting
// for one. A draft and a new private report change nothing another reader
// sees, so an agent may prepare them. Publishing and sharing stay a human's.
func requireDraftWrite(ctx context.Context, action principal.Action) error {
	if err := auth.RequireActingForAHuman(ctx, "a report draft is prepared"); err != nil {
		return err
	}
	if actor, _ := principal.Actor(ctx); !actor.SeatType.CanMutate() {
		return apperrors.ErrPermissionDenied
	}
	return requireGrants(ctx, "report_definition", action)
}

// refuseAgentOverDraft keeps an agent from replacing an open draft. Editors
// preview a draft, so it may be a human's unpublished work.
func refuseAgentOverDraft(ctx context.Context, report crmcontracts.ReportingReport) error {
	actor, ok := principal.Actor(ctx)
	if ok && actor.Type == principal.PrincipalAgent && report.DraftRevision != nil {
		return invalid("this report already has a draft; an editor publishes or discards it before another is prepared")
	}
	return nil
}

// DraftNewReport creates a report only its author sees, whatever audience the
// input names, so a new report starts as private as a draft does.
func (s *Service) DraftNewReport(ctx context.Context, in crmcontracts.ReportingReportInput) (crmcontracts.ReportingReport, error) {
	if err := requireDraftWrite(ctx, principal.ActionCreate); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	in.Audience, in.AudienceTeamId = reportingPrivate, nil
	return s.insertReport(ctx, in)
}

// SaveReportDraft stores the next revision as a draft only editors see. A
// second save replaces the draft, which no reader, edition or schedule holds.
func (s *Service) SaveReportDraft(ctx context.Context, id ids.UUID, version int64, in crmcontracts.ReportingReportDraftInput) (crmcontracts.ReportingReport, error) {
	if err := requireDraftWrite(ctx, principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var out crmcontracts.ReportingReport
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.editableReport(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if err := s.validateRevision(ctx, tx, &in.Selection); err != nil {
			return err
		}
		if err := refuseAgentOverDraft(ctx, before); err != nil {
			return err
		}
		replaced, err := s.draftSelection(ctx, tx, before)
		if err != nil {
			return err
		}
		if before.DraftRevision != nil {
			if err := s.dropRevision(ctx, tx, id, *before.DraftRevision); err != nil {
				return err
			}
		}
		draft := before.Revision + 1
		staged := before
		staged.Revision, staged.Selection = draft, in.Selection
		if err := s.saveReportRevision(ctx, tx, &staged); err != nil {
			return err
		}
		out = before
		out.DraftRevision = &draft
		out.Version++
		return s.writeDraftState(ctx, tx, reportImage{before, replaced}, reportImage{out, &staged.Selection}, "update")
	})
	return out, err
}

// DiscardReportDraft removes the draft and leaves the published revision.
func (s *Service) DiscardReportDraft(ctx context.Context, id ids.UUID, version int64) (crmcontracts.ReportingReport, error) {
	if err := requireReadWrite(ctx, "report_definition", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var out crmcontracts.ReportingReport
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.editableReport(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if before.DraftRevision == nil {
			return apperrors.ErrNotFound
		}
		discarded, err := s.draftSelection(ctx, tx, before)
		if err != nil {
			return err
		}
		out = before
		out.DraftRevision = nil
		out.Version++
		if err := s.writeDraftState(ctx, tx, reportImage{before, discarded}, reportImage{out, nil}, "update"); err != nil {
			return err
		}
		return s.dropRevision(ctx, tx, id, *before.DraftRevision)
	})
	return out, err
}

// PublishReportDraft makes the draft the revision every reader sees.
func (s *Service) PublishReportDraft(ctx context.Context, id ids.UUID, version int64) (crmcontracts.ReportingReport, error) {
	if err := requireReadWrite(ctx, "report_definition", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingReport{}, err
	}
	var out crmcontracts.ReportingReport
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		before, err := s.editableReport(ctx, tx, id, version)
		if err != nil {
			return err
		}
		if before.DraftRevision == nil {
			return apperrors.ErrNotFound
		}
		selection, _, err := s.revisionSelection(ctx, tx, id, *before.DraftRevision)
		if err != nil {
			return err
		}
		// Checked again under the publisher: another editor may have saved the
		// draft, and the scope and calendar it was checked against may have moved.
		if err := s.validateRevision(ctx, tx, &selection); err != nil {
			return err
		}
		if err := s.rewriteDraftSelection(ctx, tx, id, *before.DraftRevision, selection); err != nil {
			return err
		}
		out = before
		out.Revision, out.Selection, out.DraftRevision = *before.DraftRevision, selection, nil
		out.Version++
		return s.writeDraftState(ctx, tx, reportImage{before, &selection}, reportImage{out, nil}, "update")
	})
	return out, err
}

// RestoreReportRevision publishes a copy of an earlier revision as the newest
// one. The earlier revision, and every edition and schedule that names a
// revision, stay as they were.
func (s *Service) RestoreReportRevision(ctx context.Context, id ids.UUID, version, revision int64) (crmcontracts.ReportingReport, error) {
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
		if revision >= before.Revision {
			return invalid("choose a revision published before the current one")
		}
		selection, _, err := s.reportRevision(ctx, tx, before, revision)
		if err != nil {
			return err
		}
		if err := s.validateRevision(ctx, tx, &selection); err != nil {
			return err
		}
		out = before
		out.Revision++
		out.Selection = selection
		out.Version++
		if err := s.saveReportRevision(ctx, tx, &out); err != nil {
			return err
		}
		return s.writeDraftState(ctx, tx, reportImage{before, nil}, reportImage{out, nil}, "restore")
	})
	return out, err
}

// ListReportRevisions answers every revision, newest first, to an editor.
func (s *Service) ListReportRevisions(ctx context.Context, id ids.UUID) (crmcontracts.ReportingReportRevisionList, error) {
	if err := requireReadWrite(ctx, "report_definition", principal.ActionUpdate); err != nil {
		return crmcontracts.ReportingReportRevisionList{}, err
	}
	out := crmcontracts.ReportingReportRevisionList{Data: []crmcontracts.ReportingReportRevision{}}
	err := s.store.db.Tx(ctx, func(tx pgx.Tx) error {
		report, err := s.report(ctx, tx, id, false)
		if err != nil {
			return err
		}
		if err := s.canEditReport(ctx, tx, report); err != nil {
			return err
		}
		var b bindings
		query := "SELECT revision,created_at,created_by,selection FROM report_definition_revision WHERE report_id=" + b.add(id) + " ORDER BY revision DESC"
		rows, err := tx.Query(ctx, query, b.values...)
		if err != nil {
			return err
		}
		out.Data, err = pgx.CollectRows(rows, func(row pgx.CollectableRow) (crmcontracts.ReportingReportRevision, error) {
			var r crmcontracts.ReportingReportRevision
			err := row.Scan(&r.Revision, &r.CreatedAt, &r.CreatedBy, &r.Selection)
			r.State = revisionState(report, r.Revision)
			return r, err
		})
		return err
	})
	return out, err
}

func revisionState(report crmcontracts.ReportingReport, revision int64) crmcontracts.ReportingReportRevisionState {
	switch {
	case report.DraftRevision != nil && revision == *report.DraftRevision:
		return crmcontracts.ReportingReportRevisionStateDraft
	case revision == report.Revision:
		return crmcontracts.ReportingReportRevisionStatePublished
	default:
		return crmcontracts.ReportingReportRevisionStateEarlier
	}
}

// reportImage is a report as the audit records a revision change: the report,
// and the charts its draft holds when one is open.
type reportImage struct {
	crmcontracts.ReportingReport
	DraftSelection *crmcontracts.ReportingSelection `json:"draft_selection,omitempty"`
}

// writeDraftState records the report's published revision, draft and version
// as one change. The audit's before and after images show which of the three
// moved, with the draft's charts; the action is from the audit log's closed set.
func (s *Service) writeDraftState(ctx context.Context, tx pgx.Tx, before, after reportImage, action string) error {
	var b bindings
	query := "UPDATE report_definition SET revision=" + b.add(after.Revision) + ",draft_revision=" + b.add(after.DraftRevision) + ",version=" + b.add(after.Version) + " WHERE id=" + b.add(after.Id)
	if _, err := tx.Exec(ctx, query, b.values...); err != nil {
		return fmt.Errorf("reporting: moving a report's revisions: %w", err)
	}
	return recordChange(ctx, tx, "report_definition", ids.UUID(after.Id), action, &before, after)
}

// draftSelection is the open draft's charts, or nil when there is none.
func (s *Service) draftSelection(ctx context.Context, tx pgx.Tx, report crmcontracts.ReportingReport) (draft *crmcontracts.ReportingSelection, err error) {
	if report.DraftRevision != nil {
		var selection crmcontracts.ReportingSelection
		selection, _, err = s.revisionSelection(ctx, tx, ids.UUID(report.Id), *report.DraftRevision)
		draft = &selection
	}
	return draft, err
}

// rewriteDraftSelection stores a draft's selection as checked at publish, so
// the published revision holds what the publisher resolved.
func (s *Service) rewriteDraftSelection(ctx context.Context, tx pgx.Tx, id ids.UUID, revision int64, selection crmcontracts.ReportingSelection) error {
	encoded, err := encode(selection)
	if err != nil {
		return err
	}
	var b bindings
	query := "UPDATE report_definition_revision SET selection=" + b.add(encoded) + " WHERE report_id=" + b.add(id) + " AND revision=" + b.add(revision)
	_, err = tx.Exec(ctx, query, b.values...)
	return err
}

func (s *Service) dropRevision(ctx context.Context, tx pgx.Tx, id ids.UUID, revision int64) error {
	var b bindings
	query := "DELETE FROM report_definition_revision WHERE report_id=" + b.add(id) + " AND revision=" + b.add(revision)
	_, err := tx.Exec(ctx, query, b.values...)
	return err
}

// evaluableRevision is a published revision, or the draft for an editor.
// report() keeps DraftRevision only for one who may edit. A reader asking for
// the draft's number is answered as for any unpublished one.
func (s *Service) evaluableRevision(ctx context.Context, tx pgx.Tx, report crmcontracts.ReportingReport, revision int64) (crmcontracts.ReportingSelection, int64, error) {
	if report.DraftRevision != nil && revision == *report.DraftRevision {
		return s.revisionSelection(ctx, tx, ids.UUID(report.Id), revision)
	}
	return s.reportRevision(ctx, tx, report, revision)
}

// revisionSelection reads one revision's selection, draft or published.
func (s *Service) revisionSelection(ctx context.Context, tx pgx.Tx, id ids.UUID, revision int64) (crmcontracts.ReportingSelection, int64, error) {
	var selection crmcontracts.ReportingSelection
	var framework int64
	var b bindings
	query := "SELECT selection,framework_revision FROM report_definition_revision WHERE report_id=" + b.add(id) + " AND revision=" + b.add(revision)
	err := tx.QueryRow(ctx, query, b.values...).Scan(&selection, &framework)
	return selection, framework, storedError(err)
}
