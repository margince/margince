// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// ApplyEmploymentImport reconciles retained provider evidence in one contact transaction.
func (s *Store) ApplyEmploymentImport(ctx context.Context, contact ids.ContactID, req crmcontracts.EmploymentImportRequest) (crmcontracts.EmploymentImportReport, error) {
	action, err := employmentImportAction(req)
	if err != nil {
		return crmcontracts.EmploymentImportReport{}, err
	}
	for _, grant := range []struct {
		object string
		action principal.Action
	}{{contactEntity, principal.ActionUpdate}, {tableRelationship, principal.ActionCreate}} {
		if err := auth.Require(ctx, grant.object, grant.action); err != nil {
			return crmcontracts.EmploymentImportReport{}, err
		}
	}
	err = s.tx(ctx, func(tx pgx.Tx) error {
		// Attach writers lock the contact before its employment identity. Keep
		// that order so an import cannot deadlock against a manual attachment.
		if err := auth.HoldWritableLive(ctx, tx, contactEntity, contact.UUID); err != nil {
			return err
		}
		if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contact.String()); err != nil {
			return err
		}
		episodes, err := readEmploymentEvidence(ctx, tx, contact)
		if err != nil {
			return err
		}
		if err := s.applyEmploymentEvidence(ctx, tx, contact, episodes, req, action); err != nil {
			return err
		}
		if req.Key == nil {
			args := []any{contact}
			_, err := tx.Exec(ctx, storekit.SQLf(`UPDATE contact_provider_claim SET employment_processed_at=now(), employment_processing_error=NULL, employment_retry_count=0 WHERE contact_id=$%d AND claim_key IN ('current_employment','job_history')`, len(args)), args...)
			return err
		}
		return nil
	})
	if err != nil {
		return crmcontracts.EmploymentImportReport{}, err
	}
	return s.PreviewEmploymentImport(ctx, contact)
}

func (s *Store) applyEmploymentEpisode(ctx context.Context, tx pgx.Tx, contact ids.ContactID, e employmentEvidence, req crmcontracts.EmploymentImportRequest, action string) error {
	prior, err := employmentImportItem(ctx, tx, contact, e)
	if err != nil {
		return err
	}
	if prior.State == actionDismissed {
		return recordEmploymentOutcome(ctx, tx, contact, e, actionDismissed, idArg[ids.CompanyKind](prior.CompanyId), uuidArg(prior.RelationshipId))
	}
	if prior.State == employmentLinked {
		return retainEmploymentLink(ctx, tx, contact, e, req, action, prior)
	}

	if action == "dismiss" {
		return recordEmploymentOutcome(ctx, tx, contact, e, actionDismissed, nil, nil)
	}
	if e.invalidDates {
		return recordEmploymentOutcome(ctx, tx, contact, e, "needs_review", nil, nil)
	}
	company, err := s.resolveEmploymentCompany(ctx, tx, e, req, action)
	if err != nil {
		return err
	}
	if company == nil {
		return recordEmploymentOutcome(ctx, tx, contact, e, "needs_match", nil, nil)
	}
	capturedBy := connectorCapturedBy(e.provider)
	if action == employmentResolve {
		capturedBy, err = storekit.CapturedBy(ctx)
		if err != nil {
			return err
		}
	}
	relationship, state, err := s.linkEmploymentEpisode(ctx, tx, contact, *company, e, capturedBy)
	if err != nil {
		return err
	}
	return recordEmploymentOutcome(ctx, tx, contact, e, state, company, relationship)
}

func (s *Store) resolveEmploymentCompany(ctx context.Context, tx pgx.Tx, e employmentEvidence, req crmcontracts.EmploymentImportRequest, action string) (*ids.CompanyID, error) {
	if action == employmentResolve && req.CompanyId != nil {
		company := ids.From[ids.CompanyKind](ids.UUID(*req.CompanyId))
		if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, company.UUID); err != nil {
			return nil, err
		}
		return &company, nil
	}
	if action == employmentResolve && req.Domain != nil {
		domain, err := values.ParseDomain(strings.TrimSpace(*req.Domain))
		if err != nil {
			return nil, err
		}
		e.CompanyDomain = domain.String()
		e.trustedDomain = true
	}
	if e.CompanyDomain != "" {
		if err := storekit.LockWriteIdentity(ctx, tx, "company_domain", e.CompanyDomain); err != nil {
			return nil, err
		}
	}
	return s.matchEmploymentCompany(ctx, tx, e)
}

