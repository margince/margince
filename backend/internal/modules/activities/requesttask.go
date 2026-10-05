// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"slices"
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
	request, err := emailRequestTask(ctx, tx, source, userID, s.now())
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
// decide from the source's whole link set as the ASSIGNEE sees it, so the two
// doors file one request on the same pages.
func emailRequestTask(ctx context.Context, tx pgx.Tx, source crmcontracts.Activity, userID ids.UUID, asOf time.Time) (LogActivityInput, error) {
	message := ids.UUID(source.Id)
	filed, err := assigneeVisibleLinks(ctx, tx, message, userID)
	if err != nil {
		return LogActivityInput{}, err
	}
	sender, err := readRequestSender(ctx, tx, message, filed)
	if err != nil {
		return LogActivityInput{}, err
	}
	return emailRequestTaskInput(source, requesterLinks(filed, sender), userID, asOf), nil
}

// requestSender is who a request came from, as its participants record it.
type requestSender struct {
	// known is false for a message with no sender row at all, which older
	// capture wrote.
	known bool
	// contacts are the filed contacts among the senders.
	contacts []ids.UUID
}

// readRequestSender answers which of the contacts the request is filed under
// sent it — a subset of links already bounded to what the assignee sees.
func readRequestSender(ctx context.Context, tx pgx.Tx, message ids.UUID, filed []ActivityLinkInput) (requestSender, error) {
	candidates := []ids.UUID{}
	for _, link := range filed {
		if link.EntityType == linkEntityContact {
			candidates = append(candidates, link.EntityID)
		}
	}
	var sender requestSender
	if err := tx.QueryRow(ctx, `
		SELECT count(*) > 0, coalesce(array_agg(p.contact_id) FILTER (WHERE p.contact_id = ANY($2)), '{}')
		  FROM activity_participant p
		 WHERE p.activity_id = $1 AND p.role = 'from'`, message, candidates).Scan(&sender.known, &sender.contacts); err != nil {
		return requestSender{}, fmt.Errorf("activities: reading who sent a request: %w", err)
	}
	return sender, nil
}

// assigneeVisibleLinks is the source's links the assignee can open, read whole
// rather than through the caller's own scope: the system pass sees every link
// and a human taking the request sees only theirs, and the reminder must not
// depend on which of them filed it.
func assigneeVisibleLinks(ctx context.Context, tx pgx.Tx, message, assignee ids.UUID) ([]ActivityLinkInput, error) {
	rows, err := tx.Query(ctx, `SELECT entity_type, `+linkIDCoalesce+` FROM activity_link WHERE activity_id = $1 ORDER BY id`, message)
	if err != nil {
		return nil, fmt.Errorf("activities: reading a request's links: %w", err)
	}
	links, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ActivityLinkInput, error) {
		var link ActivityLinkInput
		err := row.Scan(&link.EntityType, &link.EntityID)
		return link, err
	})
	if err != nil {
		return nil, fmt.Errorf("activities: reading a request's links: %w", err)
	}
	byTable := map[string][]ids.UUID{}
	for _, link := range links {
		byTable[link.EntityType] = append(byTable[link.EntityType], link.EntityID)
	}
	seen := map[string]map[ids.UUID]bool{}
	for table, rowIDs := range byTable {
		if seen[table], err = auth.SeatSees(ctx, tx, assignee, table, rowIDs); err != nil {
			return nil, err
		}
	}
	return slices.DeleteFunc(links, func(link ActivityLinkInput) bool { return !seen[link.EntityType][link.EntityID] }), nil
}

// requesterLinks keeps the links to the contacts who sent the request. A
// sender filed under no contact the assignee can see leaves the reminder on
// the other records instead, so it still sits on a page; a message that never
// recorded its sender keeps every link, which is all it can say.
func requesterLinks(filed []ActivityLinkInput, sender requestSender) []ActivityLinkInput {
	if !sender.known {
		return filed
	}
	requesters, records := []ActivityLinkInput{}, []ActivityLinkInput{}
	for _, link := range filed {
		switch {
		case link.EntityType != linkEntityContact:
			records = append(records, link)
		case slices.Contains(sender.contacts, link.EntityID):
			requesters = append(requesters, link)
		}
	}
	if len(requesters) > 0 {
		return requesters
	}
	return records
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
