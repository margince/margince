// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exportedIDs is the slice an export reads: the rows its filter matches, or a
// Shortlist's members, through the same scope-forcing executor either way. It
// is not bounded by the list page limit, since an export is the surface a
// larger slice is read through, but by ExportRowLimit: a slice past it is
// refused, never cut short.
func exportedIDs(ctx context.Context, tx pgx.Tx, engine storekit.Query, src collections.FilterSource) ([]ids.UUID, error) {
	var matched []ids.UUID
	var err error
	if src.Members != nil {
		matched, err = engine.SelectExportMemberIDs(ctx, tx, src.Members)
	} else {
		matched, err = engine.SelectExportIDs(ctx, tx, src.Predicate)
	}
	if err == nil && len(matched) > storekit.ExportRowLimit {
		return nil, &exportBadRequest{
			field:  logDetailFilter,
			reason: fmt.Sprintf("matches more than %d rows, which is more than one export holds; narrow the filter and export in slices", storekit.ExportRowLimit),
		}
	}
	return matched, err
}

// exportedSlice is how the export's log row names its slice: the filter, or
// that a Shortlist's members were read.
//
//craft:ignore naked-any the log detail holds either a predicate tree or a marker string
func exportedSlice(src collections.FilterSource) any {
	if src.Members != nil {
		return "shortlist_members"
	}
	return src.Predicate
}

// withListSource adds the list an export read, when it read one, to the
// export's log detail.
func withListSource(list *ids.ListID, detail map[string]any) map[string]any {
	if list != nil {
		detail[collections.ExportDetailListID] = list.String()
	}
	return detail
}
