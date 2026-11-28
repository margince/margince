// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package recordexport

import (
	"context"
	"errors"
	"fmt"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// exportPageSize is the page the ordering read walks the matched rows in.
const exportPageSize = 200

// Ordered puts the matched ids in the order the list screen shows them under the
// sort spec a saved view stores. The screen's own list read does the ordering,
// narrowed to the matched ids. The sort vocabulary and tie-break are then the
// screen's, not a second copy. A spec the list refuses leaves the matched order.
func Ordered(ctx context.Context, db *database.DB, object, sort string, matched []ids.UUID) ([]ids.UUID, error) {
	// The projects list takes no narrowing to a set of ids, so a sorted project
	// export keeps the matched order.
	if sort == "" || len(matched) == 0 || object == objectProject {
		return matched, nil
	}
	only := func(idColumn string, arg func(any) int) (string, error) {
		return fmt.Sprintf("%s = ANY($%d)", idColumn, arg(matched)), nil
	}
	ordered := make([]ids.UUID, 0, len(matched))
	var cursor *string
	for {
		page, next, err := listPage(ctx, db, object, listQuery{sort: &sort, only: only, cursor: cursor})
		if _, refused := errors.AsType[*storekit.SortError](err); refused {
			return matched, nil
		}
		if err != nil {
			return nil, err
		}
		ordered = append(ordered, page...)
		if !next.HasMore {
			return ordered, nil
		}
		cursor = &next.NextCursor
	}
}

type listQuery struct {
	sort   *string
	only   storekit.ListMemberFilter
	cursor *string
}

func listPage(ctx context.Context, db *database.DB, object string, q listQuery) ([]ids.UUID, storekit.Page, error) {
	limit := exportPageSize
	switch object {
	case objectContact:
		rows, page, err := contacts.NewStore(db).ListContacts(ctx, contacts.ListContactsInput{Cursor: q.cursor, Limit: &limit, Sort: q.sort, Membership: q.only})
		return idsOf(rows, func(r crmcontracts.Contact) ids.UUID { return ids.UUID(r.Id) }), page, err
	case objectCompany:
		rows, page, err := contacts.NewStore(db).ListCompanies(ctx, contacts.ListCompaniesInput{Cursor: q.cursor, Limit: &limit, Sort: q.sort, Membership: q.only})
		return idsOf(rows, func(r crmcontracts.Company) ids.UUID { return ids.UUID(r.Id) }), page, err
	case objectLead:
		rows, page, err := contacts.NewStore(db).ListLeads(ctx, contacts.ListLeadsInput{Cursor: q.cursor, Limit: &limit, Sort: q.sort, Membership: q.only})
		return idsOf(rows, func(r crmcontracts.Lead) ids.UUID { return ids.UUID(r.Id) }), page, err
	case objectDeal:
		rows, page, err := deals.NewStore(db, deals.Installation{}).ListDeals(ctx, deals.ListDealsInput{Cursor: q.cursor, Limit: &limit, Sort: q.sort, Membership: q.only})
		return idsOf(rows, func(r crmcontracts.Deal) ids.UUID { return ids.UUID(r.Id) }), page, err
	}
	return nil, storekit.Page{}, fmt.Errorf("export: %q has no list to order by", object)
}

func idsOf[T any](rows []T, id func(T) ids.UUID) []ids.UUID {
	out := make([]ids.UUID, len(rows))
	for i, row := range rows {
		out[i] = id(row)
	}
	return out
}
