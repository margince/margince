// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cg:deal-room-timeline consumer: what happened in a Deal Room becomes a
// note on its deal's timeline.
//
// Without it the Deal Room is a second, separate record of the same
// relationship. A rep reading the deal sees the mail and the meetings; the
// buyer's questions, their decisions on the documents and the releases sent to
// them live somewhere else entirely, and every reader of the timeline — the
// deal brief, the meeting brief, a human scrolling it — is missing the half of
// the conversation that happened in the room.
//
// It lives in compose because the call crosses two modules: dealrooms owns the
// events, activities owns the timeline, and a module never imports a sibling
// (ADR-0054 §3), so the edge is injected here.
//
// EVERYTHING IS READ FROM THE EVENT. A comment can be edited and a document
// retitled after the fact; the payload carries what was true at the moment
// being recorded, which is what a timeline entry means.
//
// Replay is handled by the activity writer, not here: the note is keyed on
// (source_system, source_id) = (deal_room, the event id), and LogActivity is
// idempotent on that pair, so a redelivered event returns the existing note
// instead of writing a second one. The events.Dedupe wrapper in front is a
// cache, never the guarantee — it marks AFTER the effect runs, so a crash in
// that window replays.

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgxpool"

	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/dealrooms"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/events"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// roomTimelineEntity is the outbox entity type this consumer reacts to. A room
// rides the deal family stream, and its entity is the room itself.
const roomTimelineEntity = "deal_room"

// roomTimelineSource marks these notes as this consumer's effect — greppable,
// and never mistakable for a hand-written note. It is also the source_system
// half of the idempotency key.
const roomTimelineSource = "deal_room"

// roomTimelineDealEntity is the activity_link target every note carries: the
// deal the room belongs to, which is the timeline it must appear on.
const roomTimelineDealEntity = "deal"

// DealRoomTimeline writes one note per Deal Room event onto the deal.
type DealRoomTimeline struct {
	pool       *pgxpool.Pool
	activities *activities.Store
	log        *slog.Logger
}

// NewDealRoomTimeline builds the consumer over the activities store.
func NewDealRoomTimeline(pool *pgxpool.Pool, acts *activities.Store, log *slog.Logger) *DealRoomTimeline {
	return &DealRoomTimeline{pool: pool, activities: acts, log: log}
}

// HandleEvent routes one envelope. An event this consumer does not care about
// answers nil, so the group keeps flowing rather than wedging on somebody
// else's traffic — which is most of what the deal stream carries.
func (w *DealRoomTimeline) HandleEvent(ctx context.Context, env events.Envelope) error {
	if env.Entity.ID == ids.Nil || env.Entity.Type != roomTimelineEntity {
		return nil
	}
	note, carried, err := roomNote(env)
	if err != nil || !carried {
		return err
	}
	ws, err := InstallationDB(w.pool).Workspace(ctx)
	if err != nil {
		return err
	}
	return w.write(w.noteContext(ctx, env, ws.UUID), env, note)
}

// roomNoteText is one timeline entry before it has a home: what the room event
// says, and which deal it says it about.
type roomNoteText struct {
	deal    ids.UUID
	subject string
	body    string
}

// roomNote turns one envelope into the note it deserves. The bool says whether
// this consumer carries the event at all, which is a separate answer from a
// decode failure: most of the deal stream is somebody else's traffic.
//
// The switch is the whole editorial decision. A comment, a decision and a
// release are things that happened between the two sides; opening, pausing and renaming a room are the seller's own housekeeping
// and would only crowd the timeline that has to stay readable.
func roomNote(env events.Envelope) (roomNoteText, bool, error) {
	switch env.Type {
	case dealrooms.EventCommentPosted:
		posted, err := dealrooms.DecodeCommentPosted(env.Payload)
		if err != nil {
			return roomNoteText{}, false, err
		}
		return commentNote(posted), true, nil
	case dealrooms.EventDecisionRecorded:
		decided, err := dealrooms.DecodeDecisionRecorded(env.Payload)
		if err != nil {
			return roomNoteText{}, false, err
		}
		return decisionNote(decided), true, nil
	case dealrooms.EventPublished:
		released, err := dealrooms.DecodePublished(env.Payload)
		if err != nil {
			return roomNoteText{}, false, err
		}
		return publishNote(released), true, nil
	}
	return roomNoteText{}, false, nil
}

