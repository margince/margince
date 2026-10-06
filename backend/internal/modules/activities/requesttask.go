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
	seat, err := auth.SeatPrincipal(ctx, tx, userID)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	request, err := emailRequestTask(ctx, tx, source, seat, s.now())
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
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

// emailRequestTask is the reply reminder for one request, filed under who
// asked: "Needs attention" on a contact's page means that contact is waiting
// for us, and one only copied on the mail is not.
//
// The automatic pass and a human taking the request both land here, and both
// decide from the source's whole link set as `seat` — the assignee, from
// auth.SeatPrincipal — sees it, so the two doors file one request on the same
// pages.
func emailRequestTask(ctx context.Context, tx pgx.Tx, source crmcontracts.Activity, seat principal.Principal, asOf time.Time) (LogActivityInput, error) {
	filed, err := readRequestFiling(ctx, tx, ids.UUID(source.Id), seat)
	if err != nil {
		return LogActivityInput{}, err
	}
	return emailRequestTaskInput(source, requesterLinks(filed), seat.UserID, asOf), nil
}

// requestFiling is where a request is filed, as its assignee can see it.
type requestFiling struct {
	// senderKnown is false for a message with no sender row at all, which
	// older capture wrote.
	senderKnown bool
	links       []filedLink
}

type filedLink struct {
	ActivityLinkInput
	// fromSender marks a link to a contact who sent the message.
	fromSender bool
}

// readRequestFiling reads the source's live links the seat can open — read
// whole rather than through the caller's own scope, because the system pass
// sees every link and a human taking the request sees only theirs — and which
// of them sent it.
func readRequestFiling(ctx context.Context, tx pgx.Tx, message ids.UUID, seat principal.Principal) (requestFiling, error) {
	var args []any
	arg := func(v any) int { args = append(args, v); return len(args) }
	messagePos := arg(message)
	visible, err := auth.LinkTargetVisibleClause(principal.WithActor(ctx, seat), "l", arg)
	if err != nil {
		return requestFiling{}, err
	}
	if visible == "" {
		visible = scopeUnbounded
	}
	rows, err := tx.Query(ctx, fmt.Sprintf(`
		SELECT l.entity_type, %[1]s,
		       EXISTS (SELECT 1 FROM activity_participant p
		                WHERE p.activity_id = l.activity_id AND p.role = 'from' AND p.contact_id = l.contact_id),
		       EXISTS (SELECT 1 FROM activity_participant p WHERE p.activity_id = l.activity_id AND p.role = 'from')
		  FROM activity_link l
		 WHERE l.activity_id = $%[2]d AND %[3]s AND %[4]s
		 ORDER BY l.id`, linkIDCoalesceQualified("l"), messagePos, linkTargetLive("l"), visible), args...)
	if err != nil {
		return requestFiling{}, fmt.Errorf("activities: reading where a request is filed: %w", err)
	}
	var filed requestFiling
	var link filedLink
	if _, err := pgx.ForEachRow(rows, []any{&link.EntityType, &link.EntityID, &link.fromSender, &filed.senderKnown}, func() error {
		filed.links = append(filed.links, link)
		return nil
	}); err != nil {
		return requestFiling{}, fmt.Errorf("activities: reading where a request is filed: %w", err)
	}
	return filed, nil
}

// requesterLinks keeps the links to the contacts who sent the request. A
// sender filed under no contact the assignee can see leaves the reminder on
// the other records instead, so it still sits on a page; a message that never
// recorded its sender keeps every link, which is all it can say.
func requesterLinks(filed requestFiling) []ActivityLinkInput {
	all, requesters, records := []ActivityLinkInput{}, []ActivityLinkInput{}, []ActivityLinkInput{}
	for _, link := range filed.links {
		all = append(all, link.ActivityLinkInput)
		switch {
		case link.EntityType != linkEntityContact:
			records = append(records, link.ActivityLinkInput)
		case link.fromSender:
			requesters = append(requesters, link.ActivityLinkInput)
		}
	}
	switch {
	case !filed.senderKnown:
		return all
	case len(requesters) > 0:
		return requesters
	default:
		return records
	}
}

func emailRequestTaskInput(source crmcontracts.Activity, links []ActivityLinkInput, userID ids.UUID, asOf time.Time) LogActivityInput {
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
