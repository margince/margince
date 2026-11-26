// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package retirementblockers

import (
	"context"
	"fmt"

	"github.com/margince/margince/backend/internal/modules/reporting"
	"github.com/margince/margince/backend/internal/platform/auth"
)

const (
	dealPage = "deal"
	nameCol  = "b.name"
	// sqlTrue is the rule of a reader that hides no row.
	sqlTrue = "TRUE"
)

// reader is how the store that owns a blocking table serves its rows. It asks
// the read grants first, then its row rule over the row aliased b. Its SQL is
// compile-time text, and the rule's own values bind through arg.
type reader struct {
	objects []string
	name    string
	rows    func(ctx context.Context, arg func(any) int) (string, error)
	// pageType and pageID name the record whose page opens the row.
	pageType, pageID string
}

// readers lists the tables whose blocking rows the preview may name. Each entry
// asks what its store's list asks, so the preview names no row that list hides.
var readers = map[string]reader{
	"deal_room.steward_user_id": {objects: []string{"deal_room"}, name: "b.title", rows: dealRoomRows, pageType: dealPage, pageID: "b.deal_id"},
	"automation.owner_id":       {objects: []string{"automation"}, name: nameCol},
	"web_form.owner_id":         {objects: []string{"integrations"}, name: nameCol},
	// A target address can carry a token in its query, so only the rest names it.
	"webhook_subscription.owner_id": {objects: []string{"webhook_subscription"}, name: `regexp_replace(b.target_url, '[?#].*$', '')`},
	"sales_initiative.owner_id":     {objects: []string{"sales_initiative"}, name: nameCol, rows: initiativeRows},
	"report_definition.owner_id":    {objects: []string{"report_definition"}, name: nameCol, rows: reportRows("b.id")},
	"report_schedule.owner_id": {
		objects: []string{"report_definition", "report_schedule"},
		name:    "(SELECT d.name FROM report_definition d WHERE d.id = b.report_id)", rows: reportRows("b.report_id"),
	},
}

// dealRoomRows is the Deal Room list's rule: the room's deal is in the reader's scope.
func dealRoomRows(ctx context.Context, arg func(any) int) (string, error) {
	return auth.DealChildScopeClause(ctx, "b", arg)
}

func initiativeRows(ctx context.Context, arg func(any) int) (string, error) {
	scope, err := auth.ScopeClauseFor(ctx, "sales_initiative", "b", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		return sqlTrue, nil
	}
	return scope, nil
}

// reportRows is the report library's audience rule over the report column names.
func reportRows(report string) func(context.Context, func(any) int) (string, error) {
	return func(ctx context.Context, arg func(any) int) (string, error) {
		audience, err := reporting.ReportAudienceClause(ctx, arg)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("EXISTS (SELECT 1 FROM report_definition d WHERE d.id = %s AND %s)", report, audience), nil
	}
}