// commentNote says who spoke and whether they started something. The body is
// deliberately not the comment text: a Deal Room comment is written to the
// buyer, and copying it onto the deal would duplicate a record that already
// lives in the room and can be edited there afterwards.
func commentNote(posted dealrooms.CommentPosted) roomNoteText {
	side := "The seller"
	if posted.Side == roomSideBuyer {
		side = "The buyer"
	}
	subject := side + " replied in the Deal Room"
	if posted.OpensThread {
		subject = side + " started a thread in the Deal Room"
	}
	body := "About the room as a whole."
	if posted.DocumentID != nil {
		body = "About a document in the room."
	}
	return roomNoteText{deal: posted.DealID, subject: subject, body: body}
}

// roomSideBuyer is the payload's spelling of the outside party.
const roomSideBuyer = "buyer"

// decisionNote records a buyer's verdict on a document version.
func decisionNote(decided dealrooms.DecisionRecorded) roomNoteText {
	subject := "The buyer asked for changes to a Deal Room document"
	if decided.Kind == roomDecisionConfirm {
		subject = "The buyer confirmed a Deal Room document"
	}
	return roomNoteText{
		deal:    decided.DealID,
		subject: subject,
		body:    "A working decision inside the room, not a signature.",
	}
}

// roomDecisionConfirm is the payload's spelling of an accepted version.
const roomDecisionConfirm = "confirm_version"

// publishNote records what the seller released to the buyer.
func publishNote(released dealrooms.Published) roomNoteText {
	return roomNoteText{
		deal:    released.DealID,
		subject: fmt.Sprintf("Deal Room release %d published", released.ReleaseNo),
		body:    "The buyer can now read the room as it stood at this moment.",
	}
}

// write logs the note against the deal, keyed on the event so a replay returns
// the note that is already there. The deal is known non-zero: the decoders
// refuse a payload that names none (dealrooms.ErrEventNamesNoDeal), so a note
// can never be attached to nothing.
func (w *DealRoomTimeline) write(ctx context.Context, env events.Envelope, note roomNoteText) error {
	system, id := roomTimelineSource, env.EventID.String()
	_, _, err := w.activities.LogActivity(ctx, activities.LogActivityInput{
		Kind:         string(crmcontracts.ActivityKindNote),
		Subject:      &note.subject,
		Body:         &note.body,
		OccurredAt:   &env.OccurredAt,
		SourceSystem: &system,
		SourceID:     &id,
		Links: []activities.ActivityLinkInput{
			{EntityType: roomTimelineDealEntity, EntityID: note.deal},
		},
		Source: roomTimelineSource,
	})
	if err != nil {
		return fmt.Errorf("deal room timeline: writing the note for %s: %w", env.Type, err)
	}
	return nil
}

// noteContext binds the workspace, the trace and the system actor this consumer
// runs as. A subscriber carries none of them: without this the governed write
// would fail for want of an actor, and the note would carry no correlation back
// to what happened in the room.
func (w *DealRoomTimeline) noteContext(ctx context.Context, env events.Envelope, ws ids.UUID) context.Context {
	ctx = principal.WithWorkspaceID(ctx, ws)
	ctx = principal.WithCorrelationID(ctx, env.Trace.CorrelationID)
	// The note's own event names what caused it, so a reader can walk from the
	// timeline entry back to the comment or release that produced it.
	ctx = principal.WithCausationEvent(ctx, env.EventID)
	return principal.WithActor(ctx, principal.Principal{
		Type: principal.PrincipalSystem, ID: "system:deal_room_timeline",
		Permissions: principal.Permissions{RowScope: principal.RowScopeAll},
	})
}
