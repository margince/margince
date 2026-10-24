// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

type followUpFieldError struct{ Field, Message string }

func (e *followUpFieldError) Error() string { return e.Message }

// FieldFault reports the refused source or assignee through either write surface.
func (e *followUpFieldError) FieldFault() (string, string, string) {
	return e.Field, faultInvalid, e.Message
}

func followUpFieldsFrom(req crmcontracts.CreateActivityRequest, in *LogActivityInput) error {
	if req.SourceActivityId != nil {
		in.SourceActivityID = new(ids.UUID(*req.SourceActivityId))
	}
	if req.FollowUpTask == nil {
		return nil
	}
	task := req.FollowUpTask
	followUp, err := TaskInputFrom(crmcontracts.CreateTaskRequest{
		Subject: task.Subject, Body: task.Body, DueAt: task.DueAt,
		AssigneeId: task.AssigneeId, Source: req.Source,
	})
	if err != nil {
		return err
	}
	in.FollowUpTask = &followUp
	return nil
}

func validateFollowUpShape(in LogActivityInput) error {
	if in.FollowUpTask != nil {
		if in.Kind != string(crmcontracts.ActivityKindMeeting) && in.Kind != string(crmcontracts.ActivityKindCall) && in.Kind != string(crmcontracts.ActivityKindNote) {
			return &followUpFieldError{Field: "follow_up_task", Message: "Only a meeting, call or note can create a follow-up task."}
		}
		if in.FollowUpTask.Kind != string(crmcontracts.ActivityKindTask) {
			return &followUpFieldError{Field: "follow_up_task", Message: "A follow-up must be a task."}
		}
	}
	if in.SourceActivityID == nil {
		return nil
	}
	if in.Kind != string(crmcontracts.ActivityKindTask) || in.RequestActivityID != nil {
		return &followUpFieldError{Field: "source_activity_id", Message: "Only a standalone task can name its source. Accept a request through request_activity_id instead."}
	}
	return nil
}

func prepareFollowUpSource(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	if err := validateFollowUpShape(*in); err != nil {
		return err
	}
	if in.SourceActivityID == nil {
		return nil
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return err
	}
	source, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](*in.SourceActivityID), storekit.LiveOnly)
	if err != nil {
		return err
	}
	if source.Kind == crmcontracts.ActivityKindTask {
		return &followUpFieldError{Field: "source_activity_id", Message: "Choose a conversation or note as the source, rather than another task."}
	}
	assignee := taskAssignee(ctx, *in)
	if assignee != nil {
		seat, err := auth.SeatPrincipal(ctx, tx, assignee.UUID)
		if err != nil {
			return err
		}
		err = auth.EnsureActivityContentVisibleLive(principal.WithActor(ctx, seat), tx, *in.SourceActivityID)
		if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
			return &followUpFieldError{Field: fieldAssignee, Message: "Choose a colleague who can read the source conversation."}
		}
		if err != nil {
			return err
		}
	}
	if len(in.Links) == 0 {
		actor, ok := principal.Actor(ctx)
		if !ok {
			return apperrors.ErrPermissionDenied
		}
		filing, err := readRequestFiling(ctx, tx, *in.SourceActivityID, actor)
		if err != nil {
			return err
		}
		for _, link := range filing.links {
			in.Links = append(in.Links, link.ActivityLinkInput)
		}
	}
	return nil
}

// prepareLoggedActivity settles the input before anything reads it: the text a
// row of its kind stores, its follow-up source and its duration.
func prepareLoggedActivity(ctx context.Context, tx pgx.Tx, in *LogActivityInput) error {
	maskTextOfKind(crmcontracts.ActivityKind(in.Kind), &in.Subject, &in.Body)
	if err := prepareFollowUpSource(ctx, tx, in); err != nil {
		return err
	}
	return validateActivityDuration(in.DurationSeconds)
}
