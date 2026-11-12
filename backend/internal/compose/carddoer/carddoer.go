// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package carddoer decides the colleague who does the work a card proposes.
// Whoever accepts a promise or a follow-up card may name another
// colleague in its doer member (approvals.DoerField). Empty keeps the card's
// own rule.
package carddoer

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/approvals"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Chosen is the colleague named in a card's doer member, or nil when the
// accepter kept the default.
func Chosen(raw string) (*ids.UUID, error) {
	if raw == "" {
		return nil, nil
	}
	doer, err := ids.Parse(raw)
	if err != nil || doer.IsZero() {
		return nil, &approvals.InvalidEditError{Cause: fmt.Errorf("%s %q names no colleague", approvals.DoerField, raw)}
	}
	return &doer, nil
}

// EnsureCanHoldWork refuses a chosen colleague the task writer would
// refuse, before the decision commits, so the card stays pending.
func EnsureCanHoldWork(ctx context.Context, tx pgx.Tx, doer ids.UUID) error {
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](doer))
	if err != nil {
		return err
	}
	if state == activities.SeatCannotHoldWork {
		return &approvals.InvalidEditError{Cause: errors.New(
			"the colleague chosen to do this cannot be given tasks — choose an active colleague")}
	}
	return nil
}

// ForPromise is who keeps an accepted promise: the colleague the accepter
// chose, else the accepter. A promise about an existing task stays with that
// task's holder, who hands it on from the task itself.
func ForPromise(raw string, aboutExistingTask bool, decider ids.UUID) (ids.UUID, error) {
	chosen, err := Chosen(raw)
	if err != nil || chosen == nil {
		return decider, err
	}
	if aboutExistingTask {
		return ids.Nil, &approvals.InvalidEditError{Cause: errors.New(
			"this promise adds recipients to a task that already has a holder — hand the task on from the task instead")}
	}
	return *chosen, nil
}
