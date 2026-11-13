// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	openapi_types "github.com/oapi-codegen/runtime/types"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// The composed read behind the canonical email viewer.
//
// It reads through readActivity rather than writing a third SELECT over
// activity: the withholding rule already exists twice in this tree, and the
// file comment on compose/contact360's hand-written sibling records that the
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

// The participant roles this viewer reads. A closed set, named so the query
// and the scan cannot spell one of them differently.
const (
	roleFrom = "from"
	roleTo   = "to"
	roleCc   = "cc"
	roleBcc  = connector.ParticipantRoleBCC
)

// GetEmailPresentation composes one email for reading.
func (s *Store) GetEmailPresentation(ctx context.Context, id ids.ActivityID, threadCursor *string) (crmcontracts.EmailPresentation, error) {
	if err := auth.Require(ctx, "activity", principal.ActionRead); err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	var out crmcontracts.EmailPresentation
	err := s.tx(ctx, func(tx pgx.Tx) (err error) {
		out, err = readEmailPresentation(ctx, tx, id, threadCursor, s.now())
		return err
	})
	return out, err
}

func readEmailPresentation(ctx context.Context, tx pgx.Tx, id ids.ActivityID, threadCursor *string, asOf time.Time) (crmcontracts.EmailPresentation, error) {
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
		Id:                activity.Id,
		Lifecycle:         crmcontracts.EmailPresentationLifecycleDelivered,
		OccurredAt:        activity.OccurredAt,
		OccurredPrecision: activity.OccurredPrecision,
		ThreadKey:         activity.ThreadKey,
		Body:              activity.Body,
	}
	if activity.Version != nil {
		out.Version = *activity.Version
	}
	out.From, out.To, out.Cc, out.Bcc = emptyParties(), emptyParties(), emptyParties(), emptyParties()
	out.Attachments = []crmcontracts.EmailAttachmentSummary{}
	out.Links = []crmcontracts.ActivityLink{}

	if withheld {
		return withheldPresentation(out, activity), nil
	}
	out.BodyCompleteness = activity.BodyCompleteness
	if activity.Language != nil {
		out.Language = new(crmcontracts.EmailPresentationLanguage(*activity.Language))
	}

	parties, err := readEmailParties(ctx, tx, id)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.From, out.To, out.Cc = parties.from, parties.to, parties.cc
	// Blind recipients are disclosed only to the seat that sent or imported the
	// message. Being allowed to read what was written is not, in general,
	// standing to learn who was copied without the others knowing.
	//
	// The one caller for whom this is the SAME question rather than a narrower
	// one is an importing seat: a capture_import row already grants the whole
	// audience arm, so it grants this too. That is the reach of the forged
	// Message-ID path importrow.go documents, not a door opened here.
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

	access, writable, err := readEmailAccess(ctx, tx, id, activity)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.Access = access
	out.Summary = availableSummary(activity, parties, len(attachments), access.DisplayStatus)
	// What HAPPENED to it. Read after the access gate for the reason the
	// attachment count is: whether a message left is something about the
	// message, so the caller's own gate is what decides.
	if out.ControllerSend, err = withEmailStateOn(ctx, tx, activity.Id, &out.Summary, asOf); err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	if err := markCalendarFacts(ctx, tx, id, &out.Summary, asOf); err != nil {
		return crmcontracts.EmailPresentation{}, err
	}

	thread, err := readThreadPage(ctx, tx, activity, threadCursor)
	if err != nil {
		return crmcontracts.EmailPresentation{}, err
	}
	out.Thread = &thread

	// Reply and forward are writes, so they follow the send's grant; relink follows
	// the row's writability.
	mayWrite := auth.AllowsWrite(ctx, "activity", principal.ActionCreate)
	out.CanReply, out.CanForward = mayWrite, mayWrite
	out.CanRelink = access.CanChange && access.ChangeMode == crmcontracts.EmailAccessChangeModeMessageAudience
	if activity.CanShareInternal != nil {
		out.CanRelink = writable
	}
	return out, nil
}

// markOpenInvitation turns the summary into an invitation's when the reader
// may still answer it.
// markCalendarFacts says whether the mail is an open invitation and what it
// changed about its meeting, as the page read does for each row.
func markCalendarFacts(ctx context.Context, tx pgx.Tx, id ids.ActivityID, summary *crmcontracts.EmailSummary, asOf time.Time) error {
	if err := markOpenInvitation(ctx, tx, id, summary, asOf); err != nil {
		return err
	}
	return markCalendarChange(ctx, tx, id, summary)
}

func markOpenInvitation(ctx context.Context, tx pgx.Tx, id ids.ActivityID, summary *crmcontracts.EmailSummary, asOf time.Time) error {
	open, err := openInvitationsTx(ctx, tx, []ids.UUID{id.UUID}, readerOrNobody(ctx), asOf)
	if err != nil {
		return err
	}
	if found, ok := open[id.UUID]; ok {
		found.mark(summary)
	}
	return nil
}

