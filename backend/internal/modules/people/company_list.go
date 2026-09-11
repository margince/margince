// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

// The company list read: the shared listPage runner bound to the
// company table — DM-VOCAB-2 sort vocabulary, the shared filter
// chain plus the classification filter, and the company row scan +
// domain attachment.

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// companyEntity is the company's auth object and table name.
const companyEntity = "company"

// companyNameColumn is the company's display column — the quick-find
// target and the DM-VOCAB-2 name sort key.
const companyNameColumn = "display_name"

// ListCompaniesInput carries the company list's contract
// parameters.
type ListCompaniesInput struct {
	// TagIDs narrows to the accounts carrying these tags, combined by TagMode.
	// The predicate is storekit's, shared with the person and deal lists.
	TagIDs  []ids.UUID
	TagMode storekit.TagMode
	// IncludeAnchor admits the installation's own company (ADR-0082/A127).
	IncludeAnchor bool
	Cursor        *string
	Limit         *int
	Query         *string
	OwnerID       *ids.UserID
	// OwnerTeamID narrows to a team's rows; Unassigned to the unowned queue.
	// Both narrow the caller's row scope and never widen it — see
	// listFilters.ownershipClause, which also refuses two of them at once.
	OwnerTeamID *ids.TeamID
	Unassigned  *bool
	// Classification is RETIRED with the column (ADR-0079/A124) and reaches no
	// wire parameter; Lifecycle and RelationshipType replace it.
	Classification   *string
	Lifecycle        *string
	RelationshipType *string
	// Industry is free text on the record; SizeBand is the contract's enum.
	Industry *string
	SizeBand *string
	// Domain narrows to the account that lists one domain — the
	// employer-inference lookup, spelled as a filter over the same
	// company_domain rows the page attaches.
	Domain          *string
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
}

// companyListFields is the company list's core sortable
// vocabulary — exactly the data-model §13.5 DM-VOCAB-2 set; active cf_
// columns join it per request.
var companyListFields = map[string]storekit.SortField{
	createdAtColumn:    storekit.Column(storekit.KindTimestamp),
	updatedAtColumn:    storekit.Column(storekit.KindTimestamp),
	companyNameColumn:  storekit.Column(fieldcatalog.TypeText),
	ownerIDColumn:      storekit.Column(storekit.KindUUID),
	lastActivityColumn: storekit.Column(storekit.KindTimestamp),
}

// companyDomainClause narrows the page to the account that lists one
// domain, or "" when the caller named none.
//
// The value is reduced by the SAME parse the write path applies
// (parseCompanyDomains → values.ParseDomain), so a caller who pasted
// `https://www.acme.example/careers` out of a signature asks about the host
// the column actually holds. A value that reduces to no host at all matches
// nothing, which is the honest answer to a domain that is not one — the
// caller asked about an account by something no account can list.
//
// Domain-row liveness follows the caller's own archived question. A live page
// compares against live domain rows: the account that holds the domain NOW is
// the one a lookup means, and `uq_company_domain` indexes exactly those. But
// archiving an account archives its domain rows in the same transaction, so
// pinning liveness here would make `include_archived=true&domain=…` a page
// that can never contain the archived account that held it — an empty answer
// reading "nobody ever had this", which is the same confident wrong answer
// this filter exists to prevent, inverted.
//
// EXISTS rather than a join: an account lists several domains, and a join
// would return it once per row the keyset cursor would then page over as if
// they were distinct accounts.
func companyDomainClause(domain *string, includeArchived bool, arg func(any) int) string {
	if domain == nil {
		return ""
	}
	live := " AND d.archived_at IS NULL"
	if includeArchived {
		live = ""
	}
	return storekit.SQLf(`EXISTS (
		SELECT 1 FROM company_domain d
		WHERE d.company_id = company.id
		  AND d.domain = $%d`+live+`)`,
		arg(foldDomainQuery(*domain)))
}

// foldDomainQuery reduces a caller's domain to the host company_domain
// stores, or to a value no row can carry when it names no host. It answers a
// string rather than a parse error because a filter that cannot match is a
// page with nothing in it, not a request the surface should refuse: the
// caller named an account by something, and no account has it.
func foldDomainQuery(raw string) string {
	parsed, err := values.ParseDomain(raw)
	if err != nil {
		return ""
	}
	return parsed.String()
}

// ListCompanies is the row-scoped company list read:
// quick-find, owner, domain, classification and custom-field filters,
// keyset pagination under the validated sort.
// companyCommonFilters is the shared filter set for an account list: every dial
// the person and lead lists also carry, plus the domain an account is found by.
//
// Named for what it holds rather than for its list, because the rename put it
// next to listfilters.go's companyListFilters — the wire-name FilterSet — and
// two things called the same thing in one package is the defect this rename
// exists to remove.
func companyCommonFilters(in ListCompaniesInput) listFilters {
	return listFilters{
		IncludeArchived: in.IncludeArchived,
		CapturedByKind:  in.CapturedByKind,
		AiWritten:       in.AiWritten,
		entity:          companyEntity,
		OwnerID:         in.OwnerID,
		OwnerTeamID:     in.OwnerTeamID,
		Unassigned:      in.Unassigned,
		Query:           in.Query,
		Cursor:          in.Cursor,
		CustomFilters:   in.CustomFilters,
		nameColumn:      companyNameColumn,
		identifier: storekit.Identifier{
			Table: "company_domain", FK: companyFK, Column: domainColumn,
		},
	}
}

