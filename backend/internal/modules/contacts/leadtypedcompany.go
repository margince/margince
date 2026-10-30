// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// leadPromotionSource names the lead as where a contact's company_name fact came from.
const leadPromotionSource = "lead_promotion"

// promotionEmployer is the employer evidence a promotion hands the shared
// resolver, and the typed company name it holds back. Typed text alone never
// creates a company: the name rides along only beside a linked company, a
// business email domain, or the caller's createCompany.
func (s *Store) promotionEmployer(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, createCompany bool,
) (EmployerChoice, string, error) {
	choice := EmployerChoice{
		CompanyName: lead.CompanyName, CompanyID: idArg[ids.CompanyKind](lead.CompanyId),
		Role: lead.Title, Source: lead.Source,
	}
	if choice.CompanyID != nil || !choice.named() || createCompany {
		return choice, "", nil
	}
	if lead.Email != nil {
		domains, err := s.employerDomainsForAddressTx(ctx, tx, string(*lead.Email))
		if err != nil || len(domains) > 0 {
			return choice, "", err
		}
	}
	typed := strings.TrimSpace(*choice.CompanyName)
	choice.CompanyName = nil
	return choice, typed, nil
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
