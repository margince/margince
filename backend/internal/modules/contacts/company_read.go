// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Reading one company: the three entry points a caller can arrive
// through, and the one row read they all share.
//
// Split from company.go because that file had grown past the 500-line
// cap carrying the create, the update, the replace-sets and this at once. The
// read is a concept of its own: it is where the computed fields are attached,
// where the archived filter is honoured, and where every column the record
// carries is scanned in one place.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// GetCompany reads one company under the caller's own gates: the
// object grant first, then row visibility, then the row itself. The grant is
// asked before the transaction as well as inside EnsureReadable, so a caller
// holding none costs no connection.
func (s *Store) GetCompany(ctx context.Context, id ids.CompanyID, archived storekit.ArchivedFilter) (crmcontracts.Company, error) {
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.Company{}, err
	}
	active, err := s.activeColumns(ctx, "company")
	if err != nil {
		return crmcontracts.Company{}, err
	}
	var out crmcontracts.Company
	err = s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = getCompanyInTx(ctx, tx, id, archived, active)
		return err
	})
	return out, err
}

// GetCompanyTx is GetCompany for a caller that already opened a
// transaction — the composite record read, which must see every one of its
// sections at the same instant and cannot afford a second connection per
// section. Same gates in the same order; only the transaction is borrowed.
//
// active is the caller's to fetch, with ActiveCompanyColumns, before it
// opens that transaction: the catalog read runs a transaction of its own, and a
// second connection taken from inside the caller's would commit separately and
// block undetectably against a lock the caller already holds.
func (s *Store) GetCompanyTx(ctx context.Context, tx pgx.Tx, id ids.CompanyID,
	archived storekit.ArchivedFilter, active CustomColumns,
) (crmcontracts.Company, error) {
	return getCompanyInTx(ctx, tx, id, archived, active.cols)
}

// getCompanyInTx is the shared body of the store-opened and
// caller-opened company reads.
func getCompanyInTx(ctx context.Context, tx pgx.Tx, id ids.CompanyID,
	archived storekit.ArchivedFilter, active []fieldcatalog.Column,
) (crmcontracts.Company, error) {
	if err := auth.EnsureReadable(ctx, tx, "company", id.UUID); err != nil {
		return crmcontracts.Company{}, err
	}
	out, err := readCompany(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	// Sampled ONCE for this read. The open-deal count and the computed
	// pipeline total come from the same rollup row, read at this date, so a
	// response never pairs a count from one FX day with a total from the next.
	asOf := rollupAsOf()
	// The contact count, on the single read too, so the page a list row opens
	// into agrees with the row. Attached here rather than in readCompany: a
	// write's before-image has no use for it.
	idx := map[openapi_types.UUID]*crmcontracts.Company{out.Id: &out}
	if err := fillContactCounts(ctx, tx, idx, []ids.UUID{id.UUID}); err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read company counts: %w", err)
	}
	// STATE-4: open_deal_count and the computed fields share one gate, the
	// same deal plus computed_field pair attachCompanyCounts applies to a list.
	// The gate is a pure permission check (no query). A caller without both
	// grants never pays for the rollup read, and both fields stay nil, so
	// omitempty drops their keys on marshal (T1). With both grants, one rollup
	// statement feeds both fields.
	if auth.ReadGranted(ctx, "computed_field") && auth.ReadGranted(ctx, "deal") {
		open, err := openPipelineRollup(ctx, tx, id, asOf)
		if err != nil {
			return crmcontracts.Company{}, fmt.Errorf("read open pipeline rollup: %w", err)
		}
		out.OpenDealCount = &open.dealCount
		rows := companyComputedFields(open)
		out.ComputedFields = &rows
	}
	return out, nil
}

// readCompanyRow is one company row with its children, and what a before-image
// or an internal check reads.
func readCompanyRow(ctx context.Context, tx pgx.Tx, id ids.CompanyID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Company, error) {
	q := `SELECT ` + companyColumns + storekit.SelectSuffix(active) + ` FROM company WHERE id = $1`
	if archived == storekit.LiveOnly {
		q += ` AND archived_at IS NULL`
	}
	o, err := scanCompany(tx.QueryRow(ctx, q, id), active)
	if errors.Is(err, pgx.ErrNoRows) {
		return crmcontracts.Company{}, apperrors.ErrNotFound
	}
	if err != nil {
		return crmcontracts.Company{}, err
	}
	companies := []crmcontracts.Company{o}
	if err := stampCompanyCapabilities(ctx, tx, companies); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := attachCompanyDomains(ctx, tx, companies); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := attachCompanyRelationshipTypes(ctx, tx, companies); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := attachCompanyPartners(ctx, tx, companies); err != nil {
		return crmcontracts.Company{}, err
	}
	return companies[0], nil
}

// scanCompany scans core + active custom columns; extra receives
// any trailing expressions the caller's SELECT appended (the sorted
// list's cursor key).
func scanCompany(row pgx.Row, active []fieldcatalog.Column, extra ...any) (crmcontracts.Company, error) {
	var o crmcontracts.Company
	var id ids.UUID
	var ownerID, parentID, mergedInto *ids.UUID
	var lifecycle string
	var addr crmcontracts.Address
	var resolvedCountries []string
	var logoObjectKey *string
	var linkedinURL, email *string
	var version int64
	var visibility string
	var assessAgainst *ids.UUID
	var sourceSystem, authorName, authorSeatName *string
	var authorID *ids.UUID

	dests := []any{
		&id, &o.DisplayName, &o.LegalName, &o.Description, &o.Industry, &o.SizeBand, &ownerID, &visibility,
		&addr.Line1, &addr.Line2, &addr.City, &addr.Region, &addr.PostalCode, &addr.Country,
		&resolvedCountries,
		&lifecycle, &parentID, &mergedInto, &logoObjectKey, &linkedinURL, &o.Phone, &email, &o.Source, &o.CapturedBy,
		&sourceSystem, &authorID, &authorName, &authorSeatName,
		&version, &o.CreatedAt, &o.UpdatedAt, &o.ArchivedAt, &o.IsAnchor,
		&o.LastActivityAt, &o.LegalHold, &assessAgainst,
	}
	cf := storekit.ScanDests(active)
	if err := row.Scan(append(append(dests, cf...), extra...)...); err != nil {
		return o, err
	}
	if values := storekit.ExtractValues(active, cf); len(values) > 0 {
		o.AdditionalProperties = values
	}

	o.Id = openapi_types.UUID(id)
	o.OwnerId = uuidPtr(ownerID)
	if v := crmcontracts.CompanyVisibility(visibility); v != "" {
		o.Visibility = &v
	}
	o.ParentCompanyId = uuidPtr(parentID)
	o.MergedIntoId = uuidPtr(mergedInto)
	lc := crmcontracts.CompanyLifecycle(lifecycle)
	o.Lifecycle = &lc
	o.LogoUrl = LogoURL(id, logoObjectKey, LogoWide)
	o.LinkedinUrl = linkedinURL
	if email != nil {
		o.Email = new(openapi_types.Email(*email))
	}
	o.AssessAgainstEntityId = uuidPtr(assessAgainst)
	if a := addressOrNil(addr); a != nil {
		o.Address = a
	}
	o.AddressCountryMismatch = addressCountryMismatch(addr.Country, resolvedCountries)
	o.Version = &version
	o.Author = sourceAuthorOf(authorID, authorSeatName, authorName, sourceSystem)
	o.ImportedFrom = provenance.ImportedFrom(sourceSystem)
	return o, nil
}
