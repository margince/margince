// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func foldDeals(view crmcontracts.Company360, in *Input) {
	if view.Deals == nil {
		return
	}
	lost := view.Deals.LostCount
	in.LostCount = &lost
	if view.Deals.WonLifetime.AmountMinor != nil && view.Deals.WonLifetime.Currency != nil {
		in.WonCurrency = *view.Deals.WonLifetime.Currency
		in.WonLifetime = *view.Deals.WonLifetime.AmountMinor
	}
	for _, deal := range view.Deals.Data {
		d := DealIn{ID: deal.DealId.String(), Name: deal.Name, Stalled: deal.Stalled}
		if deal.StageName != nil {
			d.Stage = *deal.StageName
		}
		// Both halves or neither. Without a currency the amount has no scale.
		if deal.Amount != nil && deal.Amount.AmountMinor != nil && deal.Amount.Currency != nil {
			d.Currency = *deal.Amount.Currency
			d.AmountMinor = *deal.Amount.AmountMinor
		}
		in.OpenDeals = append(in.OpenDeals, d)
	}
}

// foldOpenPipeline reads the open total off the same state strip the card
// shows, so the check and the card cannot disagree on it.
func foldOpenPipeline(view crmcontracts.Company360, in *Input) {
	if view.StateStrip == nil || view.StateStrip.Commercial == nil {
		return
	}
	commercial := view.StateStrip.Commercial
	if commercial.OpenPipelineMinorBase != nil && commercial.BaseCurrency != nil {
		in.OpenPipelineMinor = int64(*commercial.OpenPipelineMinorBase)
		in.OpenPipelineCurrency = *commercial.BaseCurrency
	}
}
