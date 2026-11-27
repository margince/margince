// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contracts

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
)

// setContractFields assigns every field the request named, which is a list
// rather than a decision: the clearing above is what reads the nulls.
func setContractFields(patch *storekit.Patch, existing crmcontracts.Contract, in crmcontracts.UpdateContractRequest) {
	if in.DealId != nil {
		patch.Set("deal_id", existing.DealId, *in.DealId)
	}
	if in.ProjectId != nil {
		patch.Set("project_id", existing.ProjectId, *in.ProjectId)
	}
	if in.ContractNumber != nil {
		patch.Set("contract_number", existing.ContractNumber, *in.ContractNumber)
	}
	if in.Title != nil {
		patch.Set("title", existing.Title, *in.Title)
	}
	if in.ValueMinor != nil {
		patch.Set("value_minor", existing.ValueMinor, *in.ValueMinor)
	}
	if in.ArrMinor != nil {
		patch.Set("arr_minor", existing.ArrMinor, *in.ArrMinor)
	}
	if in.Currency != nil {
		patch.Set("currency", existing.Currency, *in.Currency)
	}
	if in.ValueBasis != nil {
		patch.Set("value_basis", string(existing.ValueBasis), string(*in.ValueBasis))
	}
	setContractTermFields(patch, existing, in)
}

// setContractTermFields assigns the dates and the renewal terms, which is
// where most of the contract's fields are.
func setContractTermFields(patch *storekit.Patch, existing crmcontracts.Contract, in crmcontracts.UpdateContractRequest) {
	if in.StartsOn != nil {
		patch.SetDate(startsOnColumn, storekit.PlainDate(existing.StartsOn), storekit.PlainDate(in.StartsOn))
	}
	if in.EndsOn != nil {
		patch.SetDate(endsOnColumn, storekit.PlainDate(existing.EndsOn), storekit.PlainDate(in.EndsOn))
	}
	if in.RenewalOn != nil {
		patch.SetDate("renewal_on", storekit.PlainDate(existing.RenewalOn), storekit.PlainDate(in.RenewalOn))
	}
	if in.AutoRenew != nil {
		patch.Set("auto_renew", existing.AutoRenew, *in.AutoRenew)
	}
	if in.NoticePeriodDays != nil {
		patch.Set("notice_period_days", existing.NoticePeriodDays, *in.NoticePeriodDays)
	}
	if in.PaymentTermDays != nil {
		patch.Set("payment_term_days", existing.PaymentTermDays, *in.PaymentTermDays)
	}
	if in.SignedOn != nil {
		patch.SetDate("signed_on", storekit.PlainDate(existing.SignedOn), storekit.PlainDate(in.SignedOn))
	}
}
