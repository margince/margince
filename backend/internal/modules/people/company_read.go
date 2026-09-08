// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

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
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// GetCompany reads one company under the caller's own gates: the
// object grant first, then row visibility, then the row itself.
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
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return crmcontracts.Company{}, err
	}
	return getCompanyInTx(ctx, tx, id, archived, active.cols)
}

// getCompanyInTx is the shared body of the store-opened and
// caller-opened company reads.
func getCompanyInTx(ctx context.Context, tx pgx.Tx, id ids.CompanyID,
	archived storekit.ArchivedFilter, active []fieldcatalog.Column,
) (crmcontracts.Company, error) {
	if err := auth.EnsureVisible(ctx, tx, "company", id.UUID); err != nil {
		return crmcontracts.Company{}, err
	}
	out, err := readCompany(ctx, tx, id, archived, active)
	if err != nil {
		return crmcontracts.Company{}, err
	}
	// Sampled ONCE for this read, and handed to both rollup readers below. The
	// row count and the computed pipeline total are two queries against the
	// same function, and a clock read separately by each would pick different
	// FX dates for one response whenever UTC midnight fell between them —
	// which is the divergence binding the date exists to remove, reappearing
	// one level down.
	asOf := rollupAsOf()
	// The two list-row counts, on the single read too, so the page a row
	// opens into agrees with the row. Attached here rather than in
	// readCompany: a write's before-image has no use for them.
	single := []crmcontracts.Company{out}
	if err := attachCompanyCounts(ctx, tx, single, asOf); err != nil {
		return crmcontracts.Company{}, fmt.Errorf("read company counts: %w", err)
	}
	out = single[0]
	// STATE-4: the gate is a pure permission check (no query), so a
	// caller whose role lacks computed_field:read never pays for the
	// rollup read below, and out.ComputedFields stays its nil zero
	// value — omitempty then drops the key entirely on marshal (T1).
	if computedFieldsVisible(ctx) {
		open, err := openPipelineRollup(ctx, tx, id, asOf)
		if err != nil {
			return crmcontracts.Company{}, fmt.Errorf("read open pipeline rollup: %w", err)
		}
		rows := companyComputedFields(open)
		out.ComputedFields = &rows
	}
	return out, nil
}

func readCompany(ctx context.Context, tx pgx.Tx, id ids.CompanyID, archived storekit.ArchivedFilter, active []fieldcatalog.Column) (crmcontracts.Company, error) {
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
	// Stamped HERE rather than only in attachCompanyCounts, because every mutation
	// echo — create, update, archive, restore, the merge survivor — hands the
	// row back through this function and not through the counts page. Absent
	// reads as NOT writable, so a missing stamp takes the edit buttons away from
	// the owner who just saved.
	if _, err := auth.StampWritable(ctx, tx, "company", companies,
		func(o crmcontracts.Company) ids.UUID { return ids.UUID(o.Id) },
		func(o *crmcontracts.Company, may bool) { o.Writable = &may }); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := attachCompanyDomains(ctx, tx, companies); err != nil {
		return crmcontracts.Company{}, err
	}
	if err := attachCompanyRelationshipTypes(ctx, tx, companies); err != nil {
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
	var classification, lifecycle string
	var relevance *int16
	var addr crmcontracts.Address
	var logoObjectKey *string
	var linkedinURL *string
	var version int64
	var visibility string

	dests := []any{
		&id, &o.DisplayName, &o.LegalName, &o.Description, &o.Industry, &o.SizeBand, &ownerID, &visibility,
		&addr.Line1, &addr.Line2, &addr.City, &addr.Region, &addr.PostalCode, &addr.Country,
		&classification, &lifecycle, &relevance, &parentID, &mergedInto, &logoObjectKey, &linkedinURL, &o.Source, &o.CapturedBy,
		&version, &o.CreatedAt, &o.UpdatedAt, &o.ArchivedAt, &o.IsAnchor,
		&o.LastActivityAt,
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
	cls := crmcontracts.CompanyClassification(classification)
	o.Classification = &cls
	lc := crmcontracts.CompanyLifecycle(lifecycle)
	o.Lifecycle = &lc
	o.LogoUrl = LogoURL(id, logoObjectKey, LogoWide)
	o.LinkedinUrl = linkedinURL
	if a := addressOrNil(addr); a != nil {
		o.Address = a
	}
	o.Version = &version
	return o, nil
}
