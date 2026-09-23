// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// The deal read paths: single-row get, the filtered keyset list, and
// the one column list + scanner every deal read shares.

package deals

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

func (s *Store) GetDeal(ctx context.Context, id ids.DealID, archived storekit.ArchivedFilter) (crmcontracts.Deal, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return crmcontracts.Deal{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	var out crmcontracts.Deal
	err = s.Tx(ctx, func(tx pgx.Tx) (err error) {
		if err := auth.EnsureVisible(ctx, tx, dealTable, id.UUID); err != nil {
			return err
		}
		out, err = readDealForCaller(ctx, tx, id, archived, active)
		return err
	})
	return out, err
}

// readDealForCaller reads one deal and masks it — the spelling EVERY entry
// point that hands a deal back uses, a mutation response included. A response
// is a read: the row a PATCH echoes is the same row a GET withholds from, and
// the reference withholding cannot ride on write authority the way a role mask
// does — being allowed to change the DEAL says nothing about being allowed to
// read the COMPANY it names.
//
// readDeal itself stays unmasked on purpose. The update path builds its
// before-image from it, and an audit diff taken against a withheld null would
// record a change nobody made.
func readDealForCaller(ctx context.Context, tx pgx.Tx, id ids.DealID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Deal, error) {
	d, err := readDeal(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Deal{}, err
	}
	// Which closing this deal is on, for the ONE-deal read only. It is a
	// second query, so it is deliberately absent from the list path: a
	// correlated subquery in dealColumns would run once per row on every page
	// of every deal list, to answer a question only a record page asks.
	if err := attachClosingOccurrence(ctx, tx, id, &d); err != nil {
		return crmcontracts.Deal{}, err
	}
	return finishDealForCaller(ctx, tx, d)
}

// dealTaggableType is the value taggable.entity_type stores for a deal. It
// reads the same as dealTable and means something else: one is the column's
// vocabulary, the other is a relation name.
const dealTaggableType = "deal"

type ListDealsInput struct {
	// TagIDs narrows to the deals carrying these tags, combined by TagMode.
	// The predicate is storekit's, shared with the contact and account lists.
	TagIDs           []ids.UUID
	TagMode          storekit.TagMode
	Cursor           *string
	Limit            *int
	Query            *string
	PipelineID       *ids.PipelineID
	StageID          *ids.StageID
	OwnerID          *ids.UserID
	CompanyID        *ids.CompanyID
	ProjectID        *ids.ProjectID
	PartnerCompanyID *ids.CompanyID
	PartnerSourced   *bool
	// PartnerAttribution narrows to what the partner did — "sourced" or
	// "influenced". Narrower than PartnerSourced, which only asks whether a
	// partner is named at all.
	PartnerAttribution *string
	Status             *string
	// ForecastCategory narrows to one of the forecast's named buckets —
	// commit, best_case, pipeline, omitted. The column the FORECAST reads, so
	// a tile's number and the list behind it are one answer rather than two
	// derivations that can disagree.
	ForecastCategory *string
	// The three commercial-context filters. Each also admits the sentinel
	// "unset", which no enum value can express: a caller asking for deals
	// nobody has classified is asking about the absence, not about a value.
	CommercialMotion  *string
	Priority          *string
	AcquisitionSource *string
	Stalled           *bool
	// QuietForDays narrows to open deals idle at least this long, which is the
	// stalled rule at a caller-named window (QuietSQL). Separate from Stalled
	// because they answer different questions: Stalled is the product-wide
	// status, this is "notice it earlier". Set both and both apply.
	QuietForDays *int
	// CloseBefore filters calendar dates before the bound, before pagination.
	CloseBefore     *time.Time
	IncludeArchived bool
	// Sort is the contract's sort spec, validated against the core
	// vocabulary below plus the workspace's active cf_ columns.
	Sort *string
	// CustomFilters carries the request's cf_* query parameters —
	// equality matches against active custom columns (storekit listquery).
	CustomFilters map[string]string
}

// dealNameColumn is the deal's display-name column and the quick-find
// match expression.
const dealNameColumn = "name"

// dealListFields is the deal list's sortable vocabulary; active cf_ columns
// join it per request.
//
// A COLUMN THE LIST SHOWS IS A COLUMN THE LIST SORTS BY. It used to be exactly
// the data-model §13.5 DM-VOCAB-3 set, and the deals list draws seven columns
// of which that set reached three — so four headers were dead controls a reader
// had to learn to ignore, and the frontend said so in a comment beside each one
// rather than being able to fix it. Lars ruled on 2026-08-21 that the rule is
// the columns, for every list in the product; where that and the older set
// disagree, this follows the rule.
//
// Three of the eight are not columns of `deal` at all: a stage and the two
// companies are references, so each carries the expression that orders it.
var dealListFields = map[string]storekit.SortField{
	"created_at":            storekit.Column(storekit.KindTimestamp),
	"updated_at":            storekit.Column(storekit.KindTimestamp),
	"last_activity_at":      storekit.Column(storekit.KindTimestamp),
	"amount_minor":          storekit.Column(fieldcatalog.TypeCurrency),
	closeDateField:          storekit.Column(fieldcatalog.TypeDate),
	dealNameColumn:          storekit.Column(fieldcatalog.TypeText),
	"status":                storekit.Column(fieldcatalog.TypeText),
	filterStageID:           {Kind: fieldcatalog.TypeNumber, Expr: orderByStagePosition},
	filterCompanyID:         {Kind: fieldcatalog.TypeText, Expr: orderByReadableCompanyName(filterCompanyID)},
	filterPartnerCompanyID:  {Kind: fieldcatalog.TypeText, Expr: orderByReadableCompanyName(filterPartnerCompanyID)},
	filterCommercialMotion:  storekit.Column(fieldcatalog.TypeText),
	filterAcquisitionSource: storekit.Column(fieldcatalog.TypeText),
	// The wire name is `priority`; the ORDER BY is the generated rank beside
	// it, so High → Medium → Low comes out in its business order rather than
	// the alphabetical one ('high' < 'low' < 'medium' interleaves them).
	filterPriority: {Kind: fieldcatalog.TypeNumber, Expr: orderByPriorityRank},
}

// wireRowTags renders one deal row's tag chips. A twin of the contacts module's:
// a module never imports a sibling, and the shape is the contract's.
func wireRowTags(tags []storekit.RowTag) *[]crmcontracts.RowTag {
	out := make([]crmcontracts.RowTag, 0, len(tags))
	for _, t := range tags {
		var color *crmcontracts.RowTagColor
		if t.Color != nil {
			c := crmcontracts.RowTagColor(*t.Color)
			color = &c
		}
		out = append(out, crmcontracts.RowTag{
			TagId: openapi_types.UUID(t.TagID), Name: t.Name, Color: color,
		})
	}
	return &out
}

func (s *Store) ListDeals(ctx context.Context, in ListDealsInput) ([]crmcontracts.Deal, storekit.Page, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	active, err := s.activeColumns(ctx)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	pre, where, err := dealListQuery(ctx, in, active)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return storekit.RunListPage(ctx, s, pre, dealTable, dealColumns, active, where, scanDealPage,
		func(d crmcontracts.Deal) (time.Time, ids.UUID) { return d.CreatedAt, ids.UUID(d.Id) },
		func(tx pgx.Tx, page []crmcontracts.Deal) error {
			if err := finishDealPage(ctx, tx, page); err != nil {
				return err
			}
			return storekit.AttachRowTags(ctx, tx, dealTaggableType, page,
				func(d crmcontracts.Deal) ids.UUID { return ids.UUID(d.Id) },
				func(d *crmcontracts.Deal, tags []storekit.RowTag) { d.Tags = wireRowTags(tags) })
		})
}

// ListDealsTx is ListDeals inside a caller-opened transaction — the composite
// record reads, whose deal section must describe the same instant as its
// siblings. Same gate, same filters, same field-mask pass; the catalog answer
// is threaded in because a caller-opened read cannot fetch it without a
// second connection (ActiveDealColumns).
func (s *Store) ListDealsTx(ctx context.Context, tx pgx.Tx, in ListDealsInput, active CustomColumns) ([]crmcontracts.Deal, storekit.Page, error) {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return nil, storekit.Page{}, err
	}
	pre, where, err := dealListQuery(ctx, in, active.cols)
	if err != nil {
		return nil, storekit.Page{}, err
	}
	return storekit.RunListPageTx(ctx, tx, pre, dealTable, dealColumns, active.cols, where, scanDealPage,
		func(d crmcontracts.Deal) (time.Time, ids.UUID) { return d.CreatedAt, ids.UUID(d.Id) },
		func(tx pgx.Tx, page []crmcontracts.Deal) error { return finishDealPage(ctx, tx, page) })
}

