// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/momentaction"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// taskSource is where a task row came from: the message its promise was read
// out of, and the system that wrote it.
type taskSource struct {
	activity *ids.UUID
	system   string
}

// askTasksDone marks each promise task on the page with the question the
// Worklist asks of it. A promise kept by mail then reads the same on both.
func (s *Service) askTasksDone(
	ctx context.Context, tx pgx.Tx, steps []crmcontracts.Company360NextStep, sources []taskSource, now time.Time,
) error {
	// Only a human answers the question, so an agent reads the plain row.
	if !auth.Allows(ctx, "activity", principal.ActionRead) {
		return nil
	}
	if err := auth.RequireHuman(ctx); errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	} else if err != nil {
		return err
	}
	candidates, err := s.promiseCandidates(ctx, steps, sources)
	if err != nil || len(candidates) == 0 {
		return err
	}
	asked, err := momentaction.AskTasksDone(ctx, tx, candidates, now)
	if errors.Is(err, apperrors.ErrPermissionDenied) {
		return nil
	}
	if err != nil {
		return err
	}
	for i := range steps {
		if question, ok := asked[ids.UUID(steps[i].ActivityId)]; ok {
			// A row that asks is not late until the reader answers Not yet.
			steps[i].MayBeDone, steps[i].Overdue = &question, false
		}
	}
	return nil
}

// promiseCandidates are the page's promise tasks, each with the recipients
// whose claim on it is still open.
func (s *Service) promiseCandidates(
	ctx context.Context, steps []crmcontracts.Company360NextStep, sources []taskSource,
) ([]momentaction.DoneCandidate, error) {
	tasks := make([]momentaction.PromiseTask, len(steps))
	for i, step := range steps {
		tasks[i] = momentaction.PromiseTask{ID: ids.UUID(step.ActivityId), System: sources[i].system, Source: sources[i].activity, Assignee: (*ids.UUID)(step.AssigneeId)}
	}
	return momentaction.DoneCandidates(ctx, s.contacts, tasks)
}
