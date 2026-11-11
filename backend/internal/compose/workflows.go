// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"time"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/modules/automation"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/notices"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewWorkflowEngine shares the catalog executors with the worker.
func NewWorkflowEngine(db *database.DB) *automation.WorkflowEngine {
	return workflowEngineWithDrafter(db, nil)
}

// NewWorkflowEngineWithReplyDraft adds the routed reply lane to draft_email
// actions while preserving NewWorkflowEngine's deterministic default.
func NewWorkflowEngineWithReplyDraft(db *database.DB, brain completer) *automation.WorkflowEngine {
	if brain == nil {
		return NewWorkflowEngine(db)
	}
	drafter := newReplyDrafter(db.Pool(), brain, nil)
	return workflowEngineWithDrafter(db, drafter)
}

func workflowEngineWithDrafter(db *database.DB, drafter activities.EmailDrafter) *automation.WorkflowEngine {
	engine := automation.NewWorkflowEngine(db, identity.NewService(db.Pool()))
	contactsStore := contacts.NewStore(db)
	claims := automation.NewEffectClaims(db)
	ex := automation.Executors{
		RecapDraftClaims: claims,
		MeetingRecaps:    activities.NewStore(db),
		Inquiries:        automation.NewInquiryService(db, newInquiryRouting(db)),
		Provider:         NewProvider(db.Pool()),
		Approvals:        automationApprovalsAdapter{svc: NewApprovalsService(db)},
		// Drafts remain held until the approval send path checks delivery.
		Comms:     newCommsAdapter(db.Pool(), drafter, SendPath{}),
		Notifier:  noticesNotifier{store: notices.NewStore(db)},
		Claims:    claims,
		Language:  installationLanguage(db.Pool()),
		Lists:     NewListRules(db.Pool()),
		Authority: identity.NewService(db.Pool()),
	}
	engine.WithFiringFence(newInquiryFiringFence(db))
	engine.WithRulePauses(noticesNotifier{store: notices.NewStore(db)}, ex.Lists)
	for _, handler := range automation.StarterWorkflows(ex) {
		engine.RegisterWorkflow(handler)
	}
	engine.RegisterWorkflow(contacts.LeadRoutingWorkflow(contactsStore))
	for _, handler := range contacts.LeadScoreWorkflows(contactsStore) {
		engine.RegisterSystemWorkflow(handler)
	}
	for _, handler := range contacts.LeadSLAWorkflows(contactsStore) {
		engine.RegisterSystemWorkflow(handler)
	}
	activityStore := activities.NewStore(db)
	engine.RegisterSystemWorkflow(newLeadSLAEscalation(db, time.Now))
	for _, handler := range activities.FollowUpWorkflows(activityStore) {
		engine.RegisterSystemWorkflow(handler)
	}
	for _, handler := range activities.QuietReminderReopenWorkflows(activityStore) {
		engine.RegisterSystemWorkflow(handler)
	}
	for _, handler := range activities.OwnershipWorkflows(activityStore) {
		engine.RegisterSystemWorkflow(handler)
	}
	for _, handler := range activities.NotMineRearmWorkflows(activityStore) {
		engine.RegisterSystemWorkflow(handler)
	}
	return engine
}

// Automation requests use the approvals module's canonical staging service.
type automationApprovalsAdapter struct{ svc *approvals.Service }

func (a automationApprovalsAdapter) Stage(ctx context.Context, in automation.StageRequest) (ids.ApprovalID, error) {
	return a.svc.Stage(ctx, approvals.StageInput{
		Kind:           in.Kind,
		ProposedChange: in.ProposedChange,
		DiffHash:       in.DiffHash,
		TargetType:     in.TargetType,
		TargetID:       in.TargetID,
		Summary:        in.Summary,
		SystemText:     in.SystemText,
		JoinPending:    in.JoinPending,
	})
}
