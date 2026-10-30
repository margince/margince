// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The three work views of GET /worklist, and which filter values each admits.
// A view is a fetch predicate over the row's category, applied before the cut.

import (
	"context"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

const (
	viewToday     = string(crmcontracts.WorklistViewToday)
	viewDecisions = string(crmcontracts.WorklistViewDecisions)
	viewSystem    = string(crmcontracts.WorklistViewSystem)

	categoryUpcoming = crmcontracts.WorklistItemCategoryUpcoming
)

// viewOfFilter names the view a filter value belongs to. `all` belongs to every
// view, so it answers empty. Every other value is Today work.
func viewOfFilter(filter string) string {
	switch crmcontracts.WorklistFilter(filter) {
	case "", crmcontracts.WorklistFilterAll:
		return ""
	case crmcontracts.WorklistFilterDecisions:
		return viewDecisions
	case crmcontracts.WorklistFilterSystem:
		return viewSystem
	default:
		return viewToday
	}
}

// resolveView reads `filter=decisions|system` without a view as that view, and
// refuses a filter sent with a view it does not belong to. An empty list would
// tell the reader the view is clear when the question was malformed.
func resolveView(view, filter string) (string, error) {
	home := viewOfFilter(filter)
	if view == "" {
		if home == viewDecisions || home == viewSystem {
			return home, nil
		}
		return "", nil
	}
	if home != "" && home != view {
		return "", httperr.Validation("filter", "filter_outside_view",
			fmt.Sprintf("filter %q belongs to the %s view; send it with view=%s or without a view", filter, home, home))
	}
	return view, nil
}

// inView says whether a row belongs to a view. No view keeps every row.
func inView(row ranked, view string) bool {
	switch view {
	case viewDecisions:
		return row.item.Category == categoryDecisions
	case viewSystem:
		return row.item.Category == categorySystem
	case viewToday:
		return row.item.Category != categoryDecisions && row.item.Category != categorySystem
	default:
		return true
	}
}

// keepInView narrows rows to a view, and drops upcoming work unless the filter
// asks for it. Upcoming work is a section of its own and never a due row.
func keepInView(rows []ranked, view, filter string) []ranked {
	wantUpcoming := filter == string(categoryUpcoming)
	kept := make([]ranked, 0, len(rows))
	for _, row := range rows {
		if inView(row, view) && (row.item.Category == categoryUpcoming) == wantUpcoming {
			kept = append(kept, row)
		}
	}
	return kept
}

// outstanding is the population the summary counts: the view's rows that are
// due now, without upcoming work.
func outstanding(rows []ranked, view string) []ranked {
	return keepInView(rows, view, "")
}

// askedFor is the filter half of a cursor fingerprint. It binds the view, and
// collapses a filter that only repeats its view, so an alias and the view it
// opens share one walk.
func askedFor(view, filter string) string {
	if view == "" {
		return filter
	}
	if filter == view || filter == string(crmcontracts.WorklistFilterAll) {
		filter = ""
	}
	return view + "/" + filter
}

// Worklist answers the ranked day with no view named. A filter narrows what
// the page carries, never what is read, so the counts describe the same day.
func (s *Service) Worklist(
	ctx context.Context, scope, filter string, owner ids.UUID, limit int, token string,
) (crmcontracts.Worklist, error) {
	return s.WorklistView(ctx, "", scope, filter, owner, limit, token)
}

// WorklistView answers one work view of the ranked day. An empty view is the
// whole day, except that a decisions or system filter opens its view.
func (s *Service) WorklistView(
	ctx context.Context, view, scope, filter string, owner ids.UUID, limit int, token string,
) (crmcontracts.Worklist, error) {
	view, err := resolveView(view, filter)
	if err != nil {
		return crmcontracts.Worklist{}, err
	}
	// Refused before the snapshot, so a refused caller costs no pooled
	// connection. One snapshot then holds every lane read at one instant.
	if err := auth.RequireMember(ctx); err != nil {
		return crmcontracts.Worklist{}, err
	}
	var out crmcontracts.Worklist
	viewing := *s
	viewing.view = view
	err = s.inSnapshot(ctx, ReadWorklist, func(ctx context.Context) error {
		var err error
		out, err = viewing.worklistIn(ctx, scope, filter, owner, limit, token)
		return err
	})
	if err == nil {
		err = ctx.Err() // a caller who left gets no page assembled from failed lanes
	}
	return out, err
}

// echoAsked tells the client which narrowing and which view this page answers.
func (s *Service) echoAsked(out *crmcontracts.Worklist, filter string) {
	if filter != "" {
		out.Filter = new(crmcontracts.WorklistFilter(filter))
	}
	if s.view != "" {
		out.View = new(crmcontracts.WorklistView(s.view))
	}
}
