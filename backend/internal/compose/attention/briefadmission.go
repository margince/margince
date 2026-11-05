// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// Ranking and cached prose do not establish a current intervention. Only facts
// read for this reader admit an automatic suggestion; the stored brief remains reviewable.
func admittedBriefItems(day crmcontracts.Attention, asOf time.Time) []crmcontracts.AttentionItem {
	current := map[string]crmcontracts.AttentionItem{}
	if day.AtRisk != nil {
		for _, item := range *day.AtRisk {
			if item.Subject != nil {
				current[item.Subject.Id.String()] = item
			}
		}
	}
	admitted := make([]crmcontracts.AttentionItem, 0, len(day.ThisMorning))
	for _, item := range day.ThisMorning {
		if item.Subject == nil || item.Subject.Type != subjectDeal {
			continue
		}
		if observed, ok := current[item.Subject.Id.String()]; ok {
			if observed.Deal != nil {
				facts := crmcontracts.AttentionDealFacts{}
				if item.Deal != nil {
					facts = *item.Deal
				}
				facts.NoNextStep = observed.Deal.NoNextStep
				facts.NoChampion = observed.Deal.NoChampion
				facts.ChampionUnknown = observed.Deal.ChampionUnknown
				item.Deal = &facts
			}
			item.QuietDays = observed.QuietDays
			item.SubjectMovedAt = observed.SubjectMovedAt
			item.DueAt = observed.DueAt
			item.Overdue = observed.Overdue
		}
		if !hasDealIntervention(item, asOf) {
			continue
		}
		admitted = append(admitted, item)
	}
	return admitted
}

func hasDealIntervention(item crmcontracts.AttentionItem, asOf time.Time) bool {
	if quietDaysOf(item) > 0 || recoveryDue(item, asOf) {
		return true
	}
	return item.Deal != nil && ((item.Deal.NoNextStep != nil && *item.Deal.NoNextStep) ||
		(item.Deal.NoChampion != nil && *item.Deal.NoChampion))
}
