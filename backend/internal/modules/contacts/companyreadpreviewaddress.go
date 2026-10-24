// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"fmt"
	"regexp"
	"strconv"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

var sqlPlaceholder = regexp.MustCompile(`\$(\d+)`)

// previewReadAddress shows the address columns a read would fill, one row per
// part the split found. An address the split refuses fills nothing, so it
// shows nothing here; its profile-field row still shows the whole text.
func previewReadAddress(ctx context.Context, tx pgx.Tx, id ids.CompanyID, current *crmcontracts.Address, field ColdStartFieldInput) ([]CompanyReadValue, error) {
	authority, value, err := unclaimedCompanyColumn(ctx, tx, id, columnAddress, field.Value)
	if err != nil || value == "" {
		return nil, err
	}
	proposed, split := SplitAddressText(value)
	if !split {
		return nil, nil
	}
	applies, err := companyColumnWriteApplies(ctx, tx, columnAddress, authority, addressWriteArguments(id, proposed))
	if err != nil {
		return nil, err
	}
	stored := addressColumnParts(addressColumns(current))
	var out []CompanyReadValue
	for i, part := range addressColumnParts(proposed) {
		if part.value == "" {
			continue
		}
		if err := previewFieldsVisible(ctx, companyEntity, part.column); err != nil {
			return nil, err
		}
		before := stored[i].value
		after := before
		if applies {
			after = part.value
		}
		out = append(out, CompanyReadValue{
			Field: part.column, Before: before, After: after, Proposed: part.value,
			Retained: !applies, Quote: field.EvidenceSnippet, SourceURL: field.SourceURL,
		})
	}
	return out, nil
}

// previewNewCompanyAddress is previewReadAddress for a company the read would
// create, which has no address to keep.
func previewNewCompanyAddress(field ColdStartFieldInput) []CompanyReadValue {
	proposed, split := SplitAddressText(field.Value)
	if !split {
		return nil
	}
	var out []CompanyReadValue
	for _, part := range addressColumnParts(proposed) {
		if part.value != "" {
			out = append(out, CompanyReadValue{Field: part.column, After: part.value, Proposed: part.value, SourceURL: field.SourceURL, Quote: field.EvidenceSnippet})
		}
	}
	return out
}

type addressColumnPart struct {
	column string
	value  string
}

// addressColumnParts pairs each address column with its part of a, in the
// order companyAddressColumns names them.
func addressColumnParts(a crmcontracts.Address) []addressColumnPart {
	values := map[string]*string{
		columnAddressLine1: a.Line1, columnAddressLine2: a.Line2, columnAddressCity: a.City,
		columnAddressRegion: a.Region, columnAddressPostalCode: a.PostalCode, columnAddressCountry: a.Country,
	}
	out := make([]addressColumnPart, 0, len(companyAddressColumns))
	for _, column := range companyAddressColumns {
		out = append(out, addressColumnPart{column: column, value: pointerValue(values[column])})
	}
	return out
}

// previewCompanyReadTx establishes row visibility before individual-column reads.
// Writer predicates keep the preview aligned with the change the writer permits.
func companyColumnWriteApplies(ctx context.Context, tx pgx.Tx, column string, authority companyWriteAuthority, arguments []any) (bool, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return false, err
	}
	predicate, found := companyColumnReadPredicates[column][authority]
	if !found {
		return false, apperrors.ErrInvalidArgument
	}
	used := 0
	for _, m := range sqlPlaceholder.FindAllStringSubmatch(predicate, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			return false, fmt.Errorf("read placeholder in %s predicate: %w", column, err)
		}
		used = max(used, n)
	}
	if used > len(arguments) {
		return false, fmt.Errorf("the %s predicate names $%d and the preview binds %d values", column, used, len(arguments))
	}
	var applies bool
	err := tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM company WHERE "+predicate+")", arguments[:used]...).Scan(&applies)
	return applies, err
}