// dealListQuery is the half of a deal list both entry points share: the sort
// refusal, the shared prelude and the deal's own filters.
func dealListQuery(ctx context.Context, in ListDealsInput, active []fieldcatalog.Column) (*storekit.ListPrelude, []string, error) {
	if err := refuseMaskedSort(ctx, in.Sort); err != nil {
		return nil, nil, err
	}
	pre, err := storekit.BuildListPrelude(ctx, "deal", dealListFields, active,
		in.Sort, in.Limit, in.Cursor, in.CustomFilters)
	if err != nil {
		return nil, nil, err
	}
	where, err := appendDealFilters(ctx, pre.Where(), in, pre.Arg)
	if err != nil {
		return nil, nil, err
	}
	return pre, where, nil
}

// scanDealPage drains one list query's rows: each deal plus, under a
// non-default sort, the row's cursor key (the trailing __cursor_key
// column CursorKeySuffix appended).
func scanDealPage(rows pgx.Rows, active []fieldcatalog.Column, sorted *storekit.ListSort) ([]crmcontracts.Deal, []*string, error) {
	var deals []crmcontracts.Deal
	var cursorKeys []*string
	for rows.Next() {
		var key *string
		extra := []any{}
		if sorted != nil {
			extra = append(extra, &key)
		}
		d, err := scanDeal(rows, active, extra...)
		if err != nil {
			return nil, nil, err
		}
		deals = append(deals, d)
		cursorKeys = append(cursorKeys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return deals, cursorKeys, nil
}

// appendDealFilters translates the caller's list filters — archived
// visibility, full-text query, the column equality filters, and the
// stalled predicate — into WHERE clauses (the cf_ filters and the keyset
// cursor, which depends on the validated sort, stay in ListDeals).
func appendDealFilters(ctx context.Context, where []string, in ListDealsInput, arg func(any) int) ([]string, error) {
	if !in.IncludeArchived {
		where = append(where, "archived_at IS NULL")
	}
	if in.Query != nil && *in.Query != "" {
		where = append(where, storekit.QuickFindClause(arg(*in.Query), dealNameColumn))
	}
	if in.PipelineID != nil {
		where = append(where, storekit.SQLf("pipeline_id = $%d", arg(*in.PipelineID)))
	}
	if in.StageID != nil {
		where = append(where, storekit.SQLf("stage_id = $%d", arg(*in.StageID)))
	}
	if in.OwnerID != nil {
		where = append(where, storekit.SQLf("owner_id = $%d", arg(*in.OwnerID)))
	}
	if clause := storekit.TagFilterClause(ctx, dealTaggableType, "deal.id", in.TagIDs, in.TagMode, arg); clause != "" {
		where = append(where, clause)
	}
	for _, ref := range []struct {
		column, table string
		id            *ids.UUID
	}{
		{filterCompanyID, "company", uuidOfFilter(in.CompanyID)},
		{filterProjectID, "project", uuidOfFilter(in.ProjectID)},
		{filterPartnerCompanyID, "company", uuidOfFilter(in.PartnerCompanyID)},
	} {
		if ref.id == nil {
			continue
		}
		clause, err := referenceFilterClause(ctx, ref.column, ref.table, *ref.id, arg)
		if err != nil {
			return nil, err
		}
		where = append(where, clause)
	}
	if in.PartnerSourced != nil {
		if *in.PartnerSourced {
			where = append(where, PartnerSourcedSQL(""))
		} else {
			where = append(where, "NOT "+PartnerSourcedSQL(""))
		}
	}
	if in.PartnerAttribution != nil {
		clause, err := partnerAttributionFilterClause(ctx, *in.PartnerAttribution, arg)
		if err != nil {
			return nil, err
		}
		where = append(where, clause)
	}
	if in.Status != nil {
		where = append(where, storekit.SQLf("status = $%d", arg(*in.Status)))
	}
	if in.ForecastCategory != nil {
		// A deal with no category is in no bucket, which the NULL comparison
		// gives for free: the five tiles partition the CATEGORISED pipeline,
		// and an uncategorised deal belongs to none of them rather than
		// quietly joining whichever one is asked for.
		where = append(where, storekit.SQLf("forecast_category = $%d", arg(*in.ForecastCategory)))
	}
	if in.CloseBefore != nil {
		where = append(where, storekit.SQLf("expected_close_date < $%d", arg(in.CloseBefore.Format(time.DateOnly))))
	}
	where = appendCommercialFilters(where, in, arg)
	if in.Stalled != nil {
		if *in.Stalled {
			where = append(where, StalledSQL(""))
		} else {
			where = append(where, "NOT "+StalledSQL(""))
		}
	}
	if in.QuietForDays != nil {
		where = append(where, QuietSQL("", *in.QuietForDays))
	}
	return where, nil
}

// uuidOfFilter widens one optional typed filter id to the untyped UUID the
// reference table above walks. It is deliberately the only widening here: the
// phantom kind is what stops a project id being probed against company.
func uuidOfFilter[K ids.EntityKind](id *ids.ID[K]) *ids.UUID {
	if id == nil {
		return nil
	}
	return &id.UUID
}

// referenceFilterClause narrows a filter on a column that NAMES another record
// to the rows whose target the caller may read.
//
// Filtering by an id is asking whether it is there. A bare
// `company_id = $1` answers that question for a company the caller
// cannot open — the same existence oracle the projection now withholds — so
// the arm carries the target's own visibility predicate. An empty page is the
// honest answer, and it is indistinguishable from a visible company that has
// no deals, which is what existence-hiding wants.
//
// An empty scope clause means a caller who reads every row of that table;
// there is nothing to narrow and the EXISTS would only confirm what the
// composite foreign key already guarantees.
//
// A role that WITHHOLDS the column is the other half, and it is refused rather
// than narrowed: the target is one this caller could open, so every arm here
// would answer, and the answer is the reference masked_fields declined to name.
func referenceFilterClause(ctx context.Context, column, table string, id ids.UUID, arg func(any) int) (string, error) {
	if err := auth.RefuseMaskedFilter(ctx, maskObject, column); err != nil {
		return "", err
	}
	pos := arg(id)
	scope, err := auth.ScopeClauseFor(ctx, table, "ref", arg)
	if err != nil {
		return "", err
	}
	clause := storekit.SQLf("%s = $%d", column, pos)
	if scope == "" {
		return clause, nil
	}
	return clause + storekit.SQLf(
		" AND EXISTS (SELECT 1 FROM %s ref WHERE ref.id = $%d AND %s)", table, pos, scope), nil
}

// partnerAttributionFilterClause narrows to what a partner did for the deal,
// and carries the partner's OWN visibility with it.
//
// Without that second half the filter is an existence oracle for the fact the
// field mask withholds: a caller whose read of a deal masks both partner
// columns could still ask for `partner_attribution=sourced` and learn from the
// row's presence that some partner brought it. The mask and the filter have to
// withhold the same fact, so this arm answers only for deals whose partner the
// caller could open.
func partnerAttributionFilterClause(ctx context.Context, attribution string, arg func(any) int) (string, error) {
	if err := validPartnerAttribution(attribution); err != nil {
		return "", err
	}
	// The claim travels with the partner, so a role withholding one withholds
	// both, and this arm answers for the rows a mask already declined to name.
	if err := auth.RefuseMaskedFilter(ctx, maskObject, filterPartnerAttribution); err != nil {
		return "", err
	}
	clause := storekit.SQLf("partner_attribution = $%d", arg(attribution))
	scope, err := auth.ScopeClauseFor(ctx, "company", "pref", arg)
	if err != nil {
		return "", err
	}
	if scope == "" {
		return clause, nil
	}
	return clause + storekit.SQLf(
		" AND EXISTS (SELECT 1 FROM company pref WHERE pref.id = partner_company_id AND %s)", scope), nil
}
