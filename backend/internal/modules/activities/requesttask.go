// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// takeEmailRequest is the explicit review path for historical or unassigned
// requests. Lock the source before testing replay, just as the background pass
// does, so a review racing capture cannot create two reminders.
func (s *Store) takeEmailRequest(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	userID, err := requestAcceptingUser(ctx, in)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	id := *in.RequestActivityID
	if _, err := storekit.LockRow(ctx, tx, "activity", id, storekit.LiveOnly); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	args := []any{id}
	idPos := len(args)
	args = append(args, s.now())
	instant := fmt.Sprintf("$%d", len(args))
	var pending bool
	if err := tx.QueryRow(ctx, fmt.Sprintf(`SELECT coalesce((%s) AND a.occurred_at <= %s, false) FROM activity a WHERE a.id = $%d`, reviewableRequestSQL(instant), instant, idPos), args...).Scan(&pending); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	source, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](id), storekit.LiveOnly)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	request := emailRequestTaskInput(source, userID, s.now())
	// Replay carries the source's content gate AND the task's: a caller cannot
	// use a visible source to obtain somebody else's private reminder.
	replay, err := replayedActivity(ctx, tx, request)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if replay != nil {
		return replayRequestTask(ctx, tx, *replay, pending)
	}
	if !pending {
		return crmcontracts.Activity{}, false, apperrors.ErrConflict
	}
	if in.Subject != nil && strings.TrimSpace(*in.Subject) != "" {
		request.Subject = in.Subject
	}
	request.Body = in.Body
	request.DueAt, request.RemindAt = in.DueAt, in.RemindAt
	return s.LogActivityTx(ctx, tx, request)
}

func requestAcceptingUser(ctx context.Context, in LogActivityInput) (ids.UUID, error) {
	if in.Kind != string(crmcontracts.ActivityKindTask) {
		return ids.UUID{}, &RequestAcceptanceFieldError{Field: "request_activity_id", Message: "Only a task can accept a request."}
	}
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return ids.UUID{}, err
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman {
		return ids.UUID{}, apperrors.ErrPermissionDenied
	}
	if in.AssigneeID != nil && in.AssigneeID.UUID != actor.UserID {
		return ids.UUID{}, &RequestAcceptanceFieldError{Field: fieldAssignee, Message: "Accept the request for yourself; reassign the reminder after reviewing its source access."}
	}
	return actor.UserID, nil
}

func replayRequestTask(ctx context.Context, tx pgx.Tx, replay crmcontracts.Activity, pending bool) (crmcontracts.Activity, bool, error) {
	task, err := readActivityContent(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(replay.Id)), storekit.IncludeArchived)
	if err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return crmcontracts.Activity{}, false, apperrors.ErrConflict
		}
		return crmcontracts.Activity{}, false, err
	}
	if task.ArchivedAt != nil && (task.IsDone == nil || !*task.IsDone) && !pending {
		return crmcontracts.Activity{}, false, apperrors.ErrConflict
	}
	return restoreRequestTask(ctx, tx, task)
}

func emailRequestTaskInput(source crmcontracts.Activity, userID ids.UUID, asOf time.Time) LogActivityInput {
	links := []ActivityLinkInput{}
	if source.Links != nil {
		for _, link := range *source.Links {
			links = append(links, ActivityLinkInput{EntityType: string(link.EntityType), EntityID: ids.UUID(link.EntityId)})
		}
	}
	subject := "Reply to email"
	if source.Subject != nil && strings.TrimSpace(*source.Subject) != "" {
		subject = *source.Subject
	}
	sourceSystem, sourceID := EmailRequestTaskSource, source.Id.String()
	messageID, assignee := ids.UUID(source.Id), ids.From[ids.UserKind](userID)
	return LogActivityInput{
		audienceMembers: []AudienceMember{{SubjectType: string(crmcontracts.AudienceMemberSubjectTypeUser), SubjectID: userID}},
		Kind:            string(crmcontracts.ActivityKindTask), Subject: &subject, OccurredAt: &asOf, AssigneeID: &assignee,
		SourceSystem: &sourceSystem, SourceID: &sourceID, SourceActivityID: &messageID,
		Source: EmailRequestTaskSource, Origin: OriginSystemRemediation, Links: links,
	}
}
