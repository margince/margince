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

func (s Settings) Preview(ctx context.Context, id ids.UUID, in crmcontracts.WebsiteInquiryPreviewRequest, now time.Time) (crmcontracts.WebsiteInquiryHandlingPreview, error) {
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
		form, err := s.forms.GetForm(bound, id)
		if err != nil {
			return err
		}
		if form.HandlingAutomationID != nil && (in.AutomationId == nil || *form.HandlingAutomationID != ids.UUID(*in.AutomationId)) {
			return apperrors.ErrConflict
		}
		actor, ok := principal.Actor(bound)
		if !ok || (actor.Type != principal.PrincipalHuman && actor.Type != principal.PrincipalAgent) || (in.AutomationId == nil && actor.UserID == ids.Nil) {
			return apperrors.ErrPermissionDenied
		}
		owner := actor.UserID
		policyID := ids.Nil
		if in.AutomationId != nil {
			saved, err := s.policies.Get(bound, ids.From[ids.AutomationKind](ids.UUID(*in.AutomationId)))
			if err != nil {
				return err
			}
			if saved.Key != provenance.WebsiteInquirySource || in.AutomationVersion == nil || saved.Version != int64(*in.AutomationVersion) {
				return apperrors.ErrVersionSkew
			}
			savedPolicy, err := automation.ParseInquiryPolicy(saved.Params)
			if err != nil {
				return err
			}
			if savedPolicy.FormId != policy.FormId {
				return apperrors.ErrConflict
			}
			if saved.OwnerID == nil {
				return apperrors.ErrPermissionDenied
			}
			owner, policyID = saved.OwnerID.UUID, saved.ID.UUID
		}
		admit, err := s.routing.human(bound, owner)
		if err != nil {
			return err
		}
		if err := s.routing.validate(admit, policy, false); err != nil {
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
	if selected.Version == nil {
		return automation.InquiryPreview{}, apperrors.ErrVersionSkew
	}
	note, err := activities.NewStore(s.db).GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(selected.Id)), storekit.LiveOnly)
	if err != nil {
		return automation.InquiryPreview{}, err
	}
	if note.Version == nil || *note.Version != *selected.Version {
		return automation.InquiryPreview{}, apperrors.ErrVersionSkew
	}

	if note.Kind != crmcontracts.ActivityKindNote || note.SourceSystem == nil || *note.SourceSystem != provenance.WebFormSource || note.SourceId == nil {
		return automation.InquiryPreview{}, apperrors.ErrNotFound
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
