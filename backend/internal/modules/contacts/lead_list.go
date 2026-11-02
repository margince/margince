// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The lead list read: the default operational queue plus the shared listPage
// runner for explicit field sorts. Score remains available as an explicit
// sort without displacing the SLA-first default.

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

const leadEntity = "lead"

// The lead's sortable columns, named once. The vocabulary below and the
// clauses that read them agree by construction rather than by two contacts
// spelling the same column the same way.
const (
	// leadNameColumn is the display column and the name sort key.
	leadNameColumn    = "full_name"
	leadCompanyColumn = "company_name"
	leadStatusColumn  = "status"
	leadScoreColumn   = "score"
	leadSourceColumn  = "source"
	leadOriginColumn  = "origin"
	createdAtColumn   = "created_at"
	updatedAtColumn   = "updated_at"
	// lastActivityColumn is the timeline clock contact and company carry
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
	ownerIDColumn:     storekit.OwnerNameSort(leadEntity),
	// The three the list draws and does not store. Each reads the same
	// expression the row is PRINTED from (lead_read.go), so a reader sees the
	// value the page was arranged by.
	lastActivityColumn: {Kind: storekit.KindTimestamp, Expr: orderByLeadLastActivity},
	leadNextTaskField:  {Kind: storekit.KindTimestamp, Expr: orderByLeadNextTaskDue},
	leadSourceColumn:   {Kind: fieldcatalog.TypeText, Expr: orderByLeadSourceLabel},
	leadOriginColumn:   {Kind: fieldcatalog.TypeText, Expr: orderByLeadOriginLabel},
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
func orderByLeadNextTaskDue(ctx context.Context, arg func(any) int) (string, error) {
	work, err := leadTaskWorkSQL(ctx, arg)
	if err != nil {
		return "", err
	}
	return leadNextTaskDueSQL(work), nil
}

// orderByLeadSourceLabel orders by the words the Source column prints rather
// than the key behind them: a catalog whose labels and keys disagree would
// otherwise arrange the list by an identifier no reader can see.
func orderByLeadSourceLabel(context.Context, func(any) int) (string, error) {
	return leadSourceLabelSQL, nil
}

// orderByLeadOriginLabel orders by the words the Origin column prints, for the
// reason orderByLeadSourceLabel gives.
func orderByLeadOriginLabel(context.Context, func(any) int) (string, error) {
	return leadOriginLabelSQL, nil
}

// ListLeads is the row-scoped lead list read: quick-find, the status and
// owner filters, and keyset pagination under the validated sort.
func (s *Store) ListLeads(ctx context.Context, in ListLeadsInput) ([]crmcontracts.Lead, storekit.Page, error) {
	// Narrowing by a contact the caller cannot open would tell them which lead
	// it is worked through, so such a contact matches nothing.
	if in.FromContactID != nil {
		if _, err := s.GetContact(ctx, *in.FromContactID, storekit.IncludeArchived); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				return []crmcontracts.Lead{}, storekit.Page{}, nil
			}
			return nil, storekit.Page{}, err
		}
	}
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	if in.Sort == nil || *in.Sort == "" {
		return s.listLeadWorkQueue(ctx, in)
	}
	policy, err := s.slaPolicy(ctx)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	// One instant for this read: the sla_state filter is built once for the
	// page and once for the total, and they must judge the same leads overdue.
	ctx = withPinnedSLANow(ctx, leadSLAClock().UTC())
	return listPage(ctx, s, in.Sort, in.Limit, listPageSpec[crmcontracts.Lead]{
		entity:  leadEntity,
		prepare: withLeadTaskIdentities,
		columns: leadColumns,
		fields:  leadListFields,
		filters: func(ctx context.Context, active []fieldcatalog.Column, sorted *storekit.ListSort, arg func(any) int) ([]string, error) {
			return leadListWhere(ctx, in, active, sorted, policy, arg)
		},
		scan: func(rows pgx.Rows, active []fieldcatalog.Column, sorted *storekit.ListSort) ([]crmcontracts.Lead, []*string, error) {
			return scanLeadPage(rows, active, sorted, policy)
		},
		attach: attachLeadRows,
		cursorKey: func(last crmcontracts.Lead) (time.Time, ids.UUID) {
			return last.CreatedAt, ids.UUID(last.Id)
		},
	})
}

