// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
	"github.com/margince/margince/backend/internal/shared/ports/workflow"
)

type InquiryService struct {
	db      *database.DB
	claims  *EffectClaimStore
	routing InquiryRouting
}

func NewInquiryService(db *database.DB, routing InquiryRouting) *InquiryService {
	return &InquiryService{db: db, claims: NewEffectClaims(db), routing: routing}
}

func (s *InquiryService) Apply(ctx context.Context, ex Executors, ev workflow.Event, effect workflow.Effect) (workflow.RunResult, error) {
	if s.routing == nil {
		return workflow.RunResult{}, errors.New("automation: inquiry routing is not configured")
	}
	policy, err := ParseInquiryPolicy(ev.Params)
	if err != nil {
		return workflow.RunResult{}, err
	}
	receipt, err := readInquiryReceipt(ctx, ex, ev, policy)
	if err != nil {
		return workflow.RunResult{}, err
	}
	if _, err := s.currentPolicy(ctx, ev); err != nil {
		return workflow.RunResult{}, err
	}
	plan, err := s.claims.FreezeInquiry(ctx, receipt.Key, func(bound context.Context) (InquiryPlan, error) {
		return s.choose(bound, ex, ev, policy, receipt)
	})
	if err != nil {
		return workflow.RunResult{}, err
	}
	if plan.PolicyID != ev.AutomationID || plan.SourceActivityID != ev.Entity.ID || plan.Target != receipt.Target {
		return workflow.RunResult{}, errors.New("automation: another policy owns this inquiry assignment")
	}
	admit, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return workflow.RunResult{}, err
	}
	if err := auth.Require(admit, "activity", principal.ActionCreate); err != nil {
		return workflow.RunResult{}, err
	}
	if err := s.routing.CheckRecipient(ctx, plan.Recipient, plan.Target, plan.SourceActivityID); err != nil {
		return workflow.RunResult{}, err
	}
	action, err := s.originalTask(ctx, plan)
	if err != nil {
		return workflow.RunResult{}, err
	}
	if plan.Target.Type == datasource.EntityContact {
		reused, err := s.routing.ReuseArrivalTask(ctx, admit, receipt.Key, plan.SourceActivityID, plan.Recipient, plan.DueAt)
		if err != nil {
			return workflow.RunResult{}, err
		}
		if reused {
			return workflow.RunResult{Applied: []workflow.Action{markDeduplicated(action)}}, nil
		}
	}
	effect.Actions = []workflow.Action{action}
	return s.applyTask(ctx, ex, effect)
}

func (s *InquiryService) applyTask(ctx context.Context, ex Executors, effect workflow.Effect) (workflow.RunResult, error) {
	var applied []workflow.Action
	// The local provider joins this transaction, so a failed task cannot leave a fresh claim behind.
	run := func() error {
		return database.WithWorkspaceSnapshot(ctx, s.db.Pool(), func(bound context.Context) error {
			var err error
			applied, err = ApplyActions(bound, ex, effect)
			return err
		})
	}
	err := storekit.RetryLockCycles(ctx, run)
	return workflow.RunResult{Applied: applied}, err
}

func (s *InquiryService) choose(ctx context.Context, ex Executors, ev workflow.Event, policy crmcontracts.WebsiteInquiryPolicy, receipt inquiryReceipt) (InquiryPlan, error) {
	admit, err := ownerContext(ctx, ex.Authority, ev.WorkspaceID, ev.OwnerID)
	if err != nil {
		return InquiryPlan{}, err
	}
	var plan InquiryPlan
	plan.PolicyVersion, err = s.currentPolicy(ctx, ev)
	if err != nil {
		return plan, err
	}
	err = s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT id FROM workflow_run WHERE handler=@handler AND idempotency_key=@key`, pgx.NamedArgs{"handler": inquiryRecipeName, "key": runKey(websiteInquiry{}, ev)}).Scan(&plan.OriginalRunID)
	})
	if err != nil {
		return plan, err
	}
	plan.PolicyID, plan.SourceActivityID, plan.Target = ev.AutomationID, ev.Entity.ID, receipt.Target
	plan.DueAt, err = taskDueAt(ev.Params, receipt.Activity.CreatedAt, 1)
	if err != nil {
		return plan, err
	}
	plan.Recipient, plan.Reason, err = s.routing.Choose(ctx, admit, policy, receipt.Target)
	if err != nil {
		return plan, err
	}
	return plan, s.routing.CheckRecipient(ctx, plan.Recipient, plan.Target, plan.SourceActivityID)
}

func (s *InquiryService) originalTask(ctx context.Context, plan InquiryPlan) (workflow.Action, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return workflow.Action{}, err
	}
	var raw json.RawMessage
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT planned FROM workflow_run WHERE id=@id AND handler=@handler`, pgx.NamedArgs{"id": plan.OriginalRunID, "handler": inquiryRecipeName}).Scan(&raw)
	})
	if err != nil {
		return workflow.Action{}, err
	}
	var actions []workflow.Action
	if err := json.Unmarshal(raw, &actions); err != nil {
		return workflow.Action{}, err
	}
	if len(actions) != 1 || actions[0].Kind != workflow.ActionCreateTask || actions[0].Target != plan.Target {
		return workflow.Action{}, errors.New("automation: the original inquiry task is unavailable")
	}
	action := actions[0]
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(action.Args, &fields); err != nil {
		return action, err
	}
	fields["assignee_id"], err = json.Marshal(plan.Recipient)
	if err != nil {
		return action, err
	}
	fields["due_at"], err = json.Marshal(plan.DueAt)
	if err != nil {
		return action, err
	}
	action.Args, err = json.Marshal(fields)
	return action, err
}

func (s *InquiryService) currentPolicy(ctx context.Context, ev workflow.Event) (int64, error) {
	if err := auth.Require(ctx, "automation", principal.ActionRead); err != nil {
		return 0, err
	}
	var version int64
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT version FROM automation
 WHERE id=@id AND key=@key AND enabled AND archived_at IS NULL AND params=@params::jsonb AND owner_id=@owner
 FOR SHARE`, pgx.NamedArgs{"id": ev.AutomationID, "key": inquiryRecipeName, "params": ev.Params, "owner": ev.OwnerID}).Scan(&version)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, apperrors.ErrConflict
	}
	return version, err
}
