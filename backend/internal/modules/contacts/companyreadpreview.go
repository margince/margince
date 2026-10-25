// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/fieldcatalog"
)

// CompanyReadValue distinguishes proposed evidence from the value the writer would retain.
type CompanyReadValue struct {
	Field     string
	Category  string
	Before    string
	After     string
	Proposed  string
	SourceURL string
	Quote     string
	Retained  bool
}

// CompanyReadPreview groups authorized enrichment effects with the current company.
type CompanyReadPreview struct {
	Company  crmcontracts.Company
	Columns  []CompanyReadValue
	Profiles []CompanyReadValue
	Facts    []CompanyReadValue
}

// PreviewCompanyRead resolves authorized effects without applying the proposed change.
func (s *Store) PreviewCompanyRead(ctx context.Context, in DeepReadProposal) (CompanyReadPreview, error) {
	var out CompanyReadPreview
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionUpdate); err != nil {
		return out, err
	}
	fields, err := validatedDeepReadFields(in)
	if err != nil {
		return out, apperrors.ErrInvalidArgument
	}
	if len(fields)+len(in.Facts) > 500 {
		return out, apperrors.ErrInvalidArgument
	}
	active, err := s.activeColumns(ctx, companyEntity)
	if err != nil {
		return out, err
	}
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		var readErr error
		out, readErr = previewCompanyReadTx(ctx, tx, in, fields, active)
		return readErr
	})
	if err != nil {
		return CompanyReadPreview{}, err
	}
	return out, nil
}

func previewReadColumns(ctx context.Context, tx pgx.Tx, id ids.CompanyID, current crmcontracts.Company, fields []ColdStartFieldInput) ([]CompanyReadValue, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(current)
	if err != nil {
		return nil, err
	}
	var values map[string]json.RawMessage
	if err := json.Unmarshal(raw, &values); err != nil {
		return nil, err
	}
	var out []CompanyReadValue
	for _, field := range fields {
		column, backed := columnBackedColdStartFields[field.Field]
		if !backed {
			continue
		}
		if column == columnAddress {
			rows, err := previewReadAddress(ctx, tx, id, current.Address, field)
			if err != nil {
				return nil, err
			}
			out = append(out, rows...)
			continue
		}
		stored := column
		if err := previewFieldsVisible(ctx, companyEntity, stored); err != nil {
			return nil, err
		}
		authority, value, err := unclaimedCompanyColumn(ctx, tx, id, column, field.Value)
		if err != nil {
			return nil, err
		}
		if value == "" {
			continue
		}
		// The preview asks the question the write will ask, with the value the
		// write will bind: text that names no industry code writes nothing.
		bound, unmapped := companyColumnArgument(column, value)
		applies := false
		if !unmapped {
			applies, err = companyColumnWriteApplies(ctx, tx, column, authority, []any{id, bound})
			if err != nil {
				return nil, err
			}
		}
		before, err := companyReadColumnValue(values[stored])
		if err != nil {
			return nil, err
		}
		after := before
		if applies {
			after = deref(bound)
		}
		out = append(out, CompanyReadValue{Field: stored, Before: before, After: after, Proposed: value, Retained: !applies, Quote: field.EvidenceSnippet, SourceURL: field.SourceURL})
	}
	return out, nil
}

func appendReadSizeBand(ctx context.Context, out *CompanyReadPreview) error {
	if out.Company.SizeBand != nil {
		return nil
	}
	for _, fact := range out.Facts {
		if fact.Category != factCategoryCompany || fact.Field != FactEmployeeRange || fact.Retained {
			continue
		}
		band, ok := sizeBandFromEmployeeRange(fact.Proposed)
		if !ok {
			return nil
		}
		if err := previewFieldsVisible(ctx, companyEntity, filterSizeBand); err != nil {
			return err
		}
		out.Columns = append(out.Columns, CompanyReadValue{Field: filterSizeBand, After: band, Proposed: band, Quote: fact.Quote, SourceURL: fact.SourceURL})
		return nil
	}
	return nil
}

func previewCompanyReadTx(ctx context.Context, tx pgx.Tx, in DeepReadProposal, fields []ColdStartFieldInput, active []fieldcatalog.Column) (CompanyReadPreview, error) {
	var out CompanyReadPreview
	if err := previewFieldsVisible(ctx, companyEntity, "display_name"); err != nil {
		return out, err
	}
	if err := auth.EnsureWritableLive(ctx, tx, companyEntity, in.CompanyID.UUID); err != nil {
		return out, err
	}
	current, err := readCompany(ctx, tx, in.CompanyID, storekit.LiveOnly, active)
	if err != nil {
		return out, err
	}
	out.Company = current
	out.Columns, err = previewReadColumns(ctx, tx, in.CompanyID, current, fields)
	if err != nil {
		return out, err
	}
	out.Profiles, err = previewReadProfiles(ctx, tx, in.CompanyID, fields)
	if err != nil {
		return out, err
	}
	out.Facts, err = previewReadFacts(ctx, tx, in)
	if err != nil {
		return out, err
	}
	if err := appendReadSizeBand(ctx, &out); err != nil {
		return out, err
	}
	return out, nil
}

func companyReadColumnValue(raw json.RawMessage) (string, error) {
	if len(raw) == 0 || string(raw) == "null" {
		return "", nil
	}
	var value string
	err := json.Unmarshal(raw, &value)
	return value, err
}
