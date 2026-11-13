// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Previewing a filter nobody has saved yet (LVS-EXT-9).
//
// A filter builder shows a count that recomputes as clauses change, and until
// now nothing in the contract could evaluate the tree it recomputes for: the
// object list operations take flat scalar parameters and cannot express a tree,
// membership needs a stored list, and the filtered export is audit-logged — so
// driving a live recount through it would write an audit row per keystroke.
//
// It lives in compose rather than in collections for the same reason filtered
// export does: it needs the export's schema-derived projection as well as the
// collections store's engine, and a module may not import a sibling. Sharing
// that projection is also what makes the preview honest — the columns and values
// are the ones the JSON export writes for the same filter, so somebody deciding
// from a preview is deciding about the thing they would get.
//
// For somebody typing this writes nothing: no row, no audit entry, no outbox
// event. That is the property separating it from the export. A user's agent may call it too, and its preview writes one system_log
// row, because un-logged is wrong for an agent reading records in bulk.
//
// Why un-logged is defensible: GET /v1/contacts already returns the same
// Contact projection, 200 rows a page with a cursor, and writes no ledger row.
// Preview pages the same way at 100 rows, so its cursor opens no new channel.
// The export's system_log row records a file leaving the system. A preview
// makes no file, and reading records owes no row, or the list read would too.

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// logDetailFilter is the system_log detail key an agent's logged preview
// records its filter under, the key the filtered export's log uses too.
const logDetailFilter = "filter"

// filterPreviewDefaultRows is the page a caller who names no limit gets, and
// filterPreviewMaxRows the ceiling. Both are about what a builder renders while
// somebody types — a preview is a glance, not a report. The contract publishes
// the same two numbers.
const (
	filterPreviewDefaultRows = 25
	filterPreviewMaxRows     = 100
	// previewStatementBudget bounds each statement in a preview. The filter is
	// the caller's own, so the ceiling is the one database declares for that: a
	// glance that takes five seconds has already failed at being a glance.
	previewStatementBudget = database.CallerPredicateBudget
)

// filterPreviewHandlers shadows the generated PreviewFilter stub.
type filterPreviewHandlers struct {
	pool        *pgxpool.Pool
	collections *collections.Store
}

// filterPreviewRequest is the preview body, decoded here rather than through the
// generated type for one reason: the generated Filter is a map, and the engine
// takes a storekit.Predicate. Decoding straight into the predicate — exactly as
// filteredExportRequest does — keeps the tree typed from the wire inward instead
// of re-marshalling a map to get there.
type filterPreviewRequest struct {
	Resource string              `json:"resource"`
	Filter   *storekit.Predicate `json:"filter"`
	Limit    *int                `json:"limit"`
	Cursor   *string             `json:"cursor"`
}

func (h filterPreviewHandlers) PreviewFilter(w http.ResponseWriter, r *http.Request) {
	// Enforced here as well as at the transport: this returns records in bulk, so
	// it should not rest on route-pattern resolution alone.
	if err := auth.RequireHumanOrItsAgent(r.Context()); err != nil {
		httperr.Write(w, r, err)
		return
	}
	var req filterPreviewRequest
	if !httperr.Decode(w, r, &req) {
		return
	}
	if req.Filter == nil {
		httperr.Write(w, r, httperr.Validation("filter", codeInvalid,
			"a filter tree is required — preview evaluates a candidate filter, not the whole object"))
		return
	}
	limit, err := previewRowLimit(req.Limit)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	var token string
	if req.Cursor != nil {
		token = *req.Cursor
	}
	scope, after, err := previewCursor(req.Resource, *req.Filter, token)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	engine, ok, err := h.collections.SegmentEngine(r.Context(), req.Resource)
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	if !ok {
		httperr.Write(w, r, httperr.Validation("resource", codeInvalid,
			fmt.Sprintf("%q is not a filterable resource", req.Resource)))
		return
	}

	actor, _ := principal.Actor(r.Context())
	preview, err := h.preview(r.Context(), engine, *req.Filter, scope, after, limit, actor.Type == principal.PrincipalAgent)
	if err != nil {
		writeFilterPreviewError(w, r, err)
		return
	}
	preview.Resource = req.Resource
	httperr.WriteJSON(w, http.StatusOK, preview)
}

