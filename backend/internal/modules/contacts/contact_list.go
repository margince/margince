// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The contact list read: the shared listPage runner bound to the contact
// table — DM-VOCAB-1 sort vocabulary, the shared filter chain, and the
// contact row scan + child attachment.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// contactEntity is the contact's auth object and table name.
const contactEntity = "contact"

// contactNameColumn is the contact's display column — the quick-find
// target and the DM-VOCAB-1 name sort key.
const contactNameColumn = "full_name"

// ListContactsInput carries the contact list's contract parameters.
type ListContactsInput struct {
	Cursor  *string
	Limit   *int
	Query   *string
	OwnerID *ids.UserID
	// OwnerTeamID narrows to a team's rows; Unassigned to the unowned queue.
	// Both narrow the caller's row scope and never widen it — see
	// listFilters.ownershipClause, which also refuses two of them at once.
	OwnerTeamID     *ids.TeamID
	Unassigned      *bool
	IncludeArchived bool
	// CapturedByKind filters on the captured_by prefix (ADR-0075/A121 §3a).
	CapturedByKind *string
	// AiWritten filters on whether an AI wrote into the record (§3a).
	AiWritten *bool
	// Sort is the contract's sort spec, validated against the core
	// vocabulary below plus the workspace's active cf_ columns.
	Sort *string
	// CustomFilters carries the request's cf_* query parameters —
	// equality matches against active custom columns (storekit listquery).
	CustomFilters map[string]string
	// TagIDs narrows to the contacts carrying these tags, combined by TagMode.
	// The tag vocabulary belongs to another module, so this is a link
	// predicate rather than a column; storekit.TagFilterClause renders it, and
	// the company and deal lists call the same function.
	TagIDs  []ids.UUID
	TagMode storekit.TagMode
	// CompanyID narrows to the contacts employed there today. Employment is
	// an edge, not a column on contact, so this is a link predicate too — see
	// contactEmployerClause.
	CompanyID *ids.CompanyID
}

// contactListFields is the contact list's core sortable vocabulary —
// exactly the data-model §13.5 DM-VOCAB-1 set; active cf_ columns join
// it per request.
var contactListFields = map[string]storekit.SortField{
	createdAtColumn:    storekit.Column(storekit.KindTimestamp),
	updatedAtColumn:    storekit.Column(storekit.KindTimestamp),
	contactNameColumn:  storekit.Column(fieldcatalog.TypeText),
	ownerIDColumn:      storekit.Column(storekit.KindUUID),
	lastActivityColumn: storekit.Column(storekit.KindTimestamp),
	// The Company header, by the employer the row prints.
	contactEmployerField: {Kind: fieldcatalog.TypeText, Expr: orderByCurrentEmployer},
	// The Email header, by the one address the row prints.
	contactPrimaryEmailField: {Kind: fieldcatalog.TypeText, Expr: orderByReachableEmail},
}

// contactPrimaryEmailField is what the Email header sorts by, named for the wire
// field the column draws rather than a column of `contact`: an address lives on
// contact_email, and which of them a row shows is a choice.
const contactPrimaryEmailField = "primary_email"

// orderByReachableEmail orders by the address the Email column prints.
//
// The same expression the row is rendered from (ReachableEmailOrder), so the
// address a reader sees and the address the page is arranged by are one string.
// A contact with no live address shows none and orders by none, which the list
// already puts last.
func orderByReachableEmail(context.Context, func(any) int) (string, error) {
	return `(SELECT pe.email FROM contact_email pe
	          WHERE pe.contact_id = contact.id AND pe.archived_at IS NULL` +
		ReachableEmailOrder + ` LIMIT 1)`, nil
}

// contactEmployerField is what the Company header sorts by. Named for the wire
// field the column draws rather than a column of `contact`, because the employer
// is an EDGE: the row carries a company resolved through it, and the sort walks
// the same edge.
const contactEmployerField = "employer"

// orderByCurrentEmployer orders by the company the Company column prints, and
// by NOTHING when this caller may see no employer at all.
//
// The three gates that can hide an employer are the read's own
// (currentEmployerFrom), so a contact whose company is outside this caller's
// scope sorts into the tail rather than ordering the page by a name the row
// beside it leaves blank.
func orderByCurrentEmployer(ctx context.Context, arg func(any) int) (string, error) {
	from, visible, err := currentEmployerFrom(ctx, "rel.contact_id = contact.id", arg)
	if err != nil {
		return "", err
	}
	if !visible {
		// A caller who may see no employer is ordered by no employer: every row
		// sits in the tail and the page falls back to its tie-breaker.
		return "NULL::text", nil
	}
	return "(SELECT company.display_name" + from + ")", nil
}

