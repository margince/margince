// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The lead list read: the default operational queue plus the shared listPage
// runner for explicit field sorts. Score remains available as an explicit
// sort without displacing the SLA-first default.

import (
	"context"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const leadEntity = "lead"

// The lead's sortable columns, named once. The vocabulary below and the
// clauses that read them agree by construction rather than by two people
// spelling the same column the same way.
const (
	// leadNameColumn is the display column: the quick-find target and the
	// name sort key.
	leadNameColumn    = "full_name"
	leadCompanyColumn = "company_name"
	leadStatusColumn  = "status"
	leadScoreColumn   = "score"
	leadSourceColumn  = "source"
	createdAtColumn   = "created_at"
	updatedAtColumn   = "updated_at"
	// lastActivityColumn is the timeline clock person and company carry
	// (DM-VOCAB-1/2), maintained in the schema on the activity-link write.
	lastActivityColumn = "last_activity_at"
)

// leadListFields is the lead list's core sortable vocabulary; active cf_
// columns join it per request.
//
// It used to say every column the list shows is here, which was not true and
// nothing held: the list draws Last activity, and a lead's is DERIVED from
// activity_link rather than stored, so it is not a column this vocabulary can
// name. That header no longer offers a sort, and
// TestEverySortAListOffersIsOneItsResourceAccepts is what keeps the two
// answering together.
var leadListFields = map[string]storekit.SortField{
	createdAtColumn:   storekit.Column(storekit.KindTimestamp),
	updatedAtColumn:   storekit.Column(storekit.KindTimestamp),
	leadNameColumn:    storekit.Column(fieldcatalog.TypeText),
	leadCompanyColumn: storekit.Column(fieldcatalog.TypeText),
	leadStatusColumn:  storekit.Column(fieldcatalog.TypeText),
	leadScoreColumn:   storekit.Column(fieldcatalog.TypeNumber),
	ownerIDColumn:     storekit.Column(storekit.KindUUID),
	// The three the list draws and does not store. Each reads the same
	// expression the row is PRINTED from (lead_read.go), so a reader sees the
	// value the page was arranged by.
	lastActivityColumn: {Kind: storekit.KindTimestamp, Expr: orderByLeadLastActivity},
	leadNextTaskField:  {Kind: storekit.KindTimestamp, Expr: orderByLeadNextTaskDue},
	leadSourceColumn:   {Kind: fieldcatalog.TypeText, Expr: orderByLeadSourceLabel},
}

// leadNextTaskField is what the Next task header sorts by: when the next open
// task falls due. The column shows a title, a count and a deadline, and the
// deadline is the one a reader sorting it means — a list ordered by task titles
// answers a question nobody asked.
const leadNextTaskField = "next_task_due_at"

// orderByLeadLastActivity orders by the last-touch clock the row prints.
func orderByLeadLastActivity(context.Context, func(any) int) (string, error) {
	return leadLastActivitySQL(), nil
}

// orderByLeadNextTaskDue orders by the deadline the Next task column prints.
// A lead with no open task shows none and orders by none, which the list's
// ORDER BY already puts last.
func orderByLeadNextTaskDue(context.Context, func(any) int) (string, error) {
	return leadNextTaskDueSQL, nil
}

// orderByLeadSourceLabel orders by the words the Source column prints rather
// than the key behind them: a catalog whose labels and keys disagree would
// otherwise arrange the list by an identifier no reader can see.
func orderByLeadSourceLabel(context.Context, func(any) int) (string, error) {
	return leadSourceLabelSQL, nil
}

// ListLeads is the row-scoped lead list read: quick-find, the status and
// owner filters, and keyset pagination under the validated sort.
func (s *Store) ListLeads(ctx context.Context, in ListLeadsInput) ([]crmcontracts.Lead, storekit.Page, error) {
	if in.Sort == nil || *in.Sort == "" {
		return s.listLeadWorkQueue(ctx, in)
	}
	policy, err := s.slaPolicy(ctx)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return listPage(ctx, s, in.Sort, in.Limit, listPageSpec[crmcontracts.Lead]{
		entity:  leadEntity,
		columns: leadColumns,
		fields:  leadListFields,
		filters: func(active []fieldcatalog.Column, sorted *storekit.ListSort, arg func(any) int) ([]string, error) {
			where, err := listFilters{
				IncludeArchived: in.IncludeArchived,
				CapturedByKind:  in.CapturedByKind,
				AiWritten:       in.AiWritten,
				entity:          leadEntity,
				OwnerID:         in.OwnerID,
				OwnerTeamID:     in.OwnerTeamID,
				Unassigned:      in.Unassigned,
				Query:           nil,
				Cursor:          in.Cursor,
				nameColumn:      leadNameColumn,
			}.clauses(active, sorted, arg)
			if err != nil {
				return nil, err
			}
			if in.Query != nil && *in.Query != "" {
				where = append(where, leadQuickFindClause(*in.Query, arg))
			}
			// The lead's own narrowing, alongside the shared chain.
			if in.Status != nil {
				where = append(where, storekit.SQLf(leadStatusColumn+" = $%d", arg(*in.Status)))
			}
			if in.OwedAReply != nil && *in.OwedAReply {
				where = append(where, leadOwesAReplySQL)
			}
			if in.MinScore != nil {
				where = append(where, storekit.SQLf(leadScoreColumn+" >= $%d", arg(*in.MinScore)))
			}
			if in.Source != nil {
				where = append(where, leadSourceClause(*in.Source, arg))
			}
			if in.SLAState != nil {
				where = append(where, slaStateClause(policy, *in.SLAState, arg))
			}
			return where, nil
		},
		scan: func(rows pgx.Rows, active []fieldcatalog.Column, sorted *storekit.ListSort) ([]crmcontracts.Lead, []*string, error) {
			return scanLeadPage(rows, active, sorted, policy)
		},
		// A lead is one flat row: no child tables to load alongside the page.
		// The one thing the page still owes each row is whether it is this
		// caller's to change, which is one statement for the whole page.
		attach: stampLeadsWritable,
		cursorKey: func(last crmcontracts.Lead) (time.Time, ids.UUID) {
			return last.CreatedAt, ids.UUID(last.Id)
		},
	})
}

func leadQuickFindClause(query string, arg func(any) int) string {
	pos := arg(strings.TrimSpace(query))
	return storekit.SQLf(`(%s OR email = lower($%d)
		OR lower(rtrim(linkedin_url, '/')) = lower(rtrim($%d, '/')))`,
		storekit.QuickFindClause(pos, leadNameColumn), pos, pos)
}

// scanLeadPage drains one list query's rows: each lead plus, under a
// non-default sort, the row's trailing __cursor_key.
func scanLeadPage(rows pgx.Rows, active []fieldcatalog.Column, sorted *storekit.ListSort, policy leadSLAPolicy) ([]crmcontracts.Lead, []*string, error) {
	var leads []crmcontracts.Lead
	var cursorKeys []*string
	for rows.Next() {
		var key *string
		extra := []any{}
		if sorted != nil {
			extra = append(extra, &key)
		}
		l, err := scanLead(rows, active, policy, extra...)
		if err != nil {
			return nil, nil, err
		}
		leads = append(leads, l)
		cursorKeys = append(cursorKeys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return leads, cursorKeys, nil
}
