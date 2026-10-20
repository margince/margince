// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"maps"
	"sort"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/customfields"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

func creationActor(approval crmcontracts.Approval) (principal.Principal, error) {
	kind, key, found := strings.Cut(approval.ProposedBy, ":")
	if !found || key == "" {
		return principal.Principal{}, apperrors.ErrInvalidArgument
	}
	actor := principal.Principal{ID: approval.ProposedBy}
	switch kind {
	case "human":
		id, err := ids.Parse(key)
		if err != nil || id.IsZero() {
			return actor, apperrors.ErrInvalidArgument
		}
		actor.Type, actor.UserID = principal.PrincipalHuman, id
	case "agent":
		if approval.OnBehalfOf == nil || ids.UUID(*approval.OnBehalfOf).IsZero() {
			return actor, apperrors.ErrInvalidArgument
		}
		actor.Type, actor.UserID = principal.PrincipalAgent, ids.UUID(*approval.OnBehalfOf)
		actor.OnBehalfOf = actor.UserID
	case "system":
		actor.Type = principal.PrincipalSystem
	case "connector":
		actor.Type = principal.PrincipalConnector
	default:
		return actor, apperrors.ErrInvalidArgument
	}
	return actor, nil
}

func creationRequest[T any](approval crmcontracts.Approval, recordType string) (T, bool, error) {
	var empty T
	if approval.ProposedChange == nil || approval.TargetEntityType == nil || *approval.TargetEntityType != recordType {
		return empty, false, apperrors.ErrInvalidArgument
	}
	if _, rest := (*approval.ProposedChange)["operation"]; rest {
		req, err := commercialRequest[T](approval)
		return req, false, err
	}
	envelope, err := commercialRequest[struct {
		RecordType string          `json:"record_type"`
		Fields     json.RawMessage `json:"fields"`
	}](approval)
	if err != nil {
		return empty, true, err
	}
	if envelope.RecordType != recordType {
		return empty, true, apperrors.ErrInvalidArgument
	}
	var req T
	if err := datasource.StrictDecode(envelope.Fields, &req); err != nil {
		return empty, true, err
	}
	return req, true, nil
}

func leadCreationRequest(approval crmcontracts.Approval) (contacts.CreateLeadInput, error) {
	req, tool, err := creationRequest[crmcontracts.CreateLeadRequest](approval, "lead")
	if err != nil {
		return contacts.CreateLeadInput{}, err
	}
	if tool {
		req.Source = agents.ToolSource
	}
	return contacts.LeadCreationInput(req)
}

func (r approvalPreviewReader) creationPreview(ctx context.Context, approval crmcontracts.Approval, out *crmcontracts.ApprovalPreview) (*crmcontracts.ApprovalPreview, error) {
	out.Version, out.State = 2, previewIncompleteState
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "offer" {
		return r.offerCreationPreview(ctx, approval, out)
	}
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "offer_template" {
		return r.templateCreationPreview(ctx, approval, out)
	}
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "product" {
		return r.productCreationPreview(ctx, approval, out)
	}
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "project" {
		return r.projectCreationPreview(ctx, approval, out)
	}
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "deal" {
		return r.dealCreationPreview(ctx, approval, out)
	}
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "contact" {
		return r.contactCreationPreview(ctx, approval, out)
	}
	if approval.TargetEntityType != nil && *approval.TargetEntityType == "company" {
		return r.companyCreationPreview(ctx, approval, out)
	}
	in, err := leadCreationRequest(approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	creator, err := r.creationAuthority(ctx, approval)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	store := contacts.NewStore(r.db).WithFieldCatalog(customfields.NewService(r.db.Pool(), nil))
	plan, err := store.PreviewLeadCreationForActor(ctx, in, creator)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	details, effects, err := r.leadCreationDetails(ctx, plan)
	if previewUnavailable(err) {
		return out, nil
	}
	if err != nil {
		return nil, err
	}
	out.Details, out.Effects, out.State = &details, &effects, previewReadyState
	return out, r.currentEvidence(ctx, approval, out)
}

func (r approvalPreviewReader) leadCreationDetails(ctx context.Context, plan contacts.LeadCreationPreview) ([]crmcontracts.ApprovalPreviewDetail, []string, error) {
	if plan.Existing != nil {
		value, err := r.referenceValue(ctx, "lead_id", plan.Existing.Id.String())
		return []crmcontracts.ApprovalPreviewDetail{{Field: "creation_existing", Value: value}}, []string{"reuse_existing_record"}, err
	}
	in := plan.Input
	fields := map[string]any{
		"full_name": in.FullName, "email": in.Email, "phone": in.Phone, "title": in.Title,
		"company_name": in.CompanyName, "candidate_company_key": in.CandidateCompanyKey, "linkedin_url": in.LinkedInURL,
		"status": in.Status, "owner_id": in.OwnerID, "project_id": in.ProjectID, "contact_id": in.FromContactID,
		"source_system": in.SourceSystem, "source_id": in.SourceID, "source": in.Source, "score": plan.Score,
		"source_author_name": in.Author.AuthorName, "source_author_id": in.Author.AuthorID,
	}
	maps.Copy(fields, plan.CustomFields)
	details, err := r.creationFieldDetails(ctx, "lead", fields)
	if err != nil {
		return nil, nil, err
	}
	effects := []string{"create_lead_record"}
	if plan.ReviewLead != nil {
		value, err := r.referenceValue(ctx, "lead_id", plan.ReviewLead.Id.String())
		if err != nil {
			return nil, nil, err
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: "creation_duplicate", Value: value})
		effects = append(effects, "queue_duplicate_review")
	}
	return details, effects, nil
}

