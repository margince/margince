// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type ColdStartPreview struct {
	Read            CompanyReadPreview
	Name            string
	Domain          string
	Creates         bool
	QueuesVATCheck  bool
	ReviewCompanyID *ids.CompanyID
}

func (s *Store) PreviewColdStart(ctx context.Context, in ApplyColdStartProfileInput) (ColdStartPreview, error) {
	var out ColdStartPreview
	if err := auth.Require(ctx, "company", principal.ActionUpdate); err != nil {
		return out, err
	}
	if err := auth.Require(ctx, "company", principal.ActionRead); err != nil {
		return out, err
	}
	host, err := coldStartHost(in.SourceURL)
	if err != nil || len(in.Fields) == 0 || len(in.Fields) > 500 {
		return out, apperrors.ErrInvalidArgument
	}
	if err := previewFieldsVisible(ctx, "company", "display_name", "primary_domain"); err != nil {
		return out, err
	}
	candidate, _ := coldStartCandidate(host, in.Fields)
	out.Name, out.Domain = candidate.DisplayName, host
	out.QueuesVATCheck = statesAVatNumber(in.Fields)
	active, err := s.activeColumns(ctx, "company")
	if err != nil {
		return out, err
	}
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		match, err := DedupeCompany(ctx, tx, candidate)
		if err != nil {
			return err
		}
		if match.Decision == DecisionExactCollision {
			out.Read, err = previewCompanyReadTx(ctx, tx, DeepReadProposal{CompanyID: match.CompanyID, SourceURL: in.SourceURL}, in.Fields, active)
			return err
		}
		if err := auth.Require(ctx, "company", principal.ActionCreate); err != nil {
			return err
		}
		out.Creates = true
		if match.Decision == DecisionFuzzyReview {
			if err := auth.EnsureVisible(ctx, tx, "company", match.CompanyID.UUID); err != nil {
				return err
			}
			out.ReviewCompanyID = &match.CompanyID
		}
		out.Read.Profiles, err = previewNewCompanyReadFields(ctx, in.Fields)
		if err != nil {
			return err
		}
		out.Read.Columns = previewNewCompanyReadColumns(in.Fields)
		return nil
	})
	if err != nil {
		return ColdStartPreview{}, err
	}
	return out, nil
}

func previewNewCompanyReadFields(ctx context.Context, fields []ColdStartFieldInput) ([]CompanyReadValue, error) {
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
		out = append(out, CompanyReadValue{Field: field.Field, Proposed: field.Value, After: field.Value, Quote: field.EvidenceSnippet, SourceURL: field.SourceURL})
	}
	return out, nil
}

func previewNewCompanyReadColumns(fields []ColdStartFieldInput) []CompanyReadValue {
	var out []CompanyReadValue
	for _, field := range fields {
		column, backed := columnBackedColdStartFields[field.Field]
		if !backed || field.Value == "" {
			continue
		}
		value := field.Value
		if column == columnDescription {
			value = headerLine(value)
		}
		if column == columnAddress {
			column = "address_line1"
		}
		out = append(out, CompanyReadValue{Field: column, After: value, Proposed: value, SourceURL: field.SourceURL, Quote: field.EvidenceSnippet})
	}
	return out
}
