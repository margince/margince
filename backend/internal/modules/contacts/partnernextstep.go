// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// PartnerNextStep is the one next thing to do with a partner. It is written
// as a task on the partner's company, so it has a colleague who does it and
// reaches their Home and Worklist. The partner row keeps no new text.
type PartnerNextStep struct {
	CompanyID ids.CompanyID
	Step      string
	// Due is the day the step is due, or nil for none.
	Due *time.Time
}

// PartnerNextSteps writes a partner's next step as a task in the partner
// write's own transaction. Compose binds it to the activities task writer.
type PartnerNextSteps interface {
	WriteTx(ctx context.Context, tx pgx.Tx, step PartnerNextStep) error
}

// errNoNextStepWriter refuses a next step on a store composed without the
// task writer, rather than storing it as text nobody does.
var errNoNextStepWriter = errors.New("contacts: a partner next step needs the task writer, and this store has none")

// WithPartnerNextSteps wires the task writer a partner's next step goes to.
func (s *Store) WithPartnerNextSteps(steps PartnerNextSteps) *Store {
	s.partnerNextSteps = steps
	return s
}

// WithPartnerNextSteps mirrors the store option for the HTTP surface.
func (h Handlers) WithPartnerNextSteps(steps PartnerNextSteps) Handlers {
	h.store = h.store.WithPartnerNextSteps(steps)
	return h
}

// takePartnerNextStep moves a next step out of a partner write. The step
// becomes a task, and the row's old text and date are left as they stand. A
// due day without a step is refused, because a task needs words.
func takePartnerNextStep(in *UpsertPartnerInput) (*PartnerNextStep, error) {
	step, due := in.NextStep, in.NextStepDueAt
	in.NextStep, in.NextStepDueAt = nil, nil
	if step == nil || !values.HasVisibleText(*step) {
		if due != nil {
			return nil, httperr.Validation(partnerFieldNextStepDue, "invalid",
				"next_step_due_at needs next_step: say what is due, and it becomes a task")
		}
		return nil, nil //nolint:nilnil // No step was sent, so there is nothing to write.
	}
	return &PartnerNextStep{CompanyID: in.CompanyID, Step: strings.TrimSpace(*step), Due: due}, nil
}

// writePartnerNextStepTx writes the step, when the write carried one.
func (s *Store) writePartnerNextStepTx(ctx context.Context, tx pgx.Tx, step *PartnerNextStep) error {
	if step == nil {
		return nil
	}
	if s.partnerNextSteps == nil {
		return errNoNextStepWriter
	}
	return s.partnerNextSteps.WriteTx(ctx, tx, *step)
}