func (r approvalPreviewReader) creationFieldDetails(ctx context.Context, recordType string, fields map[string]any) ([]crmcontracts.ApprovalPreviewDetail, error) {
	if err := creationFieldsVisible(ctx, recordType, fields); err != nil {
		return nil, err
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	rawFields := make(map[string]json.RawMessage, len(fields))
	for field, value := range fields {
		raw, err := json.Marshal(value)
		if err != nil {
			return nil, err
		}
		rawFields[field] = raw
	}
	var details []crmcontracts.ApprovalPreviewDetail
	for _, field := range keys {
		raw := rawFields[field]
		if string(raw) == "null" && field != "owner_id" {
			continue
		}
		valueField := ""
		switch field {
		case "deal_id", "buyer_company_id", "template_id", "product_id", "valid_until", "owner_id", "project_id", "contact_id", "company_id", "parent_company_id", "partner_company_id", "pipeline_id", "stage_id", "expected_close_date", "started_at", "target_end_date", "lifecycle":
			valueField = field
		case "source_author_id":
			valueField = "user_id"
		case "converted_from_lead_id":
			valueField = "lead_id"
		}
		value, err := r.value(ctx, valueField, raw)
		if err != nil {
			return nil, err
		}
		if err := r.moneyValue(ctx, field, &value, rawFields); err != nil {
			return nil, err
		}
		if value.Kind == previewValueText && value.Text == "" {
			value.Kind = previewValueEmpty
		}
		displayField := field
		if recordType == "lead" && field == "status" {
			displayField = "lead_status"
		}
		details = append(details, crmcontracts.ApprovalPreviewDetail{Field: displayField, Value: value})
	}
	return details, nil
}

// Proposer authority determines execution-dependent effects. It never replaces
// the reviewer context used to read records or display their fields.
func (r approvalPreviewReader) creationAuthority(ctx context.Context, approval crmcontracts.Approval) (principal.Principal, error) {
	actor, err := creationActor(approval)
	if err != nil {
		return actor, err
	}
	if actor.Type == principal.PrincipalSystem {
		return actor, nil
	}
	if actor.Type != principal.PrincipalHuman && actor.Type != principal.PrincipalAgent {
		return actor, apperrors.ErrInvalidArgument
	}
	workspace, ok := principal.WorkspaceID(ctx)
	if !ok || approval.TargetEntityType == nil {
		return actor, apperrors.ErrInvalidArgument
	}
	rbac, seat, err := identity.NewService(r.db.Pool()).EffectiveAuthority(ctx, workspace, actor.UserID)
	if err != nil {
		return principal.Principal{}, err
	}
	actor.Permissions, actor.TeamIDs, actor.SeatType = rbac.Permissions, rbac.TeamIDs, seat
	if !seat.CanMutate() {
		return principal.Principal{}, apperrors.ErrSeatTierInsufficient
	}
	if err := auth.Require(principal.WithActor(ctx, actor), *approval.TargetEntityType, principal.ActionCreate); err != nil {
		return principal.Principal{}, err
	}
	return actor, nil
}

func creationFieldsVisible(ctx context.Context, recordType string, fields map[string]any) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	for _, field := range auth.MaskedFields(actor, recordType, false) {
		if _, present := fields[field]; present {
			return apperrors.ErrPermissionDenied
		}
	}
	return nil
}

func creationAddressFields(address *crmcontracts.Address) map[string]any {
	if address == nil {
		return map[string]any{}
	}
	return map[string]any{"address_line1": address.Line1, "address_line2": address.Line2, "address_city": address.City, "address_region": address.Region, "address_postal_code": address.PostalCode, "address_country": address.Country}
}
