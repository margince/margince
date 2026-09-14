// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func requireEmploymentRead(ctx context.Context, tx pgx.Tx, contact ids.ContactID) error {
	for _, object := range []string{contactEntity, companyEntity, tableRelationship} {
		if err := auth.Require(ctx, object, principal.ActionRead); err != nil {
			return err
		}
	}
	return auth.EnsureLinkTarget(ctx, tx, contactEntity, contact.UUID)
}

func readEmploymentEvidence(ctx context.Context, tx pgx.Tx, contact ids.ContactID) ([]employmentEvidence, error) {
	if err := requireEmploymentRead(ctx, tx, contact); err != nil {
		return nil, err
	}
	args := []any{contact}
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT id, run_id, provider, claim_key, value_json, retrieved_at
 FROM contact_provider_claim WHERE contact_id = $%d AND claim_key IN ('current_employment','job_history') ORDER BY retrieved_at DESC, CASE claim_key WHEN 'current_employment' THEN 0 ELSE 1 END, id DESC LIMIT 501`, len(args)), args...)
	if err != nil {
		return nil, fmt.Errorf("reading retained employment purchases: %w", err)
	}
	defer rows.Close()
	var out []employmentEvidence
	count := 0
	latestCurrent := map[string]bool{}
	for rows.Next() {
		count++
		if count > 500 {
			return nil, &EmploymentImportError{Field: contactFK, Message: "More than 500 retained employment purchases require an administrator to consolidate the history."}
		}
		var claim employmentEvidence
		var kind string
		var raw []byte
		if err := rows.Scan(&claim.claimID, &claim.runID, &claim.provider, &kind, &raw, &claim.retrieved); err != nil {
			return nil, err
		}
		episodes, err := employmentEpisodes(kind, raw)
		if err != nil {
			episodes = []employmentEvidence{{CompanyName: "Unresolved employment evidence", Status: employmentUnknown, key: claim.claimID.String(), invalidDates: true}}
		}
		for _, e := range episodes {
			if kind == employmentCurrentClaim && latestCurrent[claim.provider] {
				e.superseded = true
				e.Status = employmentUnknown
			}
			e.claimID, e.runID, e.provider, e.retrieved = claim.claimID, claim.runID, claim.provider, claim.retrieved
			out = append(out, e)
		}
		if kind == employmentCurrentClaim {
			latestCurrent[claim.provider] = true
		}
	}
	return out, rows.Err()
}

// PreviewEmploymentImport reads retained evidence and current outcomes without mutations.
func (s *Store) PreviewEmploymentImport(ctx context.Context, contact ids.ContactID) (crmcontracts.EmploymentImportReport, error) {
	out := crmcontracts.EmploymentImportReport{ContactId: openapi_types.UUID(contact.UUID), Items: []crmcontracts.EmploymentImportItem{}}
	err := s.tx(ctx, func(tx pgx.Tx) error {
		args := []any{contact}
		var warning *string
		if err := requireEmploymentRead(ctx, tx, contact); err != nil {
			return err
		}
		if err := tx.QueryRow(ctx, storekit.SQLf("SELECT max(employment_processing_error) FROM contact_provider_claim WHERE contact_id=$%d", len(args)), args...).Scan(&warning); err != nil {
			return err
		}
		if warning != nil {
			out.Warnings = &[]string{*warning}
		}
		episodes, err := readEmploymentEvidence(ctx, tx, contact)
		if err != nil {
			var evidenceError *EmploymentImportError
			if errors.As(err, &evidenceError) {
				out.Warnings = &[]string{evidenceError.Message}
				return nil
			}
			return err
		}
		var today time.Time
		if err := tx.QueryRow(ctx, `SELECT current_date`).Scan(&today); err != nil {
			return err
		}
		seen := map[string]bool{}
		for _, e := range episodes {
			if seen[e.key] {
				continue
			}
			seen[e.key] = true
			e.Status = e.effectiveStatus(today)
			item, err := employmentImportItem(ctx, tx, contact, e)
			if err != nil {
				return err
			}
			out.Items = append(out.Items, item)
		}
		return nil
	})
	return out, err
}

func employmentImportItem(ctx context.Context, tx pgx.Tx, contact ids.ContactID, e employmentEvidence) (crmcontracts.EmploymentImportItem, error) {
	out := employmentEvidenceItem(e)
	args := []any{contact}
	contactPos := len(args)
	args = append(args, e.key)
	keyPos := len(args)
	var state string
	var company, relationship *ids.UUID
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT state, company_id, relationship_id FROM provider_employment_resolution
 WHERE contact_id=$%d AND episode_key=$%d ORDER BY updated_at DESC,id DESC LIMIT 1`, contactPos, keyPos), args...).Scan(&state, &company, &relationship)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	if relationship != nil {
		args := []any{*relationship}
		var archived bool
		if err := tx.QueryRow(ctx, storekit.SQLf(`SELECT archived_at IS NOT NULL FROM relationship WHERE id=$%d`, len(args)), args...).Scan(&archived); err != nil {
			return out, err
		}
		if archived {
			state = actionDismissed
		}
	}
	if state == employmentLinked && relationship == nil {
		state = "pending"
	}
	out.State = crmcontracts.EmploymentImportItemState(state)
	if company != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, *company); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				out.State = "needs_match"
				return out, nil
			}
			return out, err
		}
		out.CompanyId = uuidPtr(company)
		research, err := employmentResearchState(ctx, tx, *company)
		if err != nil {
			return out, err
		}
		out.ResearchState = &research
	}
	out.RelationshipId = uuidPtr(relationship)
	return out, nil
}

func employmentResearchState(ctx context.Context, tx pgx.Tx, company ids.UUID) (string, error) {
	args := []any{company}
	read, err := latestSiteReadTx(ctx, tx, company)
	if err == nil {
		return read.Status, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	var hasDomain bool
	err = tx.QueryRow(ctx, storekit.SQLf(`SELECT EXISTS(SELECT 1 FROM company_domain WHERE company_id=$%d AND is_primary AND archived_at IS NULL)`, len(args)), args...).Scan(&hasDomain)
	if err != nil {
		return "", err
	}
	if !hasDomain {
		return "needs_website", nil
	}
	return "awaiting_research_policy", nil
}

func employmentEvidenceItem(e employmentEvidence) crmcontracts.EmploymentImportItem {
	out := crmcontracts.EmploymentImportItem{
		Key: e.key, Provider: e.provider, CompanyName: e.CompanyName, Role: e.Role,
		EmploymentStatus: crmcontracts.EmploymentImportItemEmploymentStatus(e.Status), State: "pending", RetrievedAt: &e.retrieved,
	}
	if e.CompanyDomain != "" {
		out.Domain = &e.CompanyDomain
	}
	if e.Started != "" {
		out.Started = &e.Started
	}
	if e.Ended != "" {
		out.Ended = &e.Ended
	}
	return out
}
