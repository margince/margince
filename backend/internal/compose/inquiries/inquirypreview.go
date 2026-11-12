// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

var errLegacyInquiryOwned = errors.New("the earlier lead handling still owns this inquiry")

// Preview shows who a policy would hand each selected submission to, and when
// the task would be due. It writes nothing.
func (s Settings) Preview(ctx context.Context, id ids.UUID, in crmcontracts.WebsiteInquiryPreviewRequest, now time.Time) (crmcontracts.WebsiteInquiryHandlingPreview, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	if err := validateInquirySelections(in.Selected, false); err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	raw, err := json.Marshal(in.Policy)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	policy, err := automation.ParseInquiryPolicy(raw)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	var out crmcontracts.WebsiteInquiryHandlingPreview
	err = database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(bound context.Context) error {
		owner, policyID, err := s.previewOwner(bound, id, in, policy)
		if err != nil {
			return err
		}
		admit, err := s.routing.human(bound, owner)
		if err != nil {
			return err
		}
		if err := s.routing.validatePolicy(admit, policy, false); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) {
				return auth.RefuseRow(bound, "automation", err)
			}
			return err
		}
		due, err := automation.InquiryDueAt(policy, now)
		if err != nil {
			return err
		}
		out = crmcontracts.WebsiteInquiryHandlingPreview{ReceivedAt: now, ExampleDueAt: due, Items: make([]crmcontracts.WebsiteInquiryPreviewItem, 0, len(in.Selected))}
		for _, selected := range in.Selected {
			out.Items = append(out.Items, s.previewSelected(bound, policyID, owner, raw, selected))
		}
		return nil
	})
	return out, err
}

// previewOwner is whose grants a preview runs under: the saved policy's owner,
// or the caller for a policy that is not saved yet.
func (s Settings) previewOwner(ctx context.Context, formID ids.UUID, in crmcontracts.WebsiteInquiryPreviewRequest, policy crmcontracts.WebsiteInquiryPolicy) (ids.UUID, ids.UUID, error) {
	form, err := s.forms.GetForm(ctx, formID)
	if err != nil {
		return ids.Nil, ids.Nil, err
	}
	if form.HandlingAutomationID != nil && (in.AutomationId == nil || *form.HandlingAutomationID != ids.UUID(*in.AutomationId)) {
		return ids.Nil, ids.Nil, apperrors.ErrConflict
	}
	actor, ok := principal.Actor(ctx)
	if !ok || (actor.Type != principal.PrincipalHuman && actor.Type != principal.PrincipalAgent) || (in.AutomationId == nil && actor.UserID == ids.Nil) {
		return ids.Nil, ids.Nil, apperrors.ErrPermissionDenied
	}
	if in.AutomationId == nil {
		return actor.UserID, ids.Nil, nil
	}
	return s.savedPolicyOwner(ctx, in, policy)
}

// savedPolicyOwner is the owner and id of the saved policy a preview names. It
// must still be the version the caller saw, for the same form.
func (s Settings) savedPolicyOwner(ctx context.Context, in crmcontracts.WebsiteInquiryPreviewRequest, policy crmcontracts.WebsiteInquiryPolicy) (ids.UUID, ids.UUID, error) {
	saved, err := s.policies.Get(ctx, ids.From[ids.AutomationKind](ids.UUID(*in.AutomationId)))
	if err != nil {
		return ids.Nil, ids.Nil, err
	}
	if saved.Key != provenance.WebsiteInquirySource || in.AutomationVersion == nil || saved.Version != int64(*in.AutomationVersion) {
		return ids.Nil, ids.Nil, apperrors.ErrVersionSkew
	}
	savedPolicy, err := automation.ParseInquiryPolicy(saved.Params)
	if err != nil {
		return ids.Nil, ids.Nil, err
	}
	if savedPolicy.FormId != policy.FormId {
		return ids.Nil, ids.Nil, apperrors.ErrConflict
	}
	if saved.OwnerID == nil {
		return ids.Nil, ids.Nil, apperrors.ErrPermissionDenied
	}
	return saved.OwnerID.UUID, saved.ID.UUID, nil
}

