// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package salesprogress

import (
	"context"
	"errors"
	"net/http"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ListSalesProgressDeals lists the open deals behind a Progress reading. The
// deals come from the deal list itself, so its row scope and masks hold.
func (h Reading) ListSalesProgressDeals(w http.ResponseWriter, r *http.Request, params crmcontracts.ListSalesProgressDealsParams) {
	ctx := r.Context()
	var out crmcontracts.SalesProgressDealPage
	err := h.DB.TxIsolated(ctx, pgx.RepeatableRead, func(tx pgx.Tx) error {
		allowed, err := identity.IndividualPerformanceFor(ctx, tx)
		if err != nil {
			return err
		}
		scope, err := h.readableScope(ctx, tx, (*crmcontracts.GetSalesProgressParamsScopeKind)(params.ScopeKind), params.ScopeId)
		if err != nil {
			return err
		}
		in := deals.ListDealsInput{Status: new("open"), Cursor: params.Cursor, Limit: params.Limit}
		if scope.Kind == crmcontracts.ReportingScopeKindTeam {
			// The list names each member's deals one by one, so it is refused
			// while the switch is off. The team totals keep their floor.
			if !allowed {
				return auth.IndividualPerformanceOffError{}
			}
			in.OwnerTeamID = new(ids.From[ids.TeamKind](ids.UUID(*scope.Id)))
		} else {
			if !auth.MayMeasureSeat(ctx, allowed, ids.UUID(*scope.Id)) {
				return auth.IndividualPerformanceOffError{}
			}
			in.OwnerID = new(ids.From[ids.UserKind](ids.UUID(*scope.Id)))
		}
		page, next, err := h.Deals.ListDealsTx(ctx, tx, in, deals.CustomColumns{})
		if err != nil {
			return err
		}
		out, err = dealWork(ctx, tx, page, h.Reports.Now())
		out.Page = crmcontracts.PageInfo{HasMore: next.HasMore}
		if next.NextCursor != "" {
			out.Page.NextCursor = &next.NextCursor
		}
		return err
	})
	if err != nil {
		httperr.Write(w, r, err)
		return
	}
	httperr.WriteJSON(w, http.StatusOK, out)
}

func dealWork(ctx context.Context, tx pgx.Tx, page []crmcontracts.Deal, now time.Time) (crmcontracts.SalesProgressDealPage, error) {
	out := crmcontracts.SalesProgressDealPage{Data: make([]crmcontracts.SalesProgressDeal, 0, len(page))}
	dealIDs := make([]ids.UUID, 0, len(page))
	for _, deal := range page {
		dealIDs = append(dealIDs, ids.UUID(deal.Id))
	}
	lines, err := dealLineKinds(ctx, tx, dealIDs)
	if err != nil {
		return out, err
	}
	tasks, err := nextDealTasks(ctx, tx, dealIDs, now)
	if err != nil {
		return out, err
	}
	for _, deal := range page {
		row := crmcontracts.SalesProgressDeal{Deal: deal, NextTask: tasks[ids.UUID(deal.Id)]}
		if !masked(deal, "amount_minor") {
			kind := saleType(deal.AmountMinor, lines[ids.UUID(deal.Id)])
			row.SaleType = &kind
		}
		out.Data = append(out.Data, row)
	}
	return out, nil
}

func masked(deal crmcontracts.Deal, field string) bool {
	return deal.MaskedFields != nil && slices.Contains(*deal.MaskedFields, field)
}

// lineKinds is what a deal's priced lines add up to in each sale category.
// foreign marks a line in another currency, which no sum may mix in.
type lineKinds struct {
	product, service, other int64
	foreign                 bool
}

// dealLineKinds reads only the deals whose amount the caller may read, so a
// split never shows what the deal list masks.
func dealLineKinds(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID) (map[ids.UUID]lineKinds, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, err
	}
	args := []any{dealIDs}
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	visible, masked, err := auth.MaskExcludedClause(ctx, "deal", "amount_minor", "d", arg)
	if err != nil {
		return nil, err
	}
	where := ""
	if masked && visible != "" {
		where = " AND " + visible
	}
	rows, err := tx.Query(ctx, `SELECT dp.deal_id, COALESCE(dp.component_kind,''), dp.amount_minor, dp.currency IS DISTINCT FROM d.currency
		FROM deal_product dp JOIN deal d ON d.id = dp.deal_id
		WHERE dp.deal_id = ANY($1) AND dp.amount_minor IS NOT NULL`+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[ids.UUID]lineKinds{}
	for rows.Next() {
		var deal ids.UUID
		var kind string
		var amount int64
		var foreign bool
		if err := rows.Scan(&deal, &kind, &amount, &foreign); err != nil {
			return nil, err
		}
		sums := out[deal]
		sums.foreign = sums.foreign || foreign
		switch Category("kind:" + kind) {
		case Product:
			sums.product += amount
		case Service:
			sums.service += amount
		default:
			sums.other += amount
		}
		out[deal] = sums
	}
	return out, rows.Err()
}

// saleType names what a deal sells from its priced lines, against its value.
// Lines that overrun the value, or mix currencies, classify nothing.
func saleType(amount *int64, lines lineKinds) crmcontracts.SalesProgressDealSaleType {
	classified := lines.product + lines.service
	if amount == nil || lines.foreign || classified <= 0 || classified+lines.other > *amount {
		return crmcontracts.SalesProgressDealSaleTypeUnclassified
	}
	switch {
	case lines.other > 0 || classified < *amount:
		return crmcontracts.SalesProgressDealSaleTypePartlyClassified
	case lines.product > 0 && lines.service > 0:
		return crmcontracts.SalesProgressDealSaleTypeProductAndService
	case lines.product > 0:
		return crmcontracts.SalesProgressDealSaleTypeProduct
	}
	return crmcontracts.SalesProgressDealSaleTypeService
}

// nextDealTasks reads each deal's earliest-due current task that every
// workspace reader may see. A caller who may not read activities gets none.
func nextDealTasks(ctx context.Context, tx pgx.Tx, dealIDs []ids.UUID, now time.Time) (map[ids.UUID]*crmcontracts.SalesProgressNextTask, error) {
	out := map[ids.UUID]*crmcontracts.SalesProgressNextTask{}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return out, nil
		}
		return nil, err
	}
	args := []any{dealIDs}
	arg := func(value any) int {
		args = append(args, value)
		return len(args)
	}
	identities, err := storekit.MailSeatIdentitiesSQL(ctx, tx, arg)
	if err != nil {
		return nil, err
	}
	current := storekit.CurrentTaskClause("a", now, identities, arg)
	due := storekit.TaskDueSQL("a")
	rows, err := tx.Query(ctx, `SELECT DISTINCT ON (l.deal_id) l.deal_id, a.subject, `+due+`, a.assignee_id
		FROM activity_link l JOIN activity a ON a.id = l.activity_id
		WHERE l.deal_id = ANY($1) AND a.archived_at IS NULL AND a.restricted_at IS NULL AND a.kind = 'task' AND NOT a.is_done`+
		auth.AudienceWorkspaceOnly("a")+` AND `+current+`
		ORDER BY l.deal_id, `+due+` NULLS LAST, a.created_at, a.id`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var deal ids.UUID
		var task crmcontracts.SalesProgressNextTask
		var assignee *ids.UUID
		var dueAt *time.Time
		if err := rows.Scan(&deal, &task.Subject, &dueAt, &assignee); err != nil {
			return nil, err
		}
		task.DueAt = dueAt
		if assignee != nil {
			task.AssigneeId = new(openapi_types.UUID(*assignee))
		}
		out[deal] = &task
	}
	return out, rows.Err()
}
