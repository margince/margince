// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadPromotionSource names the lead as where a contact's company_name fact came from.
const leadPromotionSource = "lead_promotion"

// promotionEmployer is the employer evidence a promotion hands the shared
// resolver, and the typed company name it holds back. Typed text alone never
// creates a company: the name rides along only beside a linked company, a
// business email domain, or the caller's createsCompany. The facts it decided
// on come back too, so an approval can pin them.
func (s *Store) promotionEmployer(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, createsCompany bool,
) (EmployerChoice, string, promotionEmployerFacts, error) {
	facts, err := s.promotionEmployerFactsTx(ctx, tx, lead, createsCompany)
	if err != nil {
		return EmployerChoice{}, "", facts, err
	}
	choice := EmployerChoice{
		CompanyName: lead.CompanyName, CompanyID: idArg[ids.CompanyKind](lead.CompanyId),
		Role: lead.Title, Source: lead.Source,
	}
	if choice.CompanyID != nil || !choice.named() || createsCompany {
		return choice, "", facts, nil
	}
	if facts.Domain != nil {
		domains, err := employerDomainsVisible(ctx, tx, *facts.Domain)
		if err != nil || len(domains) > 0 {
			return choice, "", facts, err
		}
	}
	choice.CompanyName = nil
	return choice, facts.TypedName, facts, nil
}

// proposeTypedCompany puts a held-back company name on the contact as a
// company_name fact, which the contact page offers to link or create. A fact
// the contact already holds is kept.
func proposeTypedCompany(ctx context.Context, tx pgx.Tx, contactID ids.ContactID, leadID ids.LeadID, typed, by string) error {
	if typed == "" {
		return nil
	}
	_, err := writeProfileFieldRow(ctx, tx, contactID, contactProfileFieldRow{
		Field: fieldCompanyName, Value: typed, EvidenceSnippet: typed,
		SourceRef: "lead:" + leadID.String(), Source: leadPromotionSource, CapturedBy: by,
	}, claimUnanswered)
	return err
}
