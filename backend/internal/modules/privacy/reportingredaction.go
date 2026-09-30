// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package privacy

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func redactSubjectReporting(ctx context.Context, tx pgx.Tx, subject ids.ContactID) error {
	if err := storekit.FenceReportProjection(ctx, tx); err != nil {
		return err
	}
	args := []ids.UUID{subject.UUID}
	p := storekit.Placeholders(args)
	rows, err := tx.Query(ctx, `SELECT DISTINCT c.source_type,c.source_id FROM report_edition_contribution c WHERE
 (c.source_type='deal' AND c.source_id IN (SELECT deal_id FROM relationship WHERE contact_id=`+p+` AND kind='deal_stakeholder')) OR
 (c.source_type='activity' AND c.source_id IN (SELECT activity_id FROM activity_link WHERE contact_id=`+p+`)) OR
 (c.source_type='sdr_handoff' AND c.source_id IN (SELECT id FROM sdr_handoff WHERE contact_id=`+p+` OR lead_id IN (SELECT id FROM lead WHERE promoted_contact_id=`+p+`)))`, subject.UUID)
	if err != nil {
		return err
	}
	type source struct {
		kind string
		id   ids.UUID
	}
	sources, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (source, error) {
		var s source
		err := row.Scan(&s.kind, &s.id)
		return s, err
	})
	if err != nil {
		return err
	}
	for _, s := range sources {
		if err := redactReportingSource(ctx, tx, s.kind, []ids.UUID{s.id}); err != nil {
			return err
		}
	}
	return nil
}

//craft:ignore naked-any Bound SQL parameters carry pgx's heterogeneous value types.
func redactReportingSource(ctx context.Context, tx pgx.Tx, kind string, sources []ids.UUID) error {
	if len(sources) == 0 {
		return nil
	}
	if err := storekit.FenceReportProjection(ctx, tx); err != nil {
		return err
	}
	args := []any{kind, sources}
	p := strings.Split(storekit.Placeholders(args), ",")
	rows, err := tx.Query(ctx, "SELECT manifest FROM report_edition WHERE id IN (SELECT edition_id FROM report_edition_contribution WHERE source_type="+p[0]+" AND source_id=ANY("+p[1]+")) ORDER BY id FOR UPDATE", args...)
	if err != nil {
		return err
	}
	editions, err := pgx.CollectRows(rows, pgx.RowTo[crmcontracts.ReportingEdition])
	if err != nil {
		return err
	}
	if kind == reportingDeal {
		// Stakeholder erasure keeps the business deal but removes its copied label.
		_, err = tx.Exec(ctx, "UPDATE report_edition_contribution SET fact=jsonb_set(fact,'{row,label}','\"Deal\"'::jsonb) WHERE source_type="+p[0]+" AND source_id=ANY("+p[1]+")", args...)
	} else {
		_, err = tx.Exec(ctx, "DELETE FROM report_edition_contribution WHERE source_type="+p[0]+" AND source_id=ANY("+p[1]+")", args...)
	}
	if err != nil {
		return err
	}
	for _, edition := range editions {
		// Reporting reconstructs permitted additive values from surviving facts;
		// no pre-erasure total or label remains in the stored projection.
		edition = clearedReportingEdition(edition)
		raw, err := json.Marshal(edition)
		if err != nil {
			return err
		}
		args = []any{raw, edition.Id}
		p = strings.Split(storekit.Placeholders(args), ",")
		if _, err := tx.Exec(ctx, "UPDATE report_edition SET manifest="+p[0]+",redacted_at=now() WHERE id="+p[1], args...); err != nil {
			return err
		}
	}
	return nil
}

func clearedReportingEdition(edition crmcontracts.ReportingEdition) crmcontracts.ReportingEdition {
	edition.Redacted = true
	edition.Withheld = true
	edition.Name = "Redacted report"
	edition.Evaluation.EvaluationKey = ""
	edition.Evaluation.Context.MemberIds = nil
	edition.Evaluation.Context.PopulationFingerprint = ""
	edition.Evaluation.Context.Scope.Label = nil
	edition.Evaluation.Selection.Scope.Label = nil
	edition.Evaluation.Charts = []crmcontracts.ReportingChart{}
	for i, metric := range edition.Evaluation.Metrics {
		edition.Evaluation.Metrics[i] = crmcontracts.ReportingMetric{Id: metric.Id, Unit: metric.Unit, Version: metric.Version, Evidence: crmcontracts.ReportingEvidenceRef{Metric: metric.Id, ContextId: metric.Evidence.ContextId}, Coverage: crmcontracts.ReportingCoverage{Status: "partial", Withheld: true}}
	}
	return edition
}

func (*RetentionService) eraseReportEdition(ctx context.Context, tx pgx.Tx, id ids.UUID) error {
	if err := storekit.FenceReportProjection(ctx, tx); err != nil {
		return err
	}
	p := storekit.Placeholders([]ids.UUID{id})
	var edition crmcontracts.ReportingEdition
	if err := tx.QueryRow(ctx, "SELECT manifest FROM report_edition WHERE id="+p+" FOR UPDATE", id).Scan(&edition); err != nil {
		return err
	}
	edition = clearedReportingEdition(edition)
	expired := true
	edition.Expired = &expired
	edition.Name = "Expired report edition"
	edition.Evaluation.Metrics = []crmcontracts.ReportingMetric{}
	if _, err := tx.Exec(ctx, "DELETE FROM report_edition_contribution WHERE edition_id="+p, id); err != nil {
		return err
	}
	raw, err := json.Marshal(edition)
	if err != nil {
		return err
	}
	return expireReportManifest(ctx, tx, id, raw)
}

//craft:ignore naked-any Bound SQL arguments contain a UUID and JSON bytes.
func expireReportManifest(ctx context.Context, tx pgx.Tx, id ids.UUID, raw []byte) error {
	args := []any{raw, id}
	p := strings.Split(storekit.Placeholders(args), ",")
	_, err := tx.Exec(ctx, "UPDATE report_edition SET manifest="+p[0]+",expired_at=now(),redacted_at=now() WHERE id="+p[1], args...)
	return err
}
