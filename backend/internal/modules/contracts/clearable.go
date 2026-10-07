// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

// Which of an agreement's fields a caller may set back to NOTHING.
//
// Its own file for the reason the deal's twin is: "what can this record forget"
// is a question a reader asks whole. Applying a clear is storekit's.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// clearableContractColumns names the wire fields whose clear this store
// honours.
//
// deal_id is here because a human is TOLD to use it. A deal and its agreements
// must name the same company, so moving a deal to another company is refused
// while agreements filed against it still name the old one — and detaching them
// is the move that refusal names. Without this, that advice would be advice
// with nothing behind it: the null decoded to a nil pointer, the patch read it
// as "not supplied", and the caller got a 200 that changed nothing.
//
// The other nullable fields are absent, and absent is not the same as refused
// twice: a null on one of them now answers 422 naming the field, which is what
// every other record type in this tree already answers. They are added when
// something asks to forget them — company_id never, because an agreement with
// no counterparty is not an agreement, and the currency and value pair never
// through this path, because a figure without its unit is what the reprice
// guards exist to refuse.
func clearableContractColumns(existing crmcontracts.Contract) map[string]storekit.Clearable {
	return map[string]storekit.Clearable{
		"deal_id": {Column: "deal_id", Current: existing.DealId},
	}
}
