// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The read side of the installation's own company: one assembly, called by the
// form's GET and by every write that answers with the record it just saved, so
// what a save returns and what a later read returns are the same view.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// readCompany assembles the form's view: the name and website from the
// company, every profile field from its provenance row.
func readAnchorCompany(ctx context.Context, tx pgx.Tx, companyID ids.CompanyID) (Company, error) {
	out := Company{CompanyID: companyID, Fields: map[string]string{}}
	if err := tx.QueryRow(ctx,
		`SELECT o.display_name, o.source, o.captured_by, o.updated_at,
		        o.logo_object_key, o.logo_icon_object_key, d.domain
		   FROM company o
		   LEFT JOIN company_domain d
		     ON d.company_id = o.id AND d.is_primary AND d.archived_at IS NULL
		  WHERE o.id = $1`,
		companyID).Scan(&out.DisplayName, &out.CompanySource, &out.CompanyCapturedBy,
		&out.UpdatedAt, &out.LogoObjectKey, &out.LogoIconObjectKey, &out.Website); err != nil {
		return Company{}, fmt.Errorf("read company: %w", err)
	}

	rows, err := tx.Query(ctx,
		`SELECT field, value, evidence_snippet, source_url, confidence,
		        source, captured_by, updated_at
		   FROM company_profile_field
		  WHERE company_id = $1
		  ORDER BY field`,
		companyID)
	if err != nil {
		return Company{}, fmt.Errorf("read company fields: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var field CompanyProfileField
		if err := rows.Scan(&field.Field, &field.Value, &field.EvidenceSnippet,
			&field.SourceURL, &field.Confidence, &field.Source, &field.CapturedBy,
			&field.UpdatedAt); err != nil {
			return Company{}, fmt.Errorf("scan company field: %w", err)
		}
		out.Fields[field.Field] = field.Value
		out.ProfileFields = append(out.ProfileFields, field)
	}
	if err := rows.Err(); err != nil {
		return Company{}, fmt.Errorf("read company fields: %w", err)
	}

	facts, err := tx.Query(ctx,
		`SELECT category, field, value, value_key, evidence_snippet, source_url,
		        confidence, source, captured_by, updated_at, version
		   FROM company_fact
		  WHERE company_id = $1
		  ORDER BY category, field, value_key, value`,
		companyID)
	if err != nil {
		return Company{}, fmt.Errorf("read company facts: %w", err)
	}
	defer facts.Close()
	for facts.Next() {
		var fact CompanyFact
		if err := facts.Scan(&fact.Category, &fact.Field, &fact.Value, &fact.ValueKey,
			&fact.EvidenceSnippet, &fact.SourceURL, &fact.Confidence, &fact.Source,
			&fact.CapturedBy, &fact.UpdatedAt, &fact.Version); err != nil {
			return Company{}, fmt.Errorf("scan company fact: %w", err)
		}
		out.Facts = append(out.Facts, fact)
	}
	if err := facts.Err(); err != nil {
		return Company{}, fmt.Errorf("read company facts: %w", err)
	}
	out.MinimumComplete = strings.TrimSpace(out.DisplayName) != "" &&
		strings.TrimSpace(out.Fields[fieldOfferSummary]) != "" &&
		strings.TrimSpace(out.Fields[fieldICP]) != ""
	return out, nil
}
