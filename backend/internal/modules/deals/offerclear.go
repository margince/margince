// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// Which of an offer's header fields a null can empty.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// validUntilColumn is named because the clear map, the header patch and the
// expiry read all spell it.
const validUntilColumn = "valid_until"

// clearableOfferColumns are the offer header's nullable columns, by request
// field.
//
// `currency` is absent because it is NOT NULL. An offer is priced in some
// currency or it is not an offer, and a repricing goes through the currency
// change that states its consequence.
//
// Clearing the buyer does not retire the rendering the way a buyer CHANGE
// does. A rendering names the buyer it was made for, and there is no new buyer
// for it to disagree with.
func clearableOfferColumns(current crmcontracts.Offer) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"buyer_company_id": {Column: "buyer_company_id", Current: current.BuyerCompanyId},
		"intro_text":       {Column: "intro_text", Current: current.IntroText},
		"terms_text":       {Column: "terms_text", Current: current.TermsText},
		"template_id":      {Column: "template_id", Current: current.TemplateId},
		validUntilColumn:   {Column: validUntilColumn, Current: current.ValidUntil},
	}
}
