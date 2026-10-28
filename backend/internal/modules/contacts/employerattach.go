// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// EmployerChoice is the employer a write attaches to a contact: an existing
// company, or a name to create one under. Neither set attaches nothing.
type EmployerChoice struct {
	CompanyID   *ids.CompanyID
	CompanyName *string
	Role        *string
	Source      string
	// Domains go on a company this choice creates, and nowhere else.
	Domains []CompanyDomainInput
}

// EmployerAttached is what attachEmployerTx wrote.
type EmployerAttached struct {
	CompanyID      ids.CompanyID
	CompanyCreated bool
	RelationshipID ids.UUID
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

// refuseAgentEmployer keeps a company out of an agent's contact create, where
// it would land beside a contact a human approved without seeing it.
func refuseAgentEmployer(ctx context.Context, choice EmployerChoice) error {
	if !choice.named() {
		return nil
	}
	if actor, ok := principal.Actor(ctx); !ok || actor.Type != principal.PrincipalAgent {
		return nil
	}
	field := fieldCompanyName
	if choice.CompanyID != nil {
		field = endCompanyColumn
	}
	return &values.ParseError{
		Field: field, Code: "employer_needs_a_human",
		Message: "an agent names a contact's employer with a relationship write, not with a contact create",
	}
}

// attachEmployerTx is the one writer of "this contact works there now". It
// creates the company from the name when no id was given, then writes the edge
// through CreateRelationshipTx, whose gates it keeps. An id wins over a name.
func (s *Store) attachEmployerTx(
	ctx context.Context, tx pgx.Tx, contactID ids.ContactID, choice EmployerChoice,
) (EmployerAttached, error) {
	var out EmployerAttached
	if choice.CompanyID != nil {
		out.CompanyID = *choice.CompanyID
	} else {
		company, err := s.CreateCompanyTx(ctx, tx, CreateCompanyInput{
			DisplayName: strings.TrimSpace(deref(choice.CompanyName)),
			Domains:     choice.Domains,
			Source:      choice.Source,
		}, CustomColumns{})
		if err != nil {
			return out, err
		}
		out.CompanyID = ids.From[ids.CompanyKind](ids.UUID(company.Id))
		out.CompanyCreated = true
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
	out.RelationshipID = edge.ID
	return out, nil
}
