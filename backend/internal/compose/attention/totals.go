// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exactSources maps each source a store counted in full, over the rows this
// page admits, to that count. The approvals count uses the lane's own query.
// narrowToScope applies the same requested-decider rule in every scope but
// team. The team scope judges by a roster the lane query does not share, so
// there the count is not offered.
func (s *Service) exactSources(day crmcontracts.Attention, scope string) map[crmcontracts.WorklistItemSource]int {
	exact := map[crmcontracts.WorklistItemSource]int{}
	if day.Counts.ApprovalsOpen != nil && (s.taskOwner != ids.Nil || scope != scopeTeam) {
		exact[crmcontracts.WorklistItemSourceApproval] = *day.Counts.ApprovalsOpen
	}
	return exact
}

// withTotals states each category's exact total where every source behind it
// is whole. Whole means read to the end, or read short with a store's count.
// Three things leave the total absent: a short source with no count, a lane
// that never answered, and a scope cut at its roster. Each hides items no
// figure here can name.
func withTotals(
	counts []crmcontracts.WorklistCount, considered []ranked, bounds map[crmcontracts.WorklistItemSource]bool,
	exact map[crmcontracts.WorklistItemSource]int, missing []crmcontracts.WorklistSourceUnavailable, scoped scopeNote,
) []crmcontracts.WorklistCount {
	if scoped.truncated || scoped.failed {
		return counts
	}
	unread, known := unreadCategories(missing)
	if !known {
		return counts
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

// unreadCategories names the categories a missing lane would have put rows in.
// known is false when one such lane names no category, so none can be trusted.
func unreadCategories(missing []crmcontracts.WorklistSourceUnavailable) (unread map[crmcontracts.WorklistCountCategory]bool, known bool) {
	unread = map[crmcontracts.WorklistCountCategory]bool{}
	for _, source := range missing {
		if source.ContributesRows != nil && !*source.ContributesRows {
			continue
		}
		if source.Category == nil {
			return nil, false
		}
		unread[crmcontracts.WorklistCountCategory(*source.Category)] = true
	}
	return unread, true
}
