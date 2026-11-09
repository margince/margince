// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"slices"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// previewDetailSources names the record fields a plain-text detail repeats
// under another key, so a mask on the field reaches the detail.
var previewDetailSources = map[string][]string{
	"promotion_contact_name":     {fieldFullName},
	"promotion_contact_email":    {fieldEmail},
	"promotion_contact_phone":    {fieldPhone, previewFieldPhones},
	"promotion_contact_title":    {fieldTitle},
	"promotion_contact_source":   {fieldSource},
	"promotion_contact_owner":    {bulkParamOwner},
	"promotion_deal_owner":       {bulkParamOwner},
	"promotion_company_industry": {fieldIndustry},
	"promotion_company_proposed": {companyNameField},
	"archive_email":              {fieldEmail},
	"archive_phone":              {fieldPhone, previewFieldPhones},
	"archive_domain":             {previewFieldDomains},
	"merge_address":              {fieldEmail, fieldPhone, previewFieldPhones},
}

// previewAddressDetails repeat an email address, so a mask on a contact's or a
// lead's email reaches them.
var previewAddressDetails = []string{
	"to", "cc", "bcc", "intended_attendee", previewHostNamedAttendee, "confirmation_recipient", "relink_participant_address",
	"merge_participant_address", "merge_confirmation", "merge_draft_to", "merge_draft_cc", "merge_draft_bcc",
	"promotion_withdrawal_address",
}

// previewFieldNamers are details whose text is the name of the field the
// details around them carry. The value is where that group starts, relative
// to the namer: a read names the values after it, a merge the one before.
var previewFieldNamers = map[string]int{
	"read_profile_field":          0,
	"read_fact_field":             0,
	"merge_profile_field":         -1,
	"merge_company_fact_field":    -1,
	"merge_company_profile_field": -1,
}

// previewMaskedObjects names, per kind, an object whose fields its details
// repeat before any record of it exists: a promotion's contact to be.
var previewMaskedObjects = map[string][]string{
	operationPromoteLead: {string(recordTypeContact)},
}

const previewFieldPhones = "phones"

// maskForViewer withholds each change and detail repeating a field the
// viewer's role masks on any object the preview read. An import row is masked
// by its own object.
func maskForViewer(ctx context.Context, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview, touched *previewTouched) error {
	hasDetails := preview.Details != nil && len(*preview.Details) > 0
	hasRows := preview.ImportRows != nil && len(*preview.ImportRows) > 0
	if len(preview.Changes) == 0 && !hasDetails && !hasRows {
		return nil
	}
	viewer, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	masked := viewerMaskedFields(viewer, approval, preview, touched)
	maskChanges(ctx, preview.Changes, masked)
	if hasDetails {
		maskDetails(ctx, *preview.Details, masked)
	}
	if hasRows {
		for i := range *preview.ImportRows {
			row := &(*preview.ImportRows)[i]
			rowMasked := auth.MaskedFields(viewer, row.Object, true)
			maskChanges(ctx, row.Changes, rowMasked)
			maskDetails(ctx, row.Details, rowMasked)
		}
	}
	return nil
}

// viewerMaskedFields collects the fields the viewer's role masks on the target
// and on the objects the preview read.
func viewerMaskedFields(viewer principal.Principal, approval crmcontracts.Approval, preview *crmcontracts.ApprovalPreview, touched *previewTouched) []string {
	return maskedFieldsFor(viewer, approvalTargetType(approval), true, previewMaskObjects(approval, previewReferences(approval, preview, touched)))
}

// previewMaskObjects names the objects whose masks reach a plan: those of the
// records it refers to, and those its kind repeats before a record exists.
func previewMaskObjects(approval crmcontracts.Approval, refs *previewTouched) []string {
	objects := slices.Clone(previewMaskedObjects[approval.Kind])
	for object := range refs.byType {
		objects = append(objects, object)
	}
	return objects
}

