// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/finance"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func (r approvalPreviewReader) companyMergeBusinessDetails(ctx context.Context, source, target ids.UUID) ([]crmcontracts.ApprovalPreviewDetail, error) {
	refs, err := r.companyMergeReferences(ctx, source)
	if err != nil {
		return nil, err
	}
	details := []crmcontracts.ApprovalPreviewDetail{}
	for _, ref := range refs {
		value, err := r.companyMergeReference(ctx, ref)
		if err != nil {
			return nil, err
		}
		details = append(details, value...)
		if ref.Type == datasource.EntityType(recordTypeCompany) {
			parent, err := r.companyMergeParent(ctx, source, target, ref.ID)
			if err != nil {
				return nil, err
			}
			details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "merge_company_parent_after", Value: parent})
		}
	}
	domains, err := contacts.NewStore(r.db).DomainTriageForCompany(ctx, ids.From[ids.CompanyKind](source))
	if err != nil {
		return nil, err
	}
	if len(domains) > 500 {
		return nil, apperrors.ErrInvalidArgument
	}
	for _, domain := range domains {
		if err := creationFieldsVisible(ctx, string(recordTypeCompany), map[string]any{previewFieldDomains: domain.Domain}); err != nil {
			return nil, err
		}
		details = append(details, previewDetail("merge_company_triaged_domain", domain.Domain))
	}
	return details, nil
}

func (r approvalPreviewReader) companyMergeParent(ctx context.Context, source, target, child ids.UUID) (crmcontracts.ApprovalPreviewValue, error) {
	if err := creationFieldsVisible(ctx, string(recordTypeCompany), map[string]any{refColParentCompany: source}); err != nil {
		return crmcontracts.ApprovalPreviewValue{}, err
	}
	parent := target
	if child == target {
		company, err := contacts.NewStore(r.db).GetCompany(ctx, ids.From[ids.CompanyKind](source), storekit.LiveOnly)
		if err != nil {
			return crmcontracts.ApprovalPreviewValue{}, err
		}
		if company.ParentCompanyId == nil {
			return readPreviewText(""), nil
		}
		parent = ids.UUID(*company.ParentCompanyId)
	}
	return r.referenceValue(ctx, refColCompany, parent.String())
}

func (r approvalPreviewReader) companyMergeReferences(ctx context.Context, source ids.UUID) ([]datasource.EntityRef, error) {
	if err := requireCompanyMergeReads(ctx); err != nil {
		return nil, err
	}
	var refs []datasource.EntityRef
	err := r.db.Tx(ctx, func(tx pgx.Tx) error {
		if err := auth.EnsureVisible(ctx, tx, mergeReadCompany, source); err != nil {
			return err
		}
		rows, err := tx.Query(ctx, `SELECT kind,id FROM (
 SELECT 'contract' kind,id FROM contract WHERE company_id=@source
 UNION ALL SELECT 'invoice',id FROM finance_invoice WHERE company_id=@source
 UNION ALL SELECT 'payment',id FROM finance_payment WHERE company_id=@source
 UNION ALL SELECT 'customer_link',id FROM finance_customer_link WHERE company_id=@source
 UNION ALL SELECT 'commission',id FROM commission_entry WHERE partner_company_id=@source
 UNION ALL SELECT 'offer',id FROM offer WHERE buyer_company_id=@source
 UNION ALL SELECT 'project',id FROM project WHERE company_id=@source
 UNION ALL SELECT 'deal',id FROM deal WHERE company_id=@source OR partner_company_id=@source
 UNION ALL SELECT 'company',id FROM company WHERE parent_company_id=@source
 ) affected ORDER BY kind,id LIMIT 501`, pgx.StrictNamedArgs{fieldSource: source})
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var ref datasource.EntityRef
			if err := rows.Scan(&ref.Type, &ref.ID); err != nil {
				return err
			}
			refs = append(refs, ref)
		}
		if err := rows.Err(); err != nil {
			return err
		}
		if len(refs) > 500 {
			return apperrors.ErrInvalidArgument
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return refs, nil
}

func (r approvalPreviewReader) companyMergeReference(ctx context.Context, ref datasource.EntityRef) ([]crmcontracts.ApprovalPreviewDetail, error) {
	if ref.Type == "invoice" || ref.Type == "payment" || ref.Type == "customer_link" {
		return r.mergeFinanceReference(ctx, ref)
	}
	var value crmcontracts.ApprovalPreviewValue
	var err error
	switch ref.Type {
	case datasource.EntityType(recordTypeOffer):
		offer, readErr := r.deals.GetOffer(ctx, ids.From[ids.OfferKind](ref.ID), storekit.IncludeArchived)
		if readErr != nil {
			return nil, readErr
		}
		if offer.OfferNumber == nil || *offer.OfferNumber == "" {
			return nil, apperrors.ErrInvalidArgument
		}
		value = crmcontracts.ApprovalPreviewValue{Kind: previewValueReference, Text: *offer.OfferNumber, EntityType: new(string(recordTypeOffer)), EntityId: new(openapi_types.UUID(ref.ID))}
	case datasource.EntityType(recordTypeCommission):
		record, readErr := r.records.Read(ctx, ref)
		if readErr != nil {
			return nil, readErr
		}
		var entry crmcontracts.CommissionEntry
		if err := json.Unmarshal(record.Fields, &entry); err != nil {
			return nil, err
		}
		value, err = r.referenceValue(ctx, dealIDField, entry.DealId.String())
		value.EntityType, value.EntityId = new(string(recordTypeCommission)), new(openapi_types.UUID(ref.ID))
	default:
		value, err = r.communicationEvidence(ctx, string(ref.Type)+"_id", ref.ID)
	}
	if err != nil {
		return nil, err
	}
	return []crmcontracts.ApprovalPreviewDetail{{Field: "merge_company_" + string(ref.Type), Value: value}}, nil
}

func (r approvalPreviewReader) mergeFinanceReference(ctx context.Context, ref datasource.EntityRef) ([]crmcontracts.ApprovalPreviewDetail, error) {
	entry, err := finance.NewStore(r.db, FinanceRates()).MergeReference(ctx, string(ref.Type), ref.ID)
	if err != nil {
		return nil, err
	}
	account, err := r.referenceValue(ctx, refColCompany, entry.CompanyID.String())
	if err != nil {
		return nil, err
	}
	label := account.Text
	if entry.Number != "" {
		label += " · " + entry.Number
	}
	detail := mergeIdentityDetail("merge_company_"+string(ref.Type), label, ref.ID)
	return []crmcontracts.ApprovalPreviewDetail{detail, previewMoment("merge_payment_at", entry.PaidAt), previewMoment("merge_finance_archived", entry.ArchivedAt)}, nil
}