// ListLeadsInput is one read of the lead list: its page, its sort and what it narrows by.
type ListLeadsInput struct {
	Cursor *string
	Limit  *int
	Status *string
	// OwnerID, OwnerTeamID and Unassigned are the owner dials every
	// owner-scoped list carries (DM-VOCAB-OWN-1), bound through
	// storekit.OwnershipClause as contact and company bind them.
	OwnerID         *ids.UserID
	OwnerTeamID     *ids.TeamID
	Unassigned      *bool
	Query           *string
	IncludeArchived bool
	// CapturedByKind filters on the captured_by prefix (ADR-0075).
	CapturedByKind *string
	// AiWritten filters on whether an AI wrote into the record.
	AiWritten *bool
	// MinScore is the triage floor. A reader working the warmest leads first
	// keeps the colder ones off the page rather than scanning past them.
	MinScore *int
	// OwedAReply keeps only the leads nobody has answered. The page is bounded,
	// so a caller cutting answered rows off it would lose the unanswered leads
	// behind them.
	OwedAReply *bool
	// Source narrows to how the row was written (import, crawl, manual,
	// `connector:<name>`).
	Source *string
	// Origin narrows to one origin key, or `unset` for the unclassified.
	Origin *string
	// TagIDs narrows to the leads carrying these tags, combined by TagMode.
	TagIDs  []ids.UUID
	TagMode storekit.TagMode
	// FromContactID narrows to the leads worked from one contact.
	FromContactID *ids.ContactID
	// SLAState narrows to leads in one first-response state; breached is the
	// overdue queue.
	SLAState *crmcontracts.ListLeadsParamsSlaState
	// Sort is the contract's sort spec, validated against the lead
	// vocabulary plus the workspace's active cf_ columns.
	Sort *string
	// Membership narrows to one list's members (list_id).
	Membership storekit.ListMemberFilter
	// MemberOf is the list a seam caller narrowed to; the provider resolves it
	// into Membership before the read.
	MemberOf *ids.UUID
	// CompanyFacts narrows by what the linked company records (leadcompanyfacts.go).
	CompanyFacts LeadCompanyFactFilters
	// Partner narrows to the leads a partner referred.
	Partner LeadPartnerFilter
}

// leadListWhere builds a lead list read's `WHERE`: the shared chain, then the lead's own
// filters. The sorted page, the work queue and the filter-reach probe all call it.
func leadListWhere(ctx context.Context, in ListLeadsInput, active []fieldcatalog.Column, sorted *storekit.ListSort,
	policy leadSLAPolicy, arg func(any) int,
) ([]string, error) {
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
	}.clauses(ctx, active, sorted, arg)
	if err != nil {
		return nil, err
	}
	narrowed, err := leadNarrowing(ctx, in, policy, arg)
	return append(where, narrowed...), err
}

// leadNarrowing is the lead's own filters, after the shared chain. Both reads
// of the list — the sorted page and the default work queue — narrow by it, so
// a filter added here reaches both or neither.
func leadNarrowing(ctx context.Context, in ListLeadsInput, policy leadSLAPolicy, arg func(any) int) ([]string, error) {
	where, err := in.CompanyFacts.clauses(ctx, arg)
	if err != nil {
		return nil, err
	}
	partner, err := in.Partner.clauses(ctx, arg)
	if err != nil {
		return nil, err
	}
	where = append(where, partner...)
	if in.Membership != nil {
		member, err := in.Membership(leadEntity+".id", arg)
		if err != nil {
			return nil, err
		}
		where = append(where, member)
	}
	if in.Query != nil && *in.Query != "" {
		where = append(where, leadQuickFindClause(*in.Query, arg))
	}
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
	if in.Origin != nil {
		where = append(where, leadOriginClause(*in.Origin, arg))
	}
	if in.SLAState != nil {
		where = append(where, slaStateClause(ctx, policy, *in.SLAState, arg))
	}
	if clause := storekit.TagFilterClause(ctx, leadEntity, "lead.id", in.TagIDs, in.TagMode, arg); clause != "" {
		where = append(where, clause)
	}
	if in.FromContactID != nil {
		where = append(where, storekit.SQLf("lead.from_contact_id = $%d", arg(*in.FromContactID)))
	}
	return where, nil
}

// attachLeadRows is what a page of leads owes each row beyond its own columns:
// whether this caller may change it, and the tags it carries. Both reads of
// the list attach through here.
func attachLeadRows(ctx context.Context, tx pgx.Tx, leads []crmcontracts.Lead) error {
	if err := stampLeadsWritable(ctx, tx, leads); err != nil {
		return err
	}
	if err := withholdUnreadableLinks(ctx, tx, leads); err != nil {
		return err
	}
	return storekit.AttachRowTags(ctx, tx, leadEntity, leads,
		func(l crmcontracts.Lead) ids.UUID { return ids.UUID(l.Id) },
		func(l *crmcontracts.Lead, tags []storekit.RowTag) { l.Tags = wireRowTags(tags) })
}

// leadQuickFindExpr is the substring target: the lead's name and the company
// it sits at. It is spelled exactly as idx_lead_name_trgm indexes it, so a
// fragment of either is one index read rather than a scan of every lead.
const leadQuickFindExpr = `(coalesce(full_name, '') || ' ' || coalesce(company_name, ''))`

func leadQuickFindClause(query string, arg func(any) int) string {
	pos := arg(strings.TrimSpace(query))
	return storekit.SQLf(`(%s
		OR lower(rtrim(linkedin_url, '/')) = lower(rtrim($%d, '/')))`,
		storekit.QuickFindClauseWith(pos, leadQuickFindExpr, storekit.LeadEmails), pos)
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
