// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exactSources names the sources a store counted exactly over the population
// this page admits, keyed to that count.
//
// The approvals count is taken under the lane's own query, and narrowToScope
// keeps an approval by the same requested-decider rule for every scope but
// team. The team narrowing keeps former teammates and judges by a roster the
// lane query does not share, so there the two populations differ and the count
// is not offered.
func (s *Service) exactSources(day crmcontracts.Attention, scope string) map[crmcontracts.WorklistItemSource]int {
	exact := map[crmcontracts.WorklistItemSource]int{}
	if day.Counts.ApprovalsOpen != nil && (s.taskOwner != ids.Nil || scope != scopeTeam) {
		exact[crmcontracts.WorklistItemSourceApproval] = *day.Counts.ApprovalsOpen
	}
	return exact
}

// withTotals states each category's exact total where every source behind it
// is whole: read to the end, or read short with an exact count from its store.
//
// A source short of its total with no exact count, a lane that never answered,
// or a scope cut at its roster leaves the total absent, because each of them
// hides items no figure here can name.
func withTotals(
	counts []crmcontracts.WorklistCount, considered []ranked, bounds map[crmcontracts.WorklistItemSource]bool,
	exact map[crmcontracts.WorklistItemSource]int, missing []crmcontracts.WorklistSourceUnavailable, scoped scopeNote,
) []crmcontracts.WorklistCount {
	if scoped.truncated || scoped.failed {
		return counts
	}
	unread := map[crmcontracts.WorklistCountCategory]bool{}
	for _, source := range missing {
		if source.ContributesRows != nil && !*source.ContributesRows {
			continue
		}
		if source.Category == nil {
			return counts
		}
		unread[crmcontracts.WorklistCountCategory(*source.Category)] = true
	}
	// Keyed by the row's own category, which countsOf counts under; a bounded
	// source with no row left is filed by categoryOfSource, as countsOf files it.
	type cut struct {
		source   crmcontracts.WorklistItemSource
		category crmcontracts.WorklistCountCategory
	}
	read := map[cut]int{}
	for _, row := range considered {
		read[cut{row.item.Source, crmcontracts.WorklistCountCategory(row.item.Category)}]++
	}
	for source := range bounds {
		at := cut{source, crmcontracts.WorklistCountCategory(categoryOfSource(source))}
		if _, seen := read[at]; !seen {
			read[at] = 0
		}
	}
	total := map[crmcontracts.WorklistCountCategory]int{}
	partial := map[crmcontracts.WorklistCountCategory]bool{}
	for at, rows := range read {
		switch whole, counted := exact[at.source]; {
		case !bounds[at.source]:
			total[at.category] += rows
		case counted:
			total[at.category] += whole
		default:
			partial[at.category] = true
		}
	}
	for i := range counts {
		category := counts[i].Category
		if partial[category] || unread[category] {
			continue
		}
		counts[i].Total = new(total[category])
	}
	return counts
}
