// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

func (s inquirySettings) Preview(ctx context.Context, id ids.UUID, in crmcontracts.WebsiteInquiryPreviewRequest, now time.Time) (crmcontracts.WebsiteInquiryHandlingPreview, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	if len(in.Selected) > 50 {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, httperr.Validation("selected", "too_many", "choose up to 50 submissions")
	}
	raw, err := json.Marshal(in.Policy)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	policy, err := automation.ParseInquiryPolicy(raw)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandlingPreview{}, err
	}
	seen := map[crmcontracts.Id]bool{}
	for _, selected := range in.Selected {
		if selected.Version == nil || *selected.Version < 1 {
			return crmcontracts.WebsiteInquiryHandlingPreview{}, httperr.Validation("selected.version", "required", "each reviewed submission needs its current version")
		}
		if selected.Id == crmcontracts.Id(ids.Nil) || seen[selected.Id] {
			return crmcontracts.WebsiteInquiryHandlingPreview{}, httperr.Validation("selected.id", "invalid", "choose distinct accepted submissions")
		}
		seen[selected.Id] = true
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
		admit, err := newInquiryRouting(s.db).human(bound, owner)
		if err != nil {
			return err
		}
		if err := newInquiryRouting(s.db).Validate(admit, policy); err != nil {
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

func (s inquirySettings) previewSelected(ctx context.Context, policyID, owner ids.UUID, raw json.RawMessage, selected crmcontracts.WebsiteInquirySelection) crmcontracts.WebsiteInquiryPreviewItem {
	item := crmcontracts.WebsiteInquiryPreviewItem{Id: selected.Id, Version: selected.Version, Status: crmcontracts.WebsiteInquiryPreviewItemStatusUnavailable}
	if selected.Version == nil {
		item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusChanged
		return item
	}
	note, err := activities.NewStore(s.db).GetActivityContent(ctx, ids.From[ids.ActivityKind](ids.UUID(selected.Id)), storekit.LiveOnly)
	if err != nil {
		return previewItemError(item, err)
	}
	if note.Version == nil || *note.Version != *selected.Version {
		item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusChanged
		return item
	}
	if note.Kind != crmcontracts.ActivityKindNote || note.SourceSystem == nil || *note.SourceSystem != provenance.WebFormSource || note.SourceId == nil {
		return item
	}
	payload, err := json.Marshal(crmcontracts.PublicEventActivityCaptured{Kind: "note", SourceSystem: note.SourceSystem, WebsiteSubmissionKey: note.SourceId})
	if err != nil {
		return previewItemError(item, err)
	}
	workspace, err := s.db.Workspace(ctx)
	if err != nil {
		return previewItemError(item, err)
	}
	ex := automation.Executors{Provider: NewProviderFor(s.db), Authority: identity.NewService(s.db.Pool())}
	ev := workflow.Event{WorkspaceID: workspace.UUID, OwnerID: owner, AutomationID: policyID, Params: raw, Payload: payload, Entity: datasource.EntityRef{Type: datasource.EntityActivity, ID: ids.UUID(selected.Id)}}
	plan, err := automation.NewInquiryService(s.db, newInquiryRouting(s.db)).Preview(ctx, ex, ev)
	if err != nil {
		return previewItemError(item, err)
	}
	recipient, target := crmcontracts.Id(plan.Recipient), crmcontracts.Id(plan.Target.ID)
	kind := crmcontracts.WebsiteInquiryPreviewItemTargetType(plan.Target.Type)
	item.Status, item.RecipientId, item.DueAt, item.Reason, item.TargetType, item.TargetId = crmcontracts.WebsiteInquiryPreviewItemStatusEligible, &recipient, &plan.DueAt, &plan.Reason, &kind, &target
	var task struct {
		System string `json:"source_system"`
		ID     string `json:"source_id"`
	}
	if err := json.Unmarshal(plan.Effect.Actions[0].Args, &task); err != nil {
		return previewItemError(item, err)
	}
	_, err = activities.NewStore(s.db).ActivityBySource(ctx, task.System, task.ID, storekit.IncludeArchived)
	if err == nil {
		item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusAlreadyHandled
	} else if !errors.Is(err, apperrors.ErrNotFound) {
		return previewItemError(item, err)
	}
	return item
}

func previewItemError(item crmcontracts.WebsiteInquiryPreviewItem, err error) crmcontracts.WebsiteInquiryPreviewItem {
	var declined *workflow.DeclinedError
	if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) || errors.As(err, &declined) {
		return item
	}
	slog.Error("website inquiry preview failed", "source_activity_id", item.Id, "error", err)
	item.Status = crmcontracts.WebsiteInquiryPreviewItemStatusFailed
	return item
}
