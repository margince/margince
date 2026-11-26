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
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Chosen is the colleague named in a card's doer member, or the zero id when
// the accepter kept the default.
func Chosen(raw string) (ids.UUID, error) {
	if raw == "" {
		return ids.Nil, nil
	}
	doer, err := ids.Parse(raw)
	if err != nil || doer.IsZero() {
		return ids.Nil, &approvals.InvalidEditError{Cause: fmt.Errorf("%s %q names no colleague", approvals.DoerField, raw)}
	}
	return doer, nil
}

// EnsureMayHold refuses a chosen colleague the task writer would refuse. It
// also refuses one who may not read every record the task is filed under,
// because the task's words come from those records. It runs before the
// decision commits, so the card stays pending.
func EnsureMayHold(
	ctx context.Context, tx pgx.Tx, doer ids.UUID, records []activities.ActivityLinkInput, asColleague ColleagueAuthority,
) error {
	state, err := activities.NewTaskSeatState(ctx, tx, ids.From[ids.UserKind](doer))
	if err != nil {
		return err
	}
	colleague, err := asColleague(ctx, doer)
	if state != activities.SeatActive || errors.Is(err, apperrors.ErrNotFound) {
		// An invited colleague has no grants to read the records with yet.
		return &approvals.InvalidEditError{Cause: errors.New(
			"the colleague chosen to do this cannot be given tasks yet — choose an active colleague")}
	}
	if err != nil {
		return err
	}
	for _, record := range records {
		may, err := auth.VisibleToExisting(colleague, tx, record.EntityType, record.EntityID)
		if err != nil {
			return err
		}
		if !may {
			return &approvals.InvalidEditError{Cause: fmt.Errorf(
				"the colleague chosen to do this cannot open the %s it is about — choose one who can", record.EntityType)}
		}
	}
	return nil
}

// ColleagueAuthority binds one colleague's own grants as the acting principal.
type ColleagueAuthority func(ctx context.Context, colleague ids.UUID) (context.Context, error)

// ForPromise is who keeps an accepted promise: the colleague the accepter
// chose, else the accepter. A promise about an existing task stays with that
// task's holder, who hands it on from the task itself.
func ForPromise(raw string, aboutExistingTask bool, decider ids.UUID) (ids.UUID, error) {
	chosen, err := Chosen(raw)
	if err != nil || chosen.IsZero() {
		return decider, err
	}
	if aboutExistingTask {
		return ids.Nil, &approvals.InvalidEditError{Cause: errors.New(
			"this promise adds recipients to a task that already has a holder — hand the task on from the task instead")}
	}
	return chosen, nil
}