// contactTagClause narrows the page to the contacts carrying the named tags.
//
// The predicate is storekit's, shared with the company and deal lists: three
// copies of a NOT EXISTS is three chances for `none` to mean something subtly
// different on one surface.
func contactTagClause(ctx context.Context, tagIDs []ids.UUID, mode storekit.TagMode, arg func(any) int) string {
	return storekit.TagFilterClause(ctx, contactEntity, "contact.id", tagIDs, mode, arg)
}

// contactEmployerClause narrows the page to the contacts who work at one account
// today, or "" when the caller named none.
//
// CURRENT PRIMARY employment only. A contact's history carries every employer
// they have had, and "who works there" is not "who has ever worked there" — a
// list that answered the second would hand a rep the leavers alongside the
// staff, which is the wrong answer wearing the right shape. The edge is the
// one `uq_rel_current_primary_employer` keeps unique per contact, so this
// matches at most one row per contact and cannot duplicate the page.
//
// EXISTS rather than a join, for the same reason the tag filter uses one: a
// join multiplies a contact by their matching edges, and the keyset cursor
// would page over those copies as though they were distinct contacts.
//
// It carries the EDGE grant, and the reason is worth stating because the edge
// contributes nothing to the response: filtering by employer answers "who works
// at Acme" one page at a time, which is a stronger disclosure than the contact
// count on the account itself — a listing beats a count. A caller refused the
// edge is refused the FILTER rather than handed an empty page: they asked a
// question about the pairs, and an empty page would answer it with "nobody",
// which is false.
func contactEmployerClause(ctx context.Context, companyID *ids.CompanyID, arg func(any) int) (string, error) {
	if companyID == nil {
		return "", nil
	}
	edgeBound, err := auth.EdgeReadScope(ctx, "rel", arg)
	if err != nil {
		return "", err
	}
	if edgeBound == "" {
		edgeBound = "TRUE"
	}
	return storekit.SQLf(`EXISTS (
		SELECT 1 FROM relationship rel
		WHERE rel.contact_id = contact.id
		  AND rel.kind = 'employment'
		  AND `+employment.CurrentPrimarySQL("rel")+`
		  AND rel.archived_at IS NULL
		  AND `+edgeBound+`
		  AND rel.company_id = $%d)`, arg(*companyID)), nil
}

// ListContacts is the row-scoped contact list read: quick-find, owner, tag and
// custom-field filters, keyset pagination under the validated sort.
func (s *Store) ListContacts(ctx context.Context, in ListContactsInput) ([]crmcontracts.Contact, storekit.Page, error) {
	shared := listFilters{
		IncludeArchived: in.IncludeArchived,
		CapturedByKind:  in.CapturedByKind,
		AiWritten:       in.AiWritten,
		entity:          contactEntity,
		OwnerID:         in.OwnerID,
		OwnerTeamID:     in.OwnerTeamID,
		Unassigned:      in.Unassigned,
		Query:           in.Query,
		Cursor:          in.Cursor,
		CustomFilters:   in.CustomFilters,
		nameColumn:      contactNameColumn,
		identifier: storekit.Identifier{
			Table: "contact_email", FK: contactFK, Column: emailColumn,
		},
	}
	return listPage(ctx, s, in.Sort, in.Limit, listPageSpec[crmcontracts.Contact]{
		entity:  contactEntity,
		columns: contactColumns,
		fields:  contactListFields,
		filters: func(active []fieldcatalog.Column, sorted *storekit.ListSort, arg func(any) int) ([]string, error) {
			where, err := shared.clauses(active, sorted, arg)
			if err != nil {
				return nil, err
			}
			if clause := contactTagClause(ctx, in.TagIDs, in.TagMode, arg); clause != "" {
				where = append(where, clause)
			}
			employer, err := contactEmployerClause(ctx, in.CompanyID, arg)
			if err != nil {
				return nil, err
			}
			if employer != "" {
				where = append(where, employer)
			}
			return where, nil
		},
		scan:   scanContactPage,
		attach: attachContactChildren,
		cursorKey: func(last crmcontracts.Contact) (time.Time, ids.UUID) {
			return last.CreatedAt, ids.UUID(last.Id)
		},
	})
}

// scanContactPage drains one list query's rows: each contact plus, under a
// non-default sort, the row's cursor key (the trailing __cursor_key
// column CursorKeySuffix appended).
func scanContactPage(rows pgx.Rows, active []fieldcatalog.Column, sorted *storekit.ListSort) ([]crmcontracts.Contact, []*string, error) {
	var contacts []crmcontracts.Contact
	var cursorKeys []*string
	for rows.Next() {
		var key *string
		extra := []any{}
		if sorted != nil {
			extra = append(extra, &key)
		}
		p, err := scanContact(rows, active, extra...)
		if err != nil {
			return nil, nil, err
		}
		contacts = append(contacts, p)
		cursorKeys = append(cursorKeys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return contacts, cursorKeys, nil
}
