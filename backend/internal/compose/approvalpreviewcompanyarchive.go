// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func (r approvalPreviewReader) archiveChildDetails(ctx context.Context, kind string, raw json.RawMessage) ([]crmcontracts.ApprovalPreviewDetail, error) {
	if kind == "contact" {
		return contactArchiveDetails(raw)
	}
	var company crmcontracts.Company
	if err := json.Unmarshal(raw, &company); err != nil {
		return nil, err
	}
	if company.Domains == nil || company.RelationshipTypes == nil {
		return nil, apperrors.ErrInvalidArgument
	}
	var out []crmcontracts.ApprovalPreviewDetail
	for _, domain := range *company.Domains {
		out = append(out, previewDetail("archive_domain", domain.Domain))
	}
	for _, relation := range *company.RelationshipTypes {
		out = append(out, previewDetail("archive_company_type", string(relation)))
		if relation == "partner" {
			if _, err := contacts.NewStore(r.db).GetPartner(ctx, ids.From[ids.CompanyKind](ids.UUID(company.Id))); err != nil {
				return nil, err
			}
			out = append(out, previewDetail("archive_partner", company.DisplayName))
		}
	}
	return out, nil
}