// preview runs the count, the page after `after` (nil for the first) and its
// rows in one snapshot (`REPEATABLE READ`). The count is the full match count on
// every page, so a paging table keeps one total.
//
// One snapshot because the row scope is decided in the page's statement and the
// rows are read in a later one. Under `READ COMMITTED` a row that turned private
// between the two would be read as it is now, past the scope that admitted it.
// The statements only read, so the snapshot cannot fail to serialize. The one
// write, the agent's log row, is a fresh insert that no update contends with.
//
// logged writes the preview to system_log in the same transaction. A human
// typing is not logged; an agent's preview is, because for an agent the same
// read is a bulk read of records that must leave a trace.
func (h filterPreviewHandlers) preview(
	ctx context.Context, engine storekit.Query, pred storekit.Predicate, scope string, after *ids.UUID, limit int, logged bool,
) (crmcontracts.FilterPreview, error) {
	var out crmcontracts.FilterPreview
	err := database.WithWorkspaceSnapshot(ctx, h.pool, func(ctx context.Context) error {
		return database.WithWorkspaceTx(ctx, h.pool, func(tx pgx.Tx) error {
			return previewIn(ctx, tx, engine, pred, scope, after, limit, logged, &out)
		})
	})
	return out, err
}

// previewIn is preview's body: every statement runs on tx, the snapshot's.
func previewIn(
	ctx context.Context, tx pgx.Tx, engine storekit.Query, pred storekit.Predicate,
	scope string, after *ids.UUID, limit int, logged bool, out *crmcontracts.FilterPreview,
) error {
	// The count is unbounded, and the caller writes its predicate: up to
	// PredicateMaxLeaves substring matches, each a possible sequential scan.
	// So the statement carries the ceiling. A filter too costly to count is
	// refused in bounded time instead of holding a pool connection.
	if err := database.BoundStatement(ctx, tx, previewStatementBudget); err != nil {
		return err
	}
	count, err := engine.CountMatching(ctx, tx, pred)
	if err != nil {
		return err
	}
	matched, more, err := engine.SelectPage(ctx, tx, pred, after, limit)
	if err != nil {
		return err
	}
	columns, rows, err := previewProjection(ctx, tx, engine, matched)
	if err != nil {
		return err
	}
	*out = crmcontracts.FilterPreview{
		MatchCount: count,
		Columns:    columns,
		Rows:       rowsAsMaps(memberData{table: engine.Table, columns: columns, rows: rows}),
		Truncated:  more,
	}
	page, err := storekit.IDPage(scope, matched, more)
	if err != nil {
		return err
	}
	if page.HasMore {
		out.NextCursor = &page.NextCursor
	}
	if !logged {
		return nil
	}
	_, err = storekit.LogSystem(ctx, tx, "preview", map[string]any{
		"table": engine.Table, logDetailFilter: pred, "match_count": count, "row_count": len(rows),
	})
	return err
}

// previewRowLimit resolves the requested page size, refusing what the contract
// calls invalid rather than quietly substituting something else.
//
// An ABSENT limit is the documented default: a caller who did not ask for a size
// is a builder rendering a glance. A limit outside the published 1..100 is a 422,
// which is the sibling preview's convention (the automation preview refuses
// window_days = 0 for the same reason) and the only answer consistent with
// declaring bounds at all — silently returning 100 rows to somebody who asked for
// 5000 tells them their request was honoured when it was rewritten.
func previewRowLimit(requested *int) (int, error) {
	if requested == nil {
		return filterPreviewDefaultRows, nil
	}
	if *requested < 1 || *requested > filterPreviewMaxRows {
		return 0, httperr.Validation("limit", codeInvalid,
			fmt.Sprintf("limit must be between 1 and %d — a preview is a glance, not a report", filterPreviewMaxRows))
	}
	return *requested, nil
}

// writeFilterPreviewError maps a refused predicate onto the 422 the contract
// promises, naming the offending field. Everything else — a permission denial
// from the engine's own read gate, a database fault — travels as itself.
func writeFilterPreviewError(w http.ResponseWriter, r *http.Request, err error) {
	if pred, ok := errors.AsType[*storekit.PredicateError](err); ok {
		httperr.Write(w, r, httperr.Validation(pred.Field, pred.Code, pred.Message))
		return
	}
	httperr.Write(w, r, err)
}

// previewCursor reads a preview's cursor for the one paging it continues: this
// resource under this filter. Both doors, the route and read_lists, read it
// here, so a cursor minted by either continues on the other.
func previewCursor(resource string, pred storekit.Predicate, token string) (string, *ids.UUID, error) {
	scope, err := storekit.CursorScope("filter_preview", resource, pred)
	if err != nil {
		return "", nil, err
	}
	after, err := storekit.DecodeIDCursor(scope, token)
	return scope, after, err
}
