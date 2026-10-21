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

func (r approvalPreviewReader) disqualifyLeadChanges(ctx context.Context, approval crmcontracts.Approval, current, changes map[string]json.RawMessage, out *crmcontracts.ApprovalPreview) error {
	if approval.TargetEntityType == nil || *approval.TargetEntityType != string(recordTypeLead) {
		return apperrors.ErrInvalidArgument
	}
	in, err := commercialRequest[contacts.DisqualifyLeadInput](approval)
	if err != nil {
		return err
	}

	store := contacts.NewStore(r.db)
	patch, err := store.PreviewDisqualifyLead(ctx, ids.From[ids.LeadKind](ids.UUID(*approval.TargetEntityId)), in, int64(*approval.TargetVersion))
	if err != nil {
		return err
	}
	before, err := json.Marshal(patch.Before())
	if err != nil {
		return err
	}
	after, err := json.Marshal(patch.After())
	if err != nil {
		return err
	}
	if err := applyPreviewChanges(current, changes, before, after, ""); err != nil {
		return err
	}
	if err := disqualifyReasonLabels(ctx, store, current, changes); err != nil {
		return err
	}
	current["lead_status"], changes["lead_status"] = current[previewFieldStatus], changes[previewFieldStatus]
	delete(current, previewFieldStatus)
	delete(changes, previewFieldStatus)
	out.Effects = &[]string{"archive_lead_remove_tags"}
	return nil
}

func disqualifyReasonLabels(ctx context.Context, store *contacts.Store, current, changes map[string]json.RawMessage) error {
	if _, changed := changes["disqualify_reason_id"]; changed {
		labels, err := store.ListLeadDisqualifyReasons(ctx)
		if err != nil {
			return err
		}
		for _, fields := range []map[string]json.RawMessage{current, changes} {
			if err := disqualifyReasonLabel(fields, labels); err != nil {
				return err
			}
		}
	}
	return nil
}

func disqualifyReasonLabel(fields map[string]json.RawMessage, labels []crmcontracts.LeadDisqualifyReason) error {
	raw := fields["disqualify_reason_id"]
	var id *ids.UUID
	if err := json.Unmarshal(raw, &id); err != nil {
		return apperrors.ErrInvalidArgument
	}
	fields["disqualify_reason"] = json.RawMessage("null")
	if id != nil {
		found := false
		for _, reason := range labels {
			if ids.UUID(reason.Id) == *id {
				label, err := json.Marshal(reason.Label)
				if err != nil {
					return err
				}
				fields["disqualify_reason"] = label
				found = true
				break
			}
		}
		if !found {
			return apperrors.ErrPermissionDenied
		}
	}
	delete(fields, "disqualify_reason_id")
	return nil
}