//nolint:nilnil // No resolved company is a successful needs-match outcome, not a failed import.
func (s *Store) matchEmploymentCompany(ctx context.Context, tx pgx.Tx, e employmentEvidence) (*ids.CompanyID, error) {
	matcher, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return nil, err
	}
	candidates := []string{}
	if e.CompanyDomain != "" {
		candidates = append(candidates, e.CompanyDomain)
	}
	outcome, err := resolveCompany(ctx, tx, ResolveCandidate{Kind: ResolveCompany, Name: e.CompanyName, Domains: candidates}, matcher)
	if err != nil {
		return nil, err
	}
	if len(outcome.Refs) == 1 && outcome.Refs[0].Exact {
		company := ids.From[ids.CompanyKind](outcome.Refs[0].ID)
		if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, company.UUID); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				return nil, nil
			}
			return nil, err
		}
		return &company, nil
	}
	// Name similarity and a URL-shaped company label are review evidence, not
	// authority to mint a company. Explicit provider domains or human-confirmed
	// websites may create after the same identity ladder found no candidate.
	if len(outcome.Refs) > 0 || !e.trustedDomain || e.CompanyDomain == "" {
		return nil, nil
	}
	if len(companyDomains(ResolveCandidate{Domains: []string{e.CompanyDomain}}, matcher)) == 0 {
		return nil, nil
	}
	company, err := s.CreateCompanyTx(ctx, tx, CreateCompanyInput{
		DisplayName: e.CompanyName, Source: e.provider,
		Domains: []CompanyDomainInput{{Domain: e.CompanyDomain, IsPrimary: true}},
	})
	if err != nil {
		return nil, fmt.Errorf("creating a resolved employer: %w", err)
	}
	id := ids.From[ids.CompanyKind](ids.UUID(company.Id))
	return &id, nil
}

func (s *Store) applyEmploymentEvidence(ctx context.Context, tx pgx.Tx, contact ids.ContactID, episodes []employmentEvidence, req crmcontracts.EmploymentImportRequest, action string) error {
	activeCurrent := currentEmploymentKeys(episodes)
	var today time.Time
	if err := tx.QueryRow(ctx, `SELECT current_date`).Scan(&today); err != nil {
		return err
	}
	group := employmentGroup(episodes, req, action)
	selected := req.Key == nil
	processed := map[string]bool{}
	for _, e := range episodes {
		if !employmentSelected(e, req.Key, group) {
			continue
		}
		if activeCurrent[e.key] {
			e.superseded = false
		}
		selected = true
		if processed[e.key] {
			if err := copyEmploymentOutcome(ctx, tx, contact, e); err != nil {
				return err
			}
			continue
		}
		processed[e.key] = true
		if action == employmentResolve && req.Key != nil && *req.Key == e.key {
			if err := e.correct(req); err != nil {
				return err
			}
		}

		e.invalidDates = e.requiresReview(today)
		e.Status = e.effectiveStatus(today)

		if err := s.applyEmploymentEpisode(ctx, tx, contact, e, req, action); err != nil {
			return err
		}
	}
	if !selected {
		return apperrors.ErrNotFound
	}
	return nil
}

func employmentGroup(episodes []employmentEvidence, req crmcontracts.EmploymentImportRequest, action string) string {
	group := ""
	if action == employmentResolve && req.ResolveGroup != nil && *req.ResolveGroup {
		for _, e := range episodes {
			if req.Key != nil && e.key == *req.Key {
				group = e.companyIdentity()
			}
		}
	}
	return group
}

func employmentImportAction(req crmcontracts.EmploymentImportRequest) (string, error) {
	if err := validEmploymentAssertion(employmentKind, enumArg(req.EmploymentStatus), nil, nil); err != nil {
		return "", err
	}
	action := employmentApply
	if req.Action != nil {
		action = string(*req.Action)
	}
	if action != employmentApply && action != employmentResolve && action != "dismiss" {
		return "", &EmploymentImportError{Field: employmentActionField, Message: "Choose apply, resolve or dismiss."}
	}
	if action != employmentApply && (req.Key == nil || *req.Key == "") {
		return "", &RequiredFieldError{Field: employmentKeyField}
	}
	return action, nil
}

func retainEmploymentLink(ctx context.Context, tx pgx.Tx, contact ids.ContactID, e employmentEvidence, req crmcontracts.EmploymentImportRequest, action string, prior crmcontracts.EmploymentImportItem) error {
	if e.superseded && prior.RelationshipId != nil {
		if err := retireSupersededEmployment(ctx, tx, ids.UUID(*prior.RelationshipId), e.provider); err != nil {
			return err
		}
	}
	if action != employmentApply && (req.Key == nil || e.key == *req.Key) {
		return apperrors.ErrVersionSkew
	}
	return recordEmploymentOutcome(ctx, tx, contact, e, employmentLinked, idArg[ids.CompanyKind](prior.CompanyId), uuidArg(prior.RelationshipId))
}

func currentEmploymentKeys(episodes []employmentEvidence) map[string]bool {
	activeCurrent := map[string]bool{}
	for _, e := range episodes {
		if !e.superseded && e.Status == employmentCurrent {
			activeCurrent[e.key] = true
		}
	}
	return activeCurrent
}

func employmentSelected(e employmentEvidence, key *string, group string) bool {
	return key == nil || *key == e.key || (group != "" && group == e.companyIdentity())
}