// withheldPresentation is the whole answer for a caller who may know the
// message exists and may not read it: markers, and nothing said.
//
// Its own function because it shares nothing with the available path below —
// no parties, no files, no thread, no reason. Each of those describes the
// message, and an empty list is a different statement from a refused one only
// if the status says which.
func withheldPresentation(out crmcontracts.EmailPresentation, activity crmcontracts.Activity) crmcontracts.EmailPresentation {
	out.Summary = withheldSummary(activity)
	out.Access = withheldAccess()
	return out
}

// availableSummary is the row for a message this caller may read.
func availableSummary(
	activity crmcontracts.Activity,
	parties emailParties,
	attachments int,
	status crmcontracts.EmailAccessStatus,
) crmcontracts.EmailSummary {
	summary := crmcontracts.EmailSummary{
		ActivityId:        activity.Id,
		OccurredAt:        activity.OccurredAt,
		OccurredPrecision: activity.OccurredPrecision,
		DisplayStatus:     status,
		AttachmentCount:   attachments,
		Move:              crmcontracts.EmailSummaryMoveNone,
		Subject:           activity.Subject,
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
	summary.Counterparty, summary.CounterpartyContactId = rowCounterparty(activity.Direction, parties)
	if activity.EmailSummary != nil {
		summary.Move = activity.EmailSummary.Move
	}
	return summary
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
		        WHERE a.id = $1 AND a.archived_at IS NULL AND a.restricted_at IS NULL
                 AND a.captured_by LIKE '%:' || $2::text)`,
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
		SELECT at.id, at.filename, at.byte_size, at.content_type, at.bytes_withheld
		  FROM attachment at
		 WHERE at.entity_type = 'activity' AND at.entity_id = $1 AND at.archived_at IS NULL
		   AND `+arrivedWithMessage("at")+`
		 ORDER BY at.created_at, at.id`, id.UUID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []crmcontracts.EmailAttachmentSummary{}
	for rows.Next() {
		var a crmcontracts.EmailAttachmentSummary
		var raw ids.UUID
		var withheld bool
		if err := rows.Scan(&raw, &a.Filename, &a.ByteSize, &a.ContentType, &withheld); err != nil {
			return nil, err
		}
		a.BytesWithheld = &withheld
		a.Id = openapi_types.UUID(raw)
		out = append(out, a)
	}
	return out, rows.Err()
}

// withheldSummary is the row a caller outside the audience still sees: when it
// happened and that it happened, with the words removed.
func withheldSummary(activity crmcontracts.Activity) crmcontracts.EmailSummary {
	summary := crmcontracts.EmailSummary{
		ActivityId:        activity.Id,
		OccurredAt:        activity.OccurredAt,
		OccurredPrecision: activity.OccurredPrecision,
		DisplayStatus:     crmcontracts.EmailAccessStatusWithheld,
		AttachmentCount:   0,
		Move:              crmcontracts.EmailSummaryMoveNone,
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

// readThreadPage reads the rest of the conversation as summaries, newest
// first. Bounded and paged: a thread has no ceiling, and a drawer that fetched
// every message would make opening the newest one cost the whole history.
//
// It runs the same gated list every timeline runs. Narrowing to a thread_key
// upgrades that list to the CONTENT gate. A member the caller may not read is
// then absent rather than withheld. The drawer shows the conversation this
// reader is party to, not its true length. A thread-narrowed list always did
// this. So the member count is the reader's own and no one else's. A mail several mailboxes
// captured counts once, as on the timeline.
func readThreadPage(
	ctx context.Context,
	tx pgx.Tx,
	activity crmcontracts.Activity,
	cursor *string,
) (crmcontracts.EmailThreadPage, error) {
	empty := crmcontracts.EmailThreadPage{Members: []crmcontracts.EmailSummary{}}
	// A message the provider gave no conversation key is its own whole
	// conversation: an empty page rather than an absent one, so the drawer
	// renders one message instead of deciding what a missing thread means.
	if activity.ThreadKey == nil || *activity.ThreadKey == "" {
		return empty, nil
	}
	key := *activity.ThreadKey
	limit := maxThreadMembers
	members, page, err := ListActivitiesTx(ctx, tx, ListActivitiesInput{
		ThreadKey:          &key,
		Cursor:             cursor,
		Limit:              &limit,
		CollapseSeatCopies: true,
	})
	if err != nil {
		return empty, err
	}
	out := crmcontracts.EmailThreadPage{Members: []crmcontracts.EmailSummary{}}
	for _, member := range members {
		if member.Kind != crmcontracts.ActivityKindEmail || standsFor(member, activity.Id) {
			continue
		}
		if member.EmailSummary != nil {
			out.Members = append(out.Members, *member.EmailSummary)
		}
	}
	if page.NextCursor != "" {
		out.NextCursor = &page.NextCursor
	}
	return out, nil
}

// standsFor reports whether a collapsed row is another mailbox's copy of the
// opened message. The drawer already shows that message, so the copy is no
// second member of its thread.
func standsFor(member crmcontracts.Activity, opened openapi_types.UUID) bool {
	return member.CopyIds != nil && slices.Contains(*member.CopyIds, opened)
}