// maskedFieldsFor collects the fields the viewer's role masks on target and on
// objects. Only the target lifts a mask conditioned on write authority, and
// only when targetWritable says the viewer still holds it.
func maskedFieldsFor(viewer principal.Principal, target string, targetWritable bool, objects []string) []string {
	var masked []string
	if target != "" {
		masked = auth.MaskedFields(viewer, target, targetWritable)
	}
	for _, object := range objects {
		if object != target {
			masked = append(masked, auth.MaskedFields(viewer, object, false)...)
		}
	}
	for _, holder := range []string{string(recordTypeContact), string(recordTypeLead)} {
		if slices.Contains(auth.MaskedFields(viewer, holder, false), fieldEmail) {
			masked = append(masked, previewAddressDetails...)
		}
	}
	return masked
}

func approvalTargetType(approval crmcontracts.Approval) string {
	if approval.TargetEntityType == nil {
		return ""
	}
	return *approval.TargetEntityType
}

// maskChanges and maskDetails tell the approvals service each value they
// replace, so the staged call that repeats it stays behind too.
func maskChanges(ctx context.Context, changes []crmcontracts.ApprovalPreviewChange, masked []string) {
	for i := range changes {
		if slices.Contains(masked, changes[i].Field) {
			approvals.RecordMasked(ctx)
			changes[i].Before = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
			changes[i].After = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
		}
	}
}

func maskDetails(ctx context.Context, details []crmcontracts.ApprovalPreviewDetail, masked []string) {
	isMasked := func(field string) bool { return slices.Contains(masked, field) }
	for i, sources := range detailFieldNames(details) {
		if slices.ContainsFunc(sources, isMasked) {
			approvals.RecordMasked(ctx)
			details[i].Value = crmcontracts.ApprovalPreviewValue{Kind: previewValueUnavailable}
		}
	}
}

// detailFieldNames answers, by position, every field name a detail repeats:
// its own, the record fields it copies, and the one its namer says it carries.
func detailFieldNames(details []crmcontracts.ApprovalPreviewDetail) [][]string {
	named := namedFieldGroups(details)
	out := make([][]string, len(details))
	for i := range details {
		sources := append([]string{details[i].Field}, previewDetailSources[details[i].Field]...)
		if named[i] != "" {
			sources = append(sources, named[i])
		}
		out[i] = sources
	}
	return out
}

// previewFieldNames lists, once each, every field name the mask checks compare
// in a plan's changes, details and import rows.
func previewFieldNames(preview *crmcontracts.ApprovalPreview) []string {
	var fields []string
	add := func(changes []crmcontracts.ApprovalPreviewChange, details []crmcontracts.ApprovalPreviewDetail) {
		for i := range changes {
			fields = append(fields, changes[i].Field)
		}
		for _, sources := range detailFieldNames(details) {
			fields = append(fields, sources...)
		}
	}
	var details []crmcontracts.ApprovalPreviewDetail
	if preview.Details != nil {
		details = *preview.Details
	}
	add(preview.Changes, details)
	if preview.ImportRows != nil {
		for i := range *preview.ImportRows {
			add((*preview.ImportRows)[i].Changes, (*preview.ImportRows)[i].Details)
		}
	}
	slices.Sort(fields)
	return slices.Compact(fields)
}

// namedFieldGroups answers, by position, the field a namer says the detail
// carries, or "" outside any named group.
func namedFieldGroups(details []crmcontracts.ApprovalPreviewDetail) []string {
	starts := map[int]string{}
	for i, detail := range details {
		if offset, namer := previewFieldNamers[detail.Field]; namer && i+offset >= 0 {
			starts[i+offset] = detail.Value.Text
		}
	}
	out := make([]string, len(details))
	current := ""
	for i := range details {
		if field, starts := starts[i]; starts {
			current = field
		}
		out[i] = current
	}
	return out
}
