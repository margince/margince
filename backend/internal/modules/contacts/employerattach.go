// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// EmployerChoice is the employer a write attaches to a contact: an existing
// company, or a name to match before creating one.
type EmployerChoice struct {
	CompanyID   *ids.CompanyID
	CompanyName *string
	Role        *string
	Source      string
	// Domains help match an employer and are written only when creating one.
	Domains []CompanyDomainInput
}

// EmployerAttached is what attachEmployerTx wrote.
type EmployerAttached struct {
	CompanyID      ids.CompanyID
	CompanyCreated bool
	edge           relationshipRow
}

// named reports whether the choice names an employer at all. A blank name is a
// box the reader left alone, never a company called "".
func (c EmployerChoice) named() bool {
	return c.CompanyID != nil || strings.TrimSpace(deref(c.CompanyName)) != ""
}

// employerChoiceFromRequest maps a request's company_id / company_name pair,
// refusing a request that sends both.
func employerChoiceFromRequest(companyID *ids.CompanyID, companyName *string, source string) (EmployerChoice, error) {
	if companyID != nil && companyName != nil {
		return EmployerChoice{}, httperr.Validation(fieldCompanyName, "invalid",
			"send company_id or company_name, not both")
	}
	return EmployerChoice{CompanyID: companyID, CompanyName: companyName, Source: source}, nil
}

// attachEmployerTx writes a contact's employment edge through CreateRelationshipTx,
// whose gates it keeps. An explicit id wins over a name.
func (s *Store) attachEmployerTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, choice EmployerChoice,
) (EmployerAttached, error) {
	var out EmployerAttached
	companyID, created, err := s.resolveNamedEmployer(ctx, tx, choice)
	if err != nil {
		return out, err
	}
	out.CompanyID, out.CompanyCreated = companyID, created
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return out, err
	}
	if err := lockEmploymentClocks(ctx, tx, CreateRelationshipInput{Kind: employmentKind, ContactID: &contactID, CompanyID: &companyID}); err != nil {
		return out, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, contactEntity, contactID.UUID); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, companyID.UUID); err != nil {
		return out, auth.NameMissingReference(endCompanyColumn, err)
	}
	if _, err := contactIsSeatTx(ctx, tx, contactID.UUID); err != nil {
		return out, err
	}
	if err := storekit.LockWriteIdentity(ctx, tx, employmentKind, contactID.String()); err != nil {
		return out, err
	}
	args := []any{contactID, companyID}
	var edgeID ids.UUID
	err = tx.QueryRow(ctx, storekit.SQLf(`SELECT id FROM relationship r
  WHERE contact_id = $%d AND company_id = $%d AND `+employment.LiveSlotSQL("r"), len(args)-1, len(args)), args...).Scan(&edgeID)
	if err == nil {
		out.edge, err = s.visibleRelationship(ctx, tx, edgeID)
		return out, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return out, err
	}
	edge, err := s.CreateRelationshipTx(ctx, tx, CreateRelationshipInput{
		ReferencesFromRequest: true,
		Kind:                  employmentKind,
		ContactID:             &contactID,
		CompanyID:             &out.CompanyID,
		Role:                  choice.Role,
		Source:                choice.Source,
	})
	if err != nil {
		return out, err
	}
	out.edge = edge
	return out, nil
}

// resolveNamedEmployer serializes the match and create with every company-name writer.
func (s *Store) resolveNamedEmployer(ctx context.Context, tx pgx.Tx, choice EmployerChoice) (ids.CompanyID, bool, error) {
	if choice.CompanyID != nil {
		return *choice.CompanyID, false, nil
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return ids.CompanyID{}, false, err
	}
	if err := lockCompanyNameWrites(ctx, tx); err != nil {
		return ids.CompanyID{}, false, err
	}
	found, err := matchNamedEmployer(ctx, tx, choice)
	if err != nil {
		return ids.CompanyID{}, false, err
	}
	if found != nil {
		return *found, false, nil
	}
	company, err := s.CreateCompanyTx(ctx, tx, CreateCompanyInput{DisplayName: strings.TrimSpace(deref(choice.CompanyName)), Domains: choice.Domains, Source: choice.Source}, CustomColumns{})
	if err != nil {
		return ids.CompanyID{}, false, err
	}
	return ids.From[ids.CompanyKind](ids.UUID(company.Id)), true, nil
}

func matchNamedEmployer(ctx context.Context, tx pgx.Tx, choice EmployerChoice) (*ids.CompanyID, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	if choice.CompanyID != nil {
		if err := auth.EnsureLinkTarget(ctx, tx, companyEntity, choice.CompanyID.UUID); err != nil {
			return nil, auth.NameMissingReference(endCompanyColumn, err)
		}
		return choice.CompanyID, nil
	}
	name := strings.TrimSpace(deref(choice.CompanyName))
	domains := make([]string, 0, len(choice.Domains))
	for _, domain := range choice.Domains {
		domains = append(domains, domain.Domain)
		if base, ok := freemail.Hostname(domain.Domain); ok {
			domains = append(domains, base)
		}
	}
	match, err := DedupeCompany(ctx, tx, CompanyCandidate{DisplayName: name, Domains: domains})
	if err != nil {
		return nil, err
	}
	if match.DomainSplit != nil {
		return nil, httperr.Validation(fieldCompanyName, "ambiguous", "the supplied domains name different companies; choose the employer")
	}
	found, err := exactEmployerName(ctx, tx, name)
	if err != nil {
		return nil, err
	}
	if match.Decision == DecisionExactCollision {
		if found != nil && *found != match.CompanyID {
			return nil, httperr.Validation(fieldCompanyName, "conflict", "the company name and work domain name different employers; choose the employer")
		}
		return &match.CompanyID, nil
	}
	if found != nil {
		return found, nil
	}
	return nil, nil //nolint:nilnil // No matching employer is an ordinary lookup result.
}

func exactEmployerName(ctx context.Context, tx pgx.Tx, name string) (*ids.CompanyID, error) {
	matches, err := EmployerNameMatchesTx(ctx, tx, name)
	if err != nil {
		return nil, err
	}
	if len(matches) > 1 {
		return nil, httperr.Validation(fieldCompanyName, "ambiguous", "more than one company has this name; choose the employer")
	}
	if len(matches) == 1 {
		return &matches[0], nil
	}
	return nil, nil //nolint:nilnil // No matching employer is an ordinary lookup result.
}

// EmployerNameMatchesTx keeps name matching identical in import previews and employer writes.
func EmployerNameMatchesTx(ctx context.Context, tx pgx.Tx, name string) ([]ids.CompanyID, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{name}
	namePos := len(args)
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, companyEntity, "o", arg)
	if err != nil {
		return nil, err
	}
	if scope == "" {
		scope = scopeAllRows
	}
	rows, err := tx.Query(ctx, storekit.SQLf(`SELECT o.id FROM company o
 WHERE o.archived_at IS NULL AND o.merged_into_id IS NULL AND %s
 AND (f_fold_import_name(o.display_name) = f_fold_import_name($%d)
 OR f_fold_import_name(o.legal_name) = f_fold_import_name($%d)) ORDER BY o.id LIMIT 2`, scope, namePos, namePos), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var matches []ids.CompanyID
	for rows.Next() {
		var id ids.CompanyID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		matches = append(matches, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("resolve employer name: %w", err)
	}
	return matches, nil
}
