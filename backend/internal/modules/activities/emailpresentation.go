// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	openapi_types "github.com/oapi-codegen/runtime/types"
)

// The composed read behind the canonical email viewer.
//
// It reads through readActivity rather than writing a third SELECT over
// activity: the withholding rule already exists twice in this tree, and the
// file comment on compose/person360's hand-written sibling records that the
// duplicate went missing a column for a whole slice, twice. What this file
// adds is what a single Activity row cannot carry — who the message went to,
// what came with it, and which write the caller's Access control performs.
//
// Every relation is read as its own bounded statement against the same
// transaction. One flat join would multiply participants by attachments by
// members, and a per-row read would be the N+1 the summary field exists to
// avoid.

// maxThreadMembers bounds one page of a conversation. A thread has no ceiling
// — a support exchange runs to hundreds — and a drawer that fetched every
// message would make opening the newest one cost the whole history.
const maxThreadMembers = 20

// GetEmailPresentation composes one email for reading.
func (s *Store) GetEmailPresentation(ctx context.Context, id ids.ActivityID, threadCursor *string) (crmcontracts.EmailPresentation, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	var out crmcontracts.EmailPresentation
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = readEmailPresentation(ctx, tx, id, threadCursor)
		return err
	})
	return out, err
}

func readEmailPresentation(ctx context.Context, tx pgx.Tx, id ids.ActivityID, threadCursor *string) (crmcontracts.EmailPresentation, error) {
	// The same discover gate and per-row audience test every activity read
	// carries: a caller who may know the row exists reads it, and the audience
	// decides whether its content comes along.
	activity, err := readActivity(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	// Not an email, so there is no email to present. Answered as not-found
	// rather than as a wrong-kind error: the caller asked for a resource that
	// does not exist at this address, and saying which kind it actually is
	// would answer a question about a row they may only discover.
	if activity.Kind != crmcontracts.ActivityKindEmail {
		return crmcontracts.EmailPresentation{}, apperrors.ErrNotFound
	}

	withheld := activity.ContentState != nil && *activity.ContentState == crmcontracts.ActivityContentStateWithheld
	out := crmcontracts.EmailPresentation{
		Id:         activity.Id,
		Lifecycle:  crmcontracts.EmailPresentationLifecycleDelivered,
		OccurredAt: activity.OccurredAt,
		ThreadKey:  activity.ThreadKey,
		Body:       activity.Body,
	}
	if activity.Version != nil {
		out.Version = *activity.Version
	}
	out.From, out.To, out.Cc, out.Bcc = emptyParties(), emptyParties(), emptyParties(), emptyParties()
	out.Attachments = []crmcontracts.EmailAttachmentSummary{}
	out.Links = []crmcontracts.ActivityLink{}

	if withheld {
		// Markers and nothing said. No parties, no files, no thread, no
		// reason: each of those describes the message this caller may not
		// read, and an empty list is a different statement from a refused one
		// only if the status says which.
		out.Summary = withheldSummary(activity)
		out.Access = withheldAccess()
		return out, nil
	}

	parties, err := readEmailParties(ctx, tx, id)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.From, out.To, out.Cc = parties.from, parties.to, parties.cc
	// Blind recipients are disclosed only to the seat that sent or imported the
	// message. Being allowed to read what was written is not standing to learn
	// who was copied without the others knowing — the audience gate answers the
	// first question and has no arm for the second.
	if sender, err := callerIsSenderSeat(ctx, tx, id); err != nil {
		return crmcontracts.EmailPresentation{}, err
	} else if sender {
		out.Bcc = parties.bcc
	}
	// Said rather than shown: an empty list reads as "nobody was blind-copied",
	// which is a different fact from "you may not see who was".
	out.BccWithheld = len(parties.bcc) > 0 && len(out.Bcc) == 0

	attachments, err := readEmailAttachments(ctx, tx, id)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.Attachments = attachments
	if activity.Links != nil {
		out.Links = *activity.Links
	}

	access, err := readEmailAccess(ctx, tx, id, activity)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.Access = access
	out.Summary = availableSummary(activity, parties, len(attachments), access.DisplayStatus)

	thread, err := readThreadPage(ctx, tx, activity, threadCursor)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.Thread = thread

	// Both actions are the caller's because the content is: a reader inside the
	// audience may answer the message and may correct what it is filed against.
	out.CanReply, out.CanRelink = true, true
	return out, nil
}

// availableSummary is the row for a message this caller may read.
func availableSummary(
	activity crmcontracts.Activity,
	parties emailParties,
	attachments int,
	status crmcontracts.EmailAccessStatus,
) crmcontracts.EmailSummary {
	summary := crmcontracts.EmailSummary{
		ActivityId:      activity.Id,
		OccurredAt:      activity.OccurredAt,
		DisplayStatus:   status,
		AttachmentCount: attachments,
		Move:            crmcontracts.EmailSummaryMoveNone,
		Subject:         activity.Subject,
	}
	if activity.Version != nil {
		summary.Version = *activity.Version
	}
	if activity.Direction != nil {
		d := crmcontracts.EmailSummaryDirection(*activity.Direction)
		summary.Direction = &d
	}
	if activity.Body != nil {
		if preview := EmailSummaryText(*activity.Body); preview != "" {
			summary.Preview = &preview
		}
	}
	// Who it was with: the far side of the exchange, which is the sender on a
	// message that came in and the recipients on one that went out.
	far := parties.to
	if activity.Direction != nil && *activity.Direction == crmcontracts.ActivityDirectionInbound {
		far = parties.from
	}
	summary.Counterparty = counterpartyOf(far)
	summary.Move = moveOf(activity)
	return summary
}

// moveOf says whose turn it is, from what this reader can see of the message
// itself. An inbound message nobody has answered is the reader's move; one we
// sent is theirs. Anything else makes no claim: a move nobody can derive
// honestly is worse on a row than no move at all, because a rep works the row
// that says they owe a reply.
func moveOf(activity crmcontracts.Activity) crmcontracts.EmailSummaryMove {
	if activity.Direction == nil {
		return crmcontracts.EmailSummaryMoveNone
	}
	switch *activity.Direction {
	case crmcontracts.ActivityDirectionInbound:
		return crmcontracts.EmailSummaryMoveNeedsReply
	case crmcontracts.ActivityDirectionOutbound:
		return crmcontracts.EmailSummaryMoveWaitingForThem
	default:
		return crmcontracts.EmailSummaryMoveNone
	}
}

// callerIsSenderSeat answers whether this caller is the seat the message went
// out as, or the mailbox owner who brought it in.
func callerIsSenderSeat(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID == (ids.UUID{}) {
		return false, nil
	}
	var sender bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		       SELECT 1 FROM capture_import ci
		        WHERE ci.activity_id = $1 AND ci.user_id = $2
		       UNION ALL
		       SELECT 1 FROM activity a
		        WHERE a.id = $1 AND a.captured_by LIKE '%:' || $2::text)`,
		id.UUID, actor.UserID).Scan(&sender); err != nil {
		return false, fmt.Errorf("activities: reading whether the caller sent this message: %w", err)
	}
	return sender, nil
}

// readEmailAttachments reads what came with the message. Reached only after
// the content gate above admitted the caller, so it carries no second gate of
// its own — the parent decided, exactly as ListAttachments' parent check does.
func readEmailAttachments(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]crmcontracts.EmailAttachmentSummary, error) {
	rows, err := tx.Query(ctx, `
		SELECT at.id, at.filename, at.byte_size, at.content_type
		  FROM attachment at
		 WHERE at.entity_type = 'activity' AND at.entity_id = $1 AND at.archived_at IS NULL
		 ORDER BY at.created_at, at.id`, id.UUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []crmcontracts.EmailAttachmentSummary{}
	for rows.Next() {
		var a crmcontracts.EmailAttachmentSummary
		var raw ids.UUID
		if err := rows.Scan(&raw, &a.Filename, &a.ByteSize, &a.ContentType); err != nil {
			return nil, err
		}
		a.Id = openapi_types.UUID(raw)
		out = append(out, a)
	}
	return out, rows.Err()
}

// emptyParties is an empty list rather than a nil one: a viewer that sees no
// `to` array cannot tell "nobody" from "the field was not sent", and the
// difference decides whether it renders a recipient row at all.
func emptyParties() []crmcontracts.EmailParty {
	return []crmcontracts.EmailParty{}
}

// withheldSummary is the row a caller outside the audience still sees: when it
// happened and that it happened, with the words removed.
func withheldSummary(activity crmcontracts.Activity) crmcontracts.EmailSummary {
	summary := crmcontracts.EmailSummary{
		ActivityId:      activity.Id,
		OccurredAt:      activity.OccurredAt,
		DisplayStatus:   crmcontracts.EmailAccessStatusWithheld,
		AttachmentCount: 0,
		Move:            crmcontracts.EmailSummaryMoveNone,
	}
	if activity.Version != nil {
		summary.Version = *activity.Version
	}
	if activity.Direction != nil {
		d := crmcontracts.EmailSummaryDirection(*activity.Direction)
		summary.Direction = &d
	}
	return summary
}

func withheldAccess() crmcontracts.EmailAccess {
	return crmcontracts.EmailAccess{
		ContentState:  crmcontracts.EmailAccessContentStateWithheld,
		DisplayStatus: crmcontracts.EmailAccessStatusWithheld,
		CanChange:     false,
		ChangeMode:    crmcontracts.EmailAccessChangeModeNone,
	}
}

// readEmailAccess assembles who reads this message and what this caller may do
// about it.
//
// change_mode is decided here, by the same test the write itself applies:
// refuseCapturedAudienceWrite refuses a direct audience write on a message any
// mailbox brought in, because a captured message's audience is derived from
// its importers rather than declared. The browser has been guessing this from
// the "connector:" prefix on captured_by, which puts a backend ownership rule
// in display code and gets a hand-typed threaded reply wrong. The server knows
// which write it would accept, so the server says.
func readEmailAccess(
	ctx context.Context,
	tx pgx.Tx,
	id ids.ActivityID,
	activity crmcontracts.Activity,
) (crmcontracts.EmailAccess, error) {
	out := crmcontracts.EmailAccess{
		ContentState:  crmcontracts.EmailAccessContentStateAvailable,
		ChangeMode:    crmcontracts.EmailAccessChangeModeNone,
		ChangeScope:   ptr(crmcontracts.EmailAccessChangeScopeNone),
		DisplayStatus: crmcontracts.EmailAccessStatusTeam,
	}
	if activity.Audience != nil {
		aud := crmcontracts.ActivityAudience(*activity.Audience)
		out.Audience = &aud
		out.DisplayStatus = statusForAudience(aud)
	}
	// The reason is content: it describes what the message is about. It travels
	// only with a message the caller may read, which is the branch this is in.
	out.Explanation = activity.AudienceReason

	var imported bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM capture_import WHERE activity_id = $1)`,
		id.UUID).Scan(&imported); err != nil {
		return crmcontracts.EmailAccess{}, fmt.Errorf("activities: reading whether a message was imported: %w", err)
	}

	writable := auth.EnsureActivityWritable(ctx, tx, id.UUID) == nil
	switch {
	case imported:
		// A captured message: the caller changes their own contribution to the
		// thread, and only their own. Every importing seat holds one, so this
		// is offered to an importer rather than to a writer of the row.
		sender, err := callerIsSenderSeat(ctx, tx, id)
		if err != nil {
			return crmcontracts.EmailAccess{}, err
		}
		out.CanChange = sender
		if sender {
			out.ChangeMode = crmcontracts.EmailAccessChangeModeThreadContribution
			out.ChangeScope = ptr(crmcontracts.EmailAccessChangeScopeThread)
		}
	case writable:
		// Hand-logged: its audience is exactly what somebody set, so a writer
		// of the row sets it.
		out.CanChange = true
		out.ChangeMode = crmcontracts.EmailAccessChangeModeMessageAudience
		out.ChangeScope = ptr(crmcontracts.EmailAccessChangeScopeMessage)
	}

	// Who is named on a selected audience, read back only for the caller who
	// may change the set. A reader with no standing to edit it has none to
	// enumerate it either.
	if out.CanChange && activity.Audience != nil && *activity.Audience == crmcontracts.ActivityAudienceSelected {
		members, err := readSelectedMembers(ctx, tx, id)
		if err != nil {
			return crmcontracts.EmailAccess{}, err
		}
		out.SelectedMembers = &members
	}
	return out, nil
}

