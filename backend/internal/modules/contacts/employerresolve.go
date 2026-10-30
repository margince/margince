// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// resolveCreatedEmployer also serves merges, which can bring employer evidence to an existing contact.
func (s *Store) resolveCreatedEmployer(ctx context.Context, tx pgx.Tx, id ids.ContactID, spec ContactSpec) error {
	choice := spec.Employer
	if choice.Source == "" {
		choice.Source = spec.Source
	}
	if choice.Role == nil {
		choice.Role = spec.Title
	}
	if choice.named() {
		return s.attachCreatedEmployer(ctx, tx, id, spec, choice)
	}
	return s.resolveInferredEmployer(ctx, tx, id, spec)
}

func (s *Store) attachCreatedEmployer(ctx context.Context, tx pgx.Tx, id ids.ContactID, spec ContactSpec, choice EmployerChoice) error {
	if choice.CompanyID == nil && len(choice.Domains) == 0 {
		domains, err := s.newEmployerDomainsTx(ctx, tx, id)
		if err != nil {
			return err
		}
		choice.Domains = domains
	}
	attached, err := s.attachEmployerTx(ctx, tx, id, choice)
	if err == nil && spec.EmployerAttached != nil {
		*spec.EmployerAttached = attached
	}
	return err
}

func (s *Store) resolveInferredEmployer(ctx context.Context, tx pgx.Tx, id ids.ContactID, spec ContactSpec) error {
	if !spec.newContact && spec.CaptureEmployer == nil {
		held, err := hasEmploymentHistory(ctx, tx, id)
		if err != nil || held {
			return err
		}
	}
	in := EnsureCounterpartyInput{Source: spec.Source, CapturedBy: spec.CapturedBy}
	if owner := spec.OwnerID; owner != nil {
		in.OwnerID = owner.UUID
	} else if owner := storekit.OwnerOrActor(ctx); owner != nil {
		in.OwnerID = owner.UUID
	}
	if spec.CaptureEmployer != nil {
		in = *spec.CaptureEmployer
	} else {
		domains, err := s.newEmployerDomainsTx(ctx, tx, id)
		if err != nil {
			return err
		}
		if len(domains) > 0 {
			in.Domain = domains[len(domains)-1].Domain
		}
	}
	result := EnsureCounterpartyResult{ContactID: id}
	if err := s.resolveDomainEmployer(ctx, tx, in, &result); err != nil {
		return err
	}
	if spec.EmployerAttached != nil && result.CompanyID != nil {
		spec.EmployerAttached.CompanyID = *result.CompanyID
	}
	if spec.EmployerResult != nil {
		spec.EmployerResult.CompanyID = result.CompanyID
		spec.EmployerResult.DomainSplit = result.DomainSplit
		spec.EmployerResult.TriagePending = result.TriagePending
		spec.EmployerResult.TriageDomain = result.TriageDomain
	}
	return nil
}

func (s *Store) resolveDomainEmployer(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, result *EnsureCounterpartyResult) error {
	if in.SuppressCompany || strings.TrimSpace(in.Domain) == "" {
		return nil
	}
	return s.ensureCompanyAndEmployment(ctx, tx, in, result)
}

// ResolveContactEmployer applies the same resolution to existing contacts and retries.
func (s *Store) ResolveContactEmployer(ctx context.Context, id ids.ContactID, choice EmployerChoice) error {
	return s.txRetryingLockCycles(ctx, func(tx pgx.Tx) error {
		_, err := s.ResolveContactEmployerTx(ctx, tx, id, choice)
		return err
	})
}

// ResolveContactEmployerTx lets an importer record created companies in its atomic undo ledger.
func (s *Store) ResolveContactEmployerTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, choice EmployerChoice) (EmployerAttached, error) {
	var out EmployerAttached
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, contactEntity, id.UUID); err != nil {
		return out, err
	}
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return out, err
	}
	err = s.resolveCreatedEmployer(ctx, tx, id, ContactSpec{Employer: choice, Source: choice.Source, CapturedBy: by, EmployerAttached: &out})
	return out, err
}

func currentEmployerID(ctx context.Context, tx pgx.Tx, id ids.ContactID) (*ids.CompanyID, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{id}
	var company ids.CompanyID
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT company_id FROM relationship r WHERE contact_id = $%d AND `+employment.CurrentPrimarySlotSQL("r"), len(args)), args...).Scan(&company)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil //nolint:nilnil // No current employer is an ordinary lookup result.
	}
	if err != nil {
		return nil, err
	}
	if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, company.UUID); err != nil {
		return nil, err
	}
	return &company, nil
}

// ensureCompanyAndEmployment reuses admitted companies or records a durable domain question.
func (s *Store) ensureCompanyAndEmployment(ctx context.Context, tx pgx.Tx, in EnsureCounterpartyInput, res *EnsureCounterpartyResult) error {
	consumer, err := s.consumerMailMatcher(ctx, tx)
	if err != nil || consumer.IsConsumer(in.Domain) {
		return err
	}
	if err := auth.Require(ctx, entityCompany, principal.ActionCreate); err != nil {
		return err
	}
	match, base, ok, err := employerMatchTx(ctx, tx, in.Domain)
	if err != nil || !ok {
		return err
	}
	res.DomainSplit = match.DomainSplit
	if match.Decision != DecisionExactCollision {
		// Unclaimed domains need classification before they become employers.
		return s.deferCompanyToTriage(ctx, tx, in, base, res)
	}
	companyID := match.CompanyID
	visible, err := auth.VisibleTo(ctx, tx, companyEntity, companyID.UUID)
	if err != nil || !visible {
		return err
	}
	if match.DomainSplit != nil {
		if _, err := enqueueDomainSplitTx(ctx, tx, *match.DomainSplit, in.Source, in.CapturedBy); err != nil {
			return err
		}
	}
	res.CompanyID = &companyID
	// A human put a company on this domain. That overrides whatever a crawl
	// concluded, a refusal included. The ledger records it as theirs, so the
	// next message stops re-asking and the trail says who overruled what.
	if err := adoptDispositionForCompany(ctx, tx, base, companyID); err != nil {
		return err
	}

	return plantEmploymentEdge(ctx, tx, in, base, res.ContactID, companyID)
}

// employerMatchTx is the company match a sender of this domain is filed under,
// shared by the ensure and the probe that asks about it first. ok is false
// when the domain is no hostname or is suppressed, and nothing is attached.
func employerMatchTx(ctx context.Context, tx pgx.Tx, domain string) (CompanyMatch, string, bool, error) {
	// Mail parsing accepts strings that are unsafe as crawl seeds or domain keys.
	base, ok := freemail.Hostname(domain)
	if !ok {
		return CompanyMatch{}, "", false, nil
	}
	// Suppression must precede matching, including domains with an existing company.
	suppressed, err := domainSuppressedTx(ctx, tx, base)
	if err != nil || suppressed {
		return CompanyMatch{}, "", false, err
	}
	match, err := DedupeCompany(ctx, tx, CompanyCandidate{Domains: []string{domain, base}})
	if err != nil {
		return CompanyMatch{}, "", false, err
	}
	return match, base, true, nil
}
