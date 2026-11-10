// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	kevents "github.com/margince/margince/backend/internal/shared/kernel/events"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type selectedInquiryScope struct{ automation, source ids.UUID }
type selectedInquiryContextKey struct{}

func selectedInquiryAllowed(ctx context.Context, ev workflow.Event) bool {
	scope, ok := ctx.Value(selectedInquiryContextKey{}).(selectedInquiryScope)
	return ok && scope.automation == ev.AutomationID && scope.source == ev.Entity.ID
}

type SelectedInquiryRun struct {
	ID         ids.UUID
	Status     string
	HasEffects bool
}

// RunSelectedInquiry executes a reviewed source event without activating future handling.
func (e *WorkflowEngine) RunSelectedInquiry(ctx context.Context, id ids.AutomationID, version int64, envelope kevents.Envelope) (SelectedInquiryRun, error) {
	if err := auth.Require(ctx, "automation", principal.ActionUpdate); err != nil {
		return SelectedInquiryRun{}, err
	}
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return SelectedInquiryRun{}, err
	}
	saved, err := NewAutomationStore(e.db).Get(ctx, id)
	if err != nil {
		return SelectedInquiryRun{}, err
	}
	if saved.Version != version {
		return SelectedInquiryRun{}, apperrors.ErrVersionSkew
	}
	if saved.Key != inquiryRecipeName || saved.OwnerID == nil {
		return SelectedInquiryRun{}, apperrors.ErrConflict
	}
	handler := e.handlerNamed(inquiryRecipeName)
	if handler == nil || !handler.Spec().Trigger.MatchesEventType(envelope.Type) {
		return SelectedInquiryRun{}, apperrors.ErrNotFound
	}
	workspace, err := e.db.Workspace(ctx)
	if err != nil {
		return SelectedInquiryRun{}, err
	}
	event := workflowEvent(envelope, workspace.UUID)
	event.AutomationID, event.OwnerID, event.Params = id.UUID, saved.OwnerID.UUID, saved.Params
	base := runKey(handler, event)
	existing, found, err := e.selectedInquiryRun(ctx, base)
	if err != nil {
		return existing, err
	}
	if found && ((existing.Status == "applied" && existing.HasEffects) || existing.Status == runStatusRequiresApproval) {
		return existing, nil
	}
	if found {
		event.RetryAttempt, err = e.nextSelectedInquiryAttempt(ctx, base, id)
		if err != nil {
			return existing, err
		}
	}
	runCtx := context.WithValue(ctx, selectedInquiryContextKey{}, selectedInquiryScope{automation: id.UUID, source: event.Entity.ID})
	runCtx = principal.SystemActing(runCtx, systemActor)
	runCtx = principal.WithCausationEvent(runCtx, event.ID)
	runErr := e.runOne(runCtx, handler, event)
	result, found, readErr := e.selectedInquiryRun(ctx, runKey(handler, event))
	if readErr != nil {
		return result, errors.Join(runErr, readErr)
	}
	if !found && runErr == nil {
		result.Status = runSkipped
	}
	return result, runErr
}

func (e *WorkflowEngine) selectedInquiryRun(ctx context.Context, key string) (SelectedInquiryRun, bool, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return SelectedInquiryRun{}, false, err
	}
	var out SelectedInquiryRun
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id,status,COALESCE(jsonb_array_length(applied),0)>0 FROM workflow_run WHERE handler=@handler AND idempotency_key=@key`, pgx.NamedArgs{"handler": inquiryRecipeName, "key": key}).Scan(&out.ID, &out.Status, &out.HasEffects)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return out, false, nil
	}
	return out, err == nil, err
}

func (e *WorkflowEngine) nextSelectedInquiryAttempt(ctx context.Context, key string, id ids.AutomationID) (int, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return 0, err
	}
	var latest int
	err := e.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT `+retryOrdinalSQL("")+` FROM workflow_run WHERE handler=@handler AND `+runAutomationIDSQL+`=@automation AND `+baseRunKeySQL("")+`=@key`, pgx.NamedArgs{"handler": inquiryRecipeName, "automation": id.String(), "key": key}).Scan(&latest)
	})
	return latest + 1, err
}
