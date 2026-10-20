// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func previewReadProfiles(ctx context.Context, tx pgx.Tx, id ids.CompanyID, fields []ColdStartFieldInput) ([]CompanyReadValue, error) {
	out := make([]CompanyReadValue, 0, len(fields))
	seen := make(map[string]bool, len(fields))
	for _, field := range fields {
		if seen[field.Field] {
			return nil, apperrors.ErrInvalidArgument
		}
		seen[field.Field] = true
		if err := previewFieldsVisible(ctx, "company", field.Field); err != nil {
			return nil, err
		}
		var before string
		replaceable := true
		err := tx.QueryRow(ctx, `SELECT value, `+companyProfileReplaceableSQL+` FROM company_profile_field WHERE company_id=@company AND field=@field`, pgx.NamedArgs{"company": id, "field": field.Field}).Scan(&before, &replaceable)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		after := before
		if replaceable {
			after = field.Value
		}
		out = append(out, CompanyReadValue{Field: field.Field, Before: before, After: after, Proposed: field.Value, SourceURL: field.SourceURL, Quote: field.EvidenceSnippet, Retained: !replaceable})
	}
	return out, nil
}

func previewReadFacts(ctx context.Context, tx pgx.Tx, in DeepReadProposal) ([]CompanyReadValue, error) {
	out := make([]CompanyReadValue, 0, len(in.Facts))
	seen := make(map[[3]string]bool, len(in.Facts))
	for _, fact := range in.Facts {
		key := [3]string{fact.Category, fact.Field, fact.ValueKey}
		if seen[key] {
			return nil, apperrors.ErrInvalidArgument
		}
		seen[key] = true
		if err := previewFieldsVisible(ctx, "company", fact.Field); err != nil {
			return nil, err
		}
		var before string
		replaceable := true
		err := tx.QueryRow(ctx, `SELECT value, `+companyFactReplaceableSQL+` FROM company_fact WHERE company_id=@company AND category=@category AND field=@field AND value_key=@key`, pgx.NamedArgs{"company": in.CompanyID, "category": fact.Category, "field": fact.Field, "key": fact.ValueKey}).Scan(&before, &replaceable)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
		after := before
		if replaceable {
			after = fact.Value
		}
		out = append(out, CompanyReadValue{Field: fact.Field, Category: fact.Category, Before: before, After: after, Proposed: fact.Value, SourceURL: fact.SourceURL, Quote: fact.EvidenceSnippet, Retained: !replaceable})
	}
	return out, nil
}
