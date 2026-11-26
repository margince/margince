// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package automation

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Update changes a rule. A website inquiry policy changes under its form's
// fence, so no submission is handled halfway through the change.
func (s *AutomationStore) Update(ctx context.Context, id ids.AutomationID, in UpdateAutomationInput) (Automation, error) {
	var out Automation
	err := s.withInquiryChange(ctx, id, principal.ActionUpdate, func(bound context.Context) error {
		var err error
		out, err = s.updateRule(bound, id, in)
		return err
	})
	return out, err
}

// Archive archives a rule, under its form's fence as Update does.
func (s *AutomationStore) Archive(ctx context.Context, id ids.AutomationID) error {
	return s.withInquiryChange(ctx, id, principal.ActionDelete, func(bound context.Context) error { return s.archiveRule(bound, id) })
}

func (s *AutomationStore) withInquiryChange(ctx context.Context, id ids.AutomationID, action principal.Action, run func(context.Context) error) error {
	if err := auth.Require(ctx, "automation", action); err != nil {
		return err
	}
	var key string
	var form *string
	err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `SELECT key,params->>'form_id' FROM automation WHERE id=$1 AND archived_at IS NULL`, id).Scan(&key, &form)
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return apperrors.ErrNotFound
	}
	if err != nil {
		return err
	}
	if key != inquiryRecipeName {
		return run(ctx)
	}
	if form == nil || s.inquiries == nil {
		return errors.New("automation: inquiry change has no form fence")
	}
	formID, err := ids.Parse(*form)
	if err != nil {
		return err
	}
	return s.inquiries.Within(ctx, formID, run)
}