// statusForAudience is the word the badge prints for a message the caller can
// read. "team" never means the whole workspace: the linked record's own scope
// still decides who may discover the row at all.
func statusForAudience(aud crmcontracts.ActivityAudience) crmcontracts.EmailAccessStatus {
	switch aud {
	case crmcontracts.ActivityAudienceParticipants:
		return crmcontracts.EmailAccessStatusParticipants
	case crmcontracts.ActivityAudienceSelected:
		return crmcontracts.EmailAccessStatusSelected
	default:
		return crmcontracts.EmailAccessStatusTeam
	}
}

func readSelectedMembers(ctx context.Context, tx pgx.Tx, id ids.ActivityID) ([]crmcontracts.AudienceMember, error) {
	rows, err := tx.Query(ctx, `
		SELECT subject_type, subject_id
		  FROM activity_audience_member
		 WHERE activity_id = $1
		 ORDER BY subject_type, subject_id`, id.UUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []crmcontracts.AudienceMember{}
	for rows.Next() {
		var subjectType string
		var subjectID ids.UUID
		if err := rows.Scan(&subjectType, &subjectID); err != nil {
			return nil, err
		}
		out = append(out, crmcontracts.AudienceMember{
			SubjectType: crmcontracts.AudienceMemberSubjectType(subjectType),
			SubjectId:   openapi_types.UUID(subjectID),
		})
	}
	return out, rows.Err()
}

// readThreadPage reads the rest of the conversation as summaries, newest
// first. Bounded and paged: a thread has no ceiling, and a drawer that fetched
// every message would make opening the newest one cost the whole history.
//
// It runs the same gated list every timeline runs, so a member the caller may
// not read comes back withheld rather than missing — the drawer says a message
// is there and not theirs, which is the honest shape of a conversation with a
// limited message in it.
func readThreadPage(
	ctx context.Context,
	tx pgx.Tx,
	activity crmcontracts.Activity,
	cursor *string,
) (*crmcontracts.EmailThreadPage, error) {
	if activity.ThreadKey == nil || *activity.ThreadKey == "" {
		return nil, nil
	}
	key := *activity.ThreadKey
	limit := maxThreadMembers
	members, page, err := ListActivitiesTx(ctx, tx, ListActivitiesInput{
		ThreadKey: &key,
		Cursor:    cursor,
		Limit:     &limit,
	})
	if err != nil {
		return nil, err
	}
	out := crmcontracts.EmailThreadPage{Members: []crmcontracts.EmailSummary{}}
	for _, member := range members {
		if member.Kind != crmcontracts.ActivityKindEmail {
			continue
		}
		if member.EmailSummary != nil {
			out.Members = append(out.Members, *member.EmailSummary)
		}
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return &out, nil
}

func ptr[T any](v T) *T { return &v }

type emailParties struct {
	from, to, cc, bcc []crmcontracts.EmailParty
}

// readEmailParties reads the message's normalised recipients. They are stored
// as activity_participant rows with a role, which is why the viewer never has
// to parse a provider payload to learn who was on a message.
//
// A person's name is resolved only through the caller's own row scope: an
// address the caller may not see a contact for stays an address, which is the
// truth rather than a blank.
func readEmailParties(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (emailParties, error) {
	args := []any{id}
	arg := func(v any) int { args = append(args, v); return len(args) }
	scope, err := auth.ScopeClauseFor(ctx, "person", "p", arg)
	if err != nil {
		return emailParties{}, err
	}
	personJoin := `LEFT JOIN person p ON p.id = ap.person_id AND p.archived_at IS NULL`
	if scope != "" {
		personJoin += ` AND (` + scope + `)`
	}
	rows, err := tx.Query(ctx, `
		SELECT ap.role, coalesce(ap.address, ''), p.id, p.full_name, ap.user_id
		  FROM activity_participant ap
		  `+personJoin+`
		 WHERE ap.activity_id = $1
		   AND ap.role IN ('from', 'to', 'cc', 'bcc')
		 ORDER BY CASE ap.role
		            WHEN 'from' THEN 1 WHEN 'to' THEN 2 WHEN 'cc' THEN 3 ELSE 4
		          END, ap.created_at, ap.id`, args...)
	if err != nil {
		return emailParties{}, err
	}
	defer rows.Close()

	var out emailParties
	for rows.Next() {
		var role, address string
		var personID, userID *ids.UUID
		var fullName *string
		if err := rows.Scan(&role, &address, &personID, &fullName, &userID); err != nil {
			return emailParties{}, err
		}
		party := crmcontracts.EmailParty{Address: address, DisplayName: fullName}
		if personID != nil {
			pid := openapi_types.UUID(*personID)
			party.PersonId = &pid
		}
		if userID != nil {
			uid := openapi_types.UUID(*userID)
			party.UserId = &uid
		}
		switch role {
		case "from":
			out.from = append(out.from, party)
		case "to":
			out.to = append(out.to, party)
		case "cc":
			out.cc = append(out.cc, party)
		case "bcc":
			out.bcc = append(out.bcc, party)
		}
	}
	return out, rows.Err()
}

// counterpartyOf names the other side for a row: the first party the caller
// can name, and how many more there were. A message whose participants all
// resolve to nothing gets no counterparty rather than an invented stranger.
func counterpartyOf(parties []crmcontracts.EmailParty) *string {
	if len(parties) == 0 {
		return nil
	}
	var named string
	for _, p := range parties {
		if p.DisplayName != nil && strings.TrimSpace(*p.DisplayName) != "" {
			named = *p.DisplayName
			break
		}
		if named == "" && p.Address != "" {
			named = p.Address
		}
	}
	if named == "" {
		return nil
	}
	if extra := len(parties) - 1; extra > 0 {
		named += " +" + strconv.Itoa(extra)
	}
	return &named
}