func (s Settings) previewSelected(ctx context.Context, policyID, owner ids.UUID, raw json.RawMessage, selected crmcontracts.WebsiteInquirySelection) crmcontracts.WebsiteInquiryPreviewItem {
	item := crmcontracts.WebsiteInquiryPreviewItem{Id: selected.Id, Version: selected.Version, Status: crmcontracts.WebsiteInquiryPreviewItemStatusUnavailable}
	plan, err := s.planSelectedInquiry(ctx, policyID, owner, raw, selected)
	if err != nil {
		return previewItemError(item, err)
	}
	if plan.ExistingTask != nil {
		item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusAlreadyHandled
		reason := "inquiry_task_exists"
		item.Reason = &reason
		return item
	}
	recipient, wireTarget := crmcontracts.Id(plan.Recipient), crmcontracts.Id(plan.Target.ID)
	kind := crmcontracts.WebsiteInquiryPreviewItemTargetType(plan.Target.Type)
	item.Status, item.RecipientId, item.DueAt, item.Reason, item.TargetType, item.TargetId = crmcontracts.WebsiteInquiryPreviewItemStatusEligible, &recipient, &plan.DueAt, &plan.Reason, &kind, &wireTarget
	system, key, err := inquiryTaskSource(plan)
	if err != nil {
		return previewItemError(item, err)
	}
	_, err = activities.NewStore(s.db).ActivityBySource(ctx, system, key, storekit.IncludeArchived)
	if err == nil {
		item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusAlreadyHandled
		reason := "inquiry_task_exists"
		item.RecipientId, item.DueAt, item.Reason = nil, nil, &reason
	} else if !errors.Is(err, apperrors.ErrNotFound) {
		return previewItemError(item, err)
	}
	return item
}

func previewItemError(item crmcontracts.WebsiteInquiryPreviewItem, err error) crmcontracts.WebsiteInquiryPreviewItem {
	item.RecipientId, item.DueAt, item.TargetId, item.TargetType, item.Reason = nil, nil, nil, nil, nil
	item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusUnavailable
	if errors.Is(err, apperrors.ErrVersionSkew) {
		item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusChanged
		return item
	}
	if errors.Is(err, errLegacyInquiryOwned) {
		reason := "legacy_handling_owned"
		item.Reason = &reason
		return item
	}
	var declined *workflow.DeclinedError
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) || errors.As(err, &declined) {
		return item
	}
	slog.Error("website inquiry preview failed", "source_activity_id", item.Id, "error", err)
	item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusFailed
	return item
}

func (s Settings) planSelectedInquiry(ctx context.Context, policyID, owner ids.UUID, raw json.RawMessage, selected crmcontracts.WebsiteInquirySelection) (automation.InquiryPreview, error) {
	note, err := s.selectedNote(ctx, selected)
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	ex := s.executors
	target, err := automation.InquiryTarget(note)
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	if _, err := ex.Provider.Read(ctx, target); err != nil {
		return automation.InquiryPreview{}, err
	}
	owned, err := s.policies.LegacyLeadTaskOwned(ctx, *note.SourceId)
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	if owned {
		return automation.InquiryPreview{}, errLegacyInquiryOwned
	}
	payload, err := json.Marshal(crmcontracts.PublicEventActivityCaptured{Kind: "note", SourceSystem: note.SourceSystem, WebsiteSubmissionKey: note.SourceId})
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	ev := workflow.Event{WorkspaceID: workspace.UUID, OwnerID: owner, AutomationID: policyID, Params: raw, Payload: payload, Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.UUID(selected.Id)}}
	plan, err := automation.NewInquiryService(s.db, s.routing).Preview(ctx, ex, ev)
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	return plan, nil
}

// selectedNote is the submission note a caller selected, at the version the
// caller saw.
func (s Settings) selectedNote(ctx context.Context, selected crmcontracts.WebsiteInquirySelection) (crmcontracts.Activity, error) {
	if selected.Version == nil {
		return crmcontracts.Activity{}, apperrors.ErrVersionSkew
	}
	note, err := activities.NewStore(s.db).GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(selected.Id)), storekit.LiveOnly)
	if err != nil {
		return note, err
	}
	if note.Version == nil || *note.Version != *selected.Version {
		return note, apperrors.ErrVersionSkew
	}
	if note.Kind != crmcontracts.ActivityKindNote || note.SourceSystem == nil || *note.SourceSystem != provenance.WebFormSource || note.SourceId == nil {
		return note, apperrors.ErrNotFound
	}
	return note, nil
}

func inquiryTaskSource(plan automation.InquiryPreview) (string, string, error) {
	if len(plan.Effect.Actions) != 1 {
		return "", "", apperrors.ErrConflict
	}
	var task struct {
		System string `json:"source_system"`
		ID     string `json:"source_id"`
	}
	if err := json.Unmarshal(plan.Effect.Actions[0].Args, &task); err != nil {
		return "", "", err
	}
	if task.System == "" || task.ID == "" {
		return "", "", apperrors.ErrConflict
	}
	return task.System, task.ID, nil
}
