// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package inquiries

import (
	"context"
	"errors"
	"log/slog"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/kernel/provenance"
)

func validateInquirySelections(selected []crmcontracts.WebsiteInquirySelection, requireSelection bool) error {
	if len(selected) > 50 || (requireSelection && len(selected) == 0) {
		return httperr.Validation("selected", "invalid_count", "choose between 1 and 50 submissions")
	}
	seen := map[crmcontracts.Id]bool{}
	for _, item := range selected {
		if item.Version == nil || *item.Version < 1 {
			return httperr.Validation("selected.version", "required", "each reviewed submission needs its current version")
		}
		if item.Id == crmcontracts.Id(ids.Nil) || seen[item.Id] {
			return httperr.Validation("selected.id", "invalid", "choose distinct accepted submissions")
		}
		seen[item.Id] = true
	}
	return nil
}

func (s Settings) existingPolicy(ctx context.Context, formID ids.UUID, in crmcontracts.WebsiteInquiryExistingRequest) (automation.Automation, crmcontracts.WebsiteInquiryPolicy, error) {
	if err := httperr.RequireBodyID("automation_id", ids.UUID(in.AutomationId)); err != nil {
		return automation.Automation{}, crmcontracts.WebsiteInquiryPolicy{}, err
	}
	form, err := s.forms.GetForm(ctx, formID)
	if err != nil {
		return automation.Automation{}, crmcontracts.WebsiteInquiryPolicy{}, err
	}
	if form.HandlingAutomationID == nil || *form.HandlingAutomationID != ids.UUID(in.AutomationId) {
		return automation.Automation{}, crmcontracts.WebsiteInquiryPolicy{}, apperrors.ErrConflict
	}
	saved, err := s.policies.Get(ctx, ids.From[ids.AutomationKind](ids.UUID(in.AutomationId)))
	if err != nil {
		return saved, crmcontracts.WebsiteInquiryPolicy{}, err
	}
	if in.AutomationVersion == nil || saved.Version != int64(*in.AutomationVersion) {
		return saved, crmcontracts.WebsiteInquiryPolicy{}, apperrors.ErrVersionSkew
	}
	if saved.Key != provenance.WebsiteInquirySource || saved.OwnerID == nil {
		return saved, crmcontracts.WebsiteInquiryPolicy{}, apperrors.ErrConflict
	}
	policy, err := automation.ParseInquiryPolicy(saved.Params)
	if err != nil {
		return saved, policy, err
	}
	if ids.UUID(policy.FormId) != formID {
		return saved, policy, apperrors.ErrConflict
	}
	return saved, policy, nil
}

// HandleExisting runs a form's saved policy over submissions it accepted
// earlier, one at a time, and reports each one's outcome.
func (s Settings) HandleExisting(ctx context.Context, formID ids.UUID, in crmcontracts.WebsiteInquiryExistingRequest) (crmcontracts.WebsiteInquiryHandlingResults, error) {
	if err := auth.Require(ctx, "automation", principal.ActionUpdate); err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionUpdate); err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	if err := validateInquirySelections(in.Selected, true); err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	saved, policy, err := s.existingPolicy(ctx, formID, in)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	routing := s.routing
	if err := routing.validatePolicy(ctx, policy, false); err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	authorizer, err := routing.human(ctx, saved.OwnerID.UUID)
	if err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	if err := routing.validatePolicy(authorizer, policy, false); err != nil {
		return crmcontracts.WebsiteInquiryHandlingResults{}, err
	}
	engine := s.engine()
	out := crmcontracts.WebsiteInquiryHandlingResults{Items: make([]crmcontracts.WebsiteInquiryHandlingItem, 0, len(in.Selected))}
	for _, selected := range in.Selected {
		item := crmcontracts.WebsiteInquiryHandlingItem{Id: selected.Id, Version: selected.Version, Status: crmcontracts.WebsiteInquiryHandlingItemStatusFailed}
		err := s.fence.Within(ctx, formID, func(bound context.Context) error {
			var err error
			item, err = s.handleExistingOne(bound, engine, formID, in, selected)
			return err
		})
		if err != nil {
			item = handlingItemError(item, err)
		}
		out.Items = append(out.Items, item)
	}
	return out, nil
}

func (s Settings) handleExistingOne(ctx context.Context, engine *automation.WorkflowEngine, formID ids.UUID, in crmcontracts.WebsiteInquiryExistingRequest, selected crmcontracts.WebsiteInquirySelection) (crmcontracts.WebsiteInquiryHandlingItem, error) {
	item := crmcontracts.WebsiteInquiryHandlingItem{Id: selected.Id, Version: selected.Version}
	saved, _, err := s.existingPolicy(ctx, formID, in)
	if err != nil {
		return item, err
	}
	before := s.previewSelected(ctx, saved.ID.UUID, saved.OwnerID.UUID, saved.Params, selected)
	if before.Status != crmcontracts.WebsiteInquiryPreviewItemStatusEligible {
		item.Status, item.Reason = crmcontracts.WebsiteInquiryHandlingItemStatus(before.Status), before.Reason
		return item, nil
	}
	noteID := ids.From[ids.ActivityKind](ids.UUID(selected.Id))
	adopted, err := activities.NewStore(s.db).AdoptWebsiteInquiry(ctx, noteID, int64(*selected.Version), formID, saved.ID.UUID)
	if err != nil {
		return item, err
	}
	item.Version = adopted.Activity.Version
	run, runErr := engine.RunSelectedInquiry(ctx, saved.ID, saved.Version, adopted.Event)
	if run.ID != ids.Nil {
		id := crmcontracts.Id(run.ID)
		item.RunId = &id
	}
	if runErr != nil {
		return item, runErr
	}
	switch run.Status {
	case "requires_approval":
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusWaitingForApproval
	case "failed":
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusFailed
	case "applied":
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusAlreadyHandled
		if !run.HasEffects {
			item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusFailed
			return item, nil
		}
		actual, err := s.taskForSelected(ctx, saved, adopted.Activity)
		if err != nil {
			return item, err
		}
		item.Status, item.TaskId = crmcontracts.WebsiteInquiryHandlingItemStatusTaskCreated, &actual.Id
	default:
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusSkipped
	}
	return item, nil
}

func (s Settings) taskForSelected(ctx context.Context, saved automation.Automation, note crmcontracts.Activity) (crmcontracts.Activity, error) {
	if note.Version == nil {
		return crmcontracts.Activity{}, apperrors.ErrVersionSkew
	}
	plan, err := s.planSelectedInquiry(ctx, saved.ID.UUID, saved.OwnerID.UUID, saved.Params, crmcontracts.WebsiteInquirySelection{Id: note.Id, Version: note.Version})
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	if plan.ExistingTask != nil {
		return *plan.ExistingTask, nil
	}
	system, key, err := inquiryTaskSource(plan)
	if err != nil {
		return crmcontracts.Activity{}, err
	}
	return activities.NewStore(s.db).ActivityBySource(ctx, system, key, storekit.IncludeArchived)
}

func handlingItemError(item crmcontracts.WebsiteInquiryHandlingItem, err error) crmcontracts.WebsiteInquiryHandlingItem {
	switch {
	case errors.Is(err, apperrors.ErrVersionSkew):
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusChanged
	case errors.Is(err, apperrors.ErrNotFound), errors.Is(err, apperrors.ErrPermissionDenied):
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusUnavailable
	default:
		item.Status = crmcontracts.WebsiteInquiryHandlingItemStatusFailed
		slog.Error("website inquiry handling failed", "source_activity_id", item.Id, "error", err)
	}
	return item
}
