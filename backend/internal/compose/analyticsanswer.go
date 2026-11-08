// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

func analyticsAnswerWire(answer AnalyticsAnswer) crmcontracts.AnalyticsAnswer {
	out := crmcontracts.AnalyticsAnswer{
		Columns: answer.Columns, Rows: answer.Rows,
		Withheld: answer.Withheld, TotalSafe: answer.TotalSafe,
		SchemaVersion:        answer.SchemaVersion,
		PopulationNarrowed:   populationNarrowedWire(answer.PopulationNarrowed),
		ExcludedByPermission: answer.ExcludedByPermission,
	}
	if answer.BaseCurrency != "" {
		out.BaseCurrency = &answer.BaseCurrency
	}
	if !answer.RateDate.IsZero() {
		out.RateDate = &answer.RateDate
	}
	return out
}

func keptAnalyticsAnswer(answer crmcontracts.AnalyticsAnswer) crmcontracts.AnalyticsAnswer {
	answer.ExcludedByPermission = nil
	return answer
}