// ListCompanies answers one page of accounts under the caller's row scope,
// filtered by the dials the contract declares and ordered by the shared sort.
func (s *Store) ListCompanies(ctx context.Context, in ListCompaniesInput) ([]crmcontracts.Company, storekit.Page, error) {
	shared := companyCommonFilters(in)
	return listPage(ctx, s, in.Sort, in.Limit, listPageSpec[crmcontracts.Company]{
		entity:  companyEntity,
		columns: companyColumns,
		fields:  companyListFields,
		filters: func(active []fieldcatalog.Column, sorted *storekit.ListSort, arg func(any) int) ([]string, error) {
			where, err := shared.clauses(active, sorted, arg)
			if err != nil {
				return nil, err
			}
			// The installation's own company is not one of the accounts this
			// list answers about, so it is excluded unless asked for
			// (ADR-0082/A127). Appended here beside the other company-only
			// filters rather than in the shared set: the anchor is a fact about
			// companies, and no person or deal has one.
			if !in.IncludeAnchor {
				where = append(where, "NOT is_anchor")
			}
			if in.Classification != nil {
				where = append(where, storekit.SQLf("classification = $%d", arg(*in.Classification)))
			}
			where = appendCompanyLinkClauses(ctx, where, in, arg)
			// A value outside the enum is a client mistake, not a selection
			// that happens to match nothing: answering 200 with an empty page
			// tells the reader this account list is empty when the question
			// was never one the contract accepts. Validated HERE, inside the
			// store, so it lands after listPage's auth.Require rather than
			// before it.
			if in.Lifecycle != nil {
				if !crmcontracts.ListCompaniesParamsLifecycle(*in.Lifecycle).Valid() {
					return nil, httperr.Validation("lifecycle", "not_a_known_value",
						"filter by one of the account stages the contract defines, or leave the parameter off")
				}
				where = append(where, storekit.SQLf("lifecycle = $%d", arg(*in.Lifecycle)))
			}
			if in.Industry != nil {
				// Free text on the record, so matched as written rather than
				// against a vocabulary the column does not have.
				where = append(where, storekit.SQLf("industry = $%d", arg(*in.Industry)))
			}
			if in.SizeBand != nil {
				// Checked against the generated enum for the same reason
				// lifecycle is: an unknown band would answer an empty page,
				// and empty reads as "no accounts that size" rather than as
				// "that is not a size".
				if !crmcontracts.ListCompaniesParamsSizeBand(*in.SizeBand).Valid() {
					return nil, httperr.Validation("size_band", "not_a_known_value",
						"filter by one of the size bands the contract defines, or leave the parameter off")
				}
				where = append(where, storekit.SQLf("size_band = $%d", arg(*in.SizeBand)))
			}
			if in.RelationshipType != nil {
				if !crmcontracts.ListCompaniesParamsRelationshipType(*in.RelationshipType).Valid() {
					return nil, httperr.Validation("relationship_type", "not_a_known_value",
						"filter by one of the relationship types the contract defines, or leave the parameter off")
				}
				// EXISTS, not a join: an account carries several types and a
				// join would return it once per matching row, which the keyset
				// cursor would then page over as if they were distinct records.
				where = append(where, storekit.SQLf(`EXISTS (
					SELECT 1 FROM company_relationship_type rt
					WHERE rt.company_id = company.id
					  AND rt.relationship_type = $%d AND rt.archived_at IS NULL)`,
					arg(*in.RelationshipType)))
			}
			return where, nil
		},
		scan: scanCompanyPage,
		attach: func(ctx context.Context, tx pgx.Tx, companies []crmcontracts.Company) error {
			if err := attachCompanyDomains(ctx, tx, companies); err != nil {
				return err
			}
			if err := attachCompanyRelationshipTypes(ctx, tx, companies); err != nil {
				return err
			}
			if err := storekit.AttachRowTags(ctx, tx, companyEntity, companies,
				func(o crmcontracts.Company) ids.UUID { return ids.UUID(o.Id) },
				func(o *crmcontracts.Company, tags []storekit.RowTag) { o.Tags = wireRowTags(tags) }); err != nil {
				return err
			}
			// One sample for the page, for the reason the single read binds
			// one: every row on it is counted against the same day.
			return attachCompanyCounts(ctx, tx, companies, rollupAsOf())
		},
		cursorKey: func(last crmcontracts.Company) (time.Time, ids.UUID) {
			return last.CreatedAt, ids.UUID(last.Id)
		},
	})
}

// scanCompanyPage drains one list query's rows: each company
// plus, under a non-default sort, the row's cursor key (the trailing
// __cursor_key column CursorKeySuffix appended).
func scanCompanyPage(rows pgx.Rows, active []fieldcatalog.Column, sorted *storekit.ListSort) ([]crmcontracts.Company, []*string, error) {
	var companies []crmcontracts.Company
	var cursorKeys []*string
	for rows.Next() {
		var key *string
		extra := []any{}
		if sorted != nil {
			extra = append(extra, &key)
		}
		o, err := scanCompany(rows, active, extra...)
		if err != nil {
			return nil, nil, err
		}
		companies = append(companies, o)
		cursorKeys = append(cursorKeys, key)
	}
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	return companies, cursorKeys, nil
}

// appendCompanyLinkClauses adds the narrowings that reach ANOTHER table — a
// domain row and a tagging — rather than a column on company. They are
// grouped because they share that shape, and because the list they sit in has
// a size ceiling that a third one would push through.
func appendCompanyLinkClauses(
	ctx context.Context, where []string, in ListCompaniesInput, arg func(any) int,
) []string {
	if clause := companyDomainClause(in.Domain, in.IncludeArchived, arg); clause != "" {
		where = append(where, clause)
	}
	if clause := storekit.TagFilterClause(
		ctx, companyEntity, "company.id", in.TagIDs, in.TagMode, arg,
	); clause != "" {
		where = append(where, clause)
	}
	return where
}
