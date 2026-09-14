// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
)

func demoteForRelationshipCreate(ctx context.Context, tx pgx.Tx, in CreateRelationshipInput) error {
	if in.Kind != employmentKind || in.IsCurrentPrimary == nil || !*in.IsCurrentPrimary || in.ContactID == nil {
		return nil
	}
	var args []any
	arg := func(v any) string { args = append(args, v); return storekit.SQLf("$%d", len(args)) }
	contact, ended, status, precision := arg(*in.ContactID), arg(in.EndedAt), arg(in.EmploymentStatus), arg(in.EndedPrecision)
	_, err := tx.Exec(ctx, storekit.SQLf(`UPDATE relationship SET is_current_primary=false WHERE contact_id=%s AND %s AND %s`, contact, employment.CurrentPrimarySlotSQL(""), employment.IsCurrentSQL(ended+"::date", status+"::text", precision+"::text")), args...)
	return err
}

func insertRelationshipRow(ctx context.Context, tx pgx.Tx, in CreateRelationshipInput, capturedBy string) (relationshipRow, error) {
	// Reentrant with the endpoint probe's lock: the writer itself owns the
	// archive exclusion, so extracting its caller cannot detach that guarantee.
	if in.ContactID != nil {
		if err := lockContactForAttach(ctx, tx, *in.ContactID); err != nil {
			return relationshipRow{}, err
		}
	}
	if in.CounterpartyContactID != nil {
		if err := lockContactForAttach(ctx, tx, *in.CounterpartyContactID); err != nil {
			return relationshipRow{}, err
		}
	}

	var args []any
	arg := func(v any) string { args = append(args, v); return storekit.SQLf("$%d", len(args)) }
	kind, contact, company := arg(in.Kind), arg(in.ContactID), arg(in.CompanyID)
	countercompany, countercontact, deal, project := arg(in.CounterpartyCompanyID), arg(in.CounterpartyContactID), arg(in.DealID), arg(in.ProjectID)
	role, primary, started, ended := arg(in.Role), arg(in.IsCurrentPrimary), arg(in.StartedAt), arg(in.EndedAt)
	source, captured, status, startPrecision, endPrecision := arg(in.Source), arg(capturedBy), arg(in.EmploymentStatus), arg(in.StartedPrecision), arg(in.EndedPrecision)
	// An unspecified primary is derived only when there is no competing current
	// employment. Explicit false preserves the caller's choice.
	primaryValue := storekit.SQLf(`coalesce(%s, %s='employment' AND NOT EXISTS(
 SELECT 1 FROM relationship WHERE kind='employment' AND contact_id=%s AND archived_at IS NULL
 AND (%s OR is_current_primary))) AND (%s<>'employment' OR %s)`, primary, kind, contact, employment.IsCurrentSQL("ended_at"), kind, employment.IsCurrentSQL(ended+"::date", status+"::text", endPrecision+"::text"))
	query := storekit.SQLf(`INSERT INTO relationship(kind,contact_id,company_id,counterparty_company_id,counterparty_contact_id,deal_id,project_id,
 role,is_current_primary,started_at,ended_at,source,captured_by,employment_status,started_precision,ended_precision)
 VALUES(%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s,%s) RETURNING %s`,
		kind, contact, company, countercompany, countercontact, deal, project, role, primaryValue, started, ended, source, captured, status, startPrecision, endPrecision, relationshipColumns)
	return scanRelationship(tx.QueryRow(ctx, query, args...))
}
