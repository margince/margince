// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The company-360 read side of the evidence sidecars: the confirmed facts
// (company_fact) and profile fields (company_profile_field) an
// company carries, row-scoped and evidence-or-omit. Values are staged
// and accepted through the deep-read/enrich pipeline; these reads only
// surface what a human already confirmed. A company with none answers [] —
// an empty picture is honest, never an error.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ensureCompanyReadable gates a row-scoped company sub-resource read: the row-scope
// clause plus an existence probe, so a non-existent or foreign company is
// existence-hidden (404) exactly like GetCompany — never a misleading
// empty list. EnsureVisible alone skips the existence check when the
// caller's row-scope is unrestricted (its clause is empty).
func ensureCompanyReadable(ctx context.Context, tx pgx.Tx, id ids.CompanyID) error {
	if err := auth.EnsureVisible(ctx, tx, "company", id.UUID); err != nil {
		return err
	}
	var exists bool
	if err := tx.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM company WHERE id = $1)`, id).Scan(&exists); err != nil {
		return fmt.Errorf("probe company existence: %w", err)
	}
	if !exists {
		return apperrors.ErrNotFound
	}
	return nil
}

// ListCompanyFacts returns the company's confirmed facts, ordered for a
// stable grouped render (category, then field, then the per-value key).
func (s *Store) ListCompanyFacts(ctx context.Context, id ids.CompanyID) ([]crmcontracts.CompanyFact, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.CompanyFact
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureCompanyReadable(ctx, tx, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, category, field, value, value_key, source, captured_by,
			       evidence_snippet, source_url, confidence, retrieved_at,
			       verified_at, verified_by, updated_at, version
			  FROM company_fact
			 WHERE company_id = $1
			 ORDER BY category, field, value_key, value`,
			id)
		if err != nil {
			return fmt.Errorf("list company facts: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				fact                    crmcontracts.CompanyFact
				factID                  ids.UUID
				category, field, source string
			)
			if err := rows.Scan(&factID, &category, &field, &fact.Value, &fact.ValueKey,
				&source, &fact.CapturedBy, &fact.EvidenceSnippet, &fact.SourceUrl,
				&fact.Confidence, &fact.RetrievedAt,
				&fact.VerifiedAt, &fact.VerifiedBy, &fact.UpdatedAt,
				&fact.Version); err != nil {
				return fmt.Errorf("scan company fact: %w", err)
			}
			// The row's own id, so a brief sentence written from this fact can
			// cite something the reader can open.
			id := openapi_types.UUID(factID)
			fact.Id = &id
			fact.Category = crmcontracts.CompanyFactCategory(category)
			fact.Field = crmcontracts.CompanyFactField(field)
			fact.Source = crmcontracts.CompanyFactSource(source)
			// Computed at read rather than stored: the rules are a judgment
			// about shape and will be tuned, and a stored verdict would keep
			// answering with the version of the rule that happened to run on
			// the day the row landed.
			if reason := factSuspectReason(fact.Field, fact.Value); reason != "" {
				suspect := crmcontracts.CompanyFactSuspectReason(reason)
				fact.SuspectReason = &suspect
			}
			out = append(out, fact)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate company facts: %w", err)
		}
		return nil
	})
	return out, err
}

// ListCompanyProfileFields returns the company's confirmed profile fields
// (company_profile_field). Items reuse CompanyProfileField — the
// table's field/source vocabulary is identical (migration 0099).
func (s *Store) ListCompanyProfileFields(ctx context.Context, id ids.CompanyID) ([]crmcontracts.CompanyProfileField, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return nil, err
	}
	var out []crmcontracts.CompanyProfileField
	err := s.tx(ctx, func(tx pgx.Tx) error {
		if err := ensureCompanyReadable(ctx, tx, id); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `
			SELECT id, field, value, source, captured_by,
			       evidence_snippet, source_url, confidence, retrieved_at,
			       verified_at, verified_by, updated_at, version
			  FROM company_profile_field
			 WHERE company_id = $1
			 ORDER BY field`,
			id)
		if err != nil {
			return fmt.Errorf("list company profile fields: %w", err)
		}
		defer rows.Close()
		for rows.Next() {
			var (
				pf            crmcontracts.CompanyProfileField
				rowID         ids.UUID
				field, source string
			)
			if err := rows.Scan(&rowID, &field, &pf.Value, &source, &pf.CapturedBy,
				&pf.EvidenceSnippet, &pf.SourceUrl, &pf.Confidence,
				&pf.RetrievedAt, &pf.VerifiedAt, &pf.VerifiedBy,
				&pf.UpdatedAt, &pf.Version); err != nil {
				return fmt.Errorf("scan company profile field: %w", err)
			}
			// The row's own identity, so a dossier sentence written from this
			// field can cite something the reader can open.
			wireID := openapi_types.UUID(rowID)
			pf.Id = &wireID
			pf.Field = crmcontracts.CompanyProfileFieldField(field)
			pf.Source = crmcontracts.CompanyProfileFieldSource(source)
			out = append(out, pf)
		}
		if err := rows.Err(); err != nil {
			return fmt.Errorf("iterate company profile fields: %w", err)
		}
		return nil
	})
	return out, err
}
