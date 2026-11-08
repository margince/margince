// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	extractKindOpportunity = "new_opportunity"
	commercialRoleBuyer    = "buyer"
	commercialRoleSupplier = "supplier"
	commercialRolePartner  = "partner"
	commercialRoleProvider = "provider"
	commercialRoleUnknown  = "unknown"
)

func opportunityBuyer(event extractedEvent, thread settledThread) (*ids.UUID, string) {
	if event.Kind != extractKindOpportunity {
		return nil, ""
	}
	switch event.CommercialRole {
	case commercialRoleSupplier, commercialRolePartner, commercialRoleProvider:
		return nil, event.CommercialRole
	case commercialRoleBuyer:
		for _, message := range thread.Messages {
			if message.ID.String() != event.MessageID {
				continue
			}
			if _, quoted := evidenceSpan(textlang.CurrentMessage(message.Body), event.Quote); !quoted || event.Quote == "" {
				return nil, commercialRoleUnknown
			}
			party, verified := referencedPromiser(event.BuyerRef, message.Parties)
			if verified && party.Contact != nil {
				return party.Contact, commercialRoleBuyer
			}
		}
	}
	return nil, commercialRoleUnknown
}

func validateOpportunityBuyer(event extractedEvent, thread settledThread) string {
	if event.Kind != extractKindOpportunity || event.CommercialRole != commercialRoleBuyer || event.BuyerRef == "" {
		return ""
	}
	if buyer, _ := opportunityBuyer(event, thread); buyer == nil {
		return "a buyer must be a verified external contact with a quoted request for our company’s work, never our own seat"
	}
	return ""
}

func opportunityCompany(ctx context.Context, tx pgx.Tx, buyer *ids.UUID, source ids.UUID) (ids.UUID, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return ids.Nil, err
	}
	if buyer == nil {
		return ids.Nil, nil
	}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	query := fmt.Sprintf(`SELECT DISTINCT employer.company_id FROM relationship employer
 JOIN activity source ON source.id = $%d
 JOIN company buyer_company ON buyer_company.id = employer.company_id
  AND buyer_company.archived_at IS NULL AND NOT buyer_company.is_anchor
 WHERE employer.contact_id = $%d AND `+buyerEmployment("employer", "source"), arg(source), arg(*buyer))
	rows, err := tx.Query(ctx, query, args...)
	if err != nil {
		return ids.Nil, err
	}
	companies, err := pgx.CollectRows(rows, pgx.RowTo[ids.UUID])
	if err != nil {
		return ids.Nil, err
	}
	if len(companies) != 1 {
		return ids.Nil, nil
	}
	if err := auth.EnsureVisible(ctx, tx, "company", companies[0]); err != nil {
		return ids.Nil, err
	}
	return companies[0], nil
}
