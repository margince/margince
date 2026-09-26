// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// fieldKind names the activity kind in the write shape's audit and outbox
// payloads (the one spelling of the payload key).
const (
	fieldKind    = "kind"
	fieldSubject = "subject"
)

// activityCapturedPayload builds the activity.captured event for the
// direct-log path (this package's only emit site of the event's two) — it
// never names a source_system, which is exclusive to the capture
// auto-create path (capture/sink.go's own local builder).
func activityCapturedPayload(kind, channelProvider string) crmcontracts.PublicEventActivityCaptured {
	p := crmcontracts.PublicEventActivityCaptured{Kind: kind}
	// Carried only when there is one, so the envelope's omitempty leaves it
	// absent rather than publishing an empty transport. A subscriber reads the
	// pair: since ADR-0107/A158 the kind alone no longer says what carried a
	// message, and without this field they could not tell one transport from
	// another at all.
	if channelProvider != "" {
		p.ChannelProvider = &channelProvider
	}
	return p
}

type LogActivityInput struct {
	// Internal creation policy; public activity input cannot set an audience.
	audienceMembers []AudienceMember
	// invitedAssignee admits an invited colleague as the task's assignee. Only
	// the HTTP create handler sets it, for an assignee the caller NAMED; every
	// automatic writer (deal check-up, lead SLA, email requests) leaves it
	// false and keeps handing work to active seats only.
	invitedAssignee bool
	Kind            string
	// ChannelProvider names the messaging transport that carried this activity —
	// a channel_provider row — and is empty for anything that did not travel on
	// one. Separate from Kind because they answer separate questions: what sort
	// of interaction happened, versus how it travelled (ADR-0107/A158).
	//
	// Non-empty exactly when Kind is KindMessage, which the database enforces in
	// both directions; a mismatch is a 422 from the CHECK, not a silent write.
	ChannelProvider string
	Subject         *string
	Body            *string
	OccurredAt      *time.Time
	Direction       *string
	// MeetingStatus is meaningful only for kind meeting; the mapping refuses
	// it on any other kind. The lead status ladder reads booked/held as
	// engagement, so a hand-logged meeting moves a lead the same as a synced
	// one.
	MeetingStatus *string
	DueAt         *time.Time
	RemindAt      *time.Time
	AssigneeID    *ids.UserID
	HostUserID    *ids.UserID
	// ClaimsHostSlot marks a meeting that HOLDS its host's hour, which is what
	// activity_meeting_no_overlap refuses a second of. Only BookMeeting sets
	// it: a booking door takes a slot on somebody's calendar, and taking one
	// twice is the fault the guard exists for. Logging a meeting records that
	// one happened and claims nothing — a rep writing up a morning of
	// back-to-back calls is describing their Tuesday, not double-booking it.
	//
	// There is no way to set it from the wire, and there must not be: a guard a
	// caller can opt out of is not one.
	ClaimsHostSlot bool
	SourceSystem   *string
	SourceID       *string
	// SourceActivityID is the activity this one was derived FROM — the meeting
	// whose transcript proposed a task. Nil on almost every activity.
	SourceActivityID  *ids.UUID
	RequestActivityID *ids.UUID
	// ThreadKey files this activity under a conversation. Empty stores NULL.
	// It is written at insert time or not at all: the (source_system,
	// source_id) upsert both capture and this path key on does nothing when
	// the row already exists, so neither leg can revise the other's value.
	ThreadKey string
	// CounterpartyEmail is the address this message was with, normalized —
	// the column capture's correspondence-positive gate (ADR-0072 §1) reads.
	// CounterpartyOutboundAttested says the workspace itself sent to that
	// address; it is affirmative intent toward them, and it is what spares
	// their later mail from suppression. Both obey the same write-once rule
	// as ThreadKey, for the same reason.
	CounterpartyEmail            string
	CounterpartyOutboundAttested bool
	// Raw is the source system's own representation of this activity, kept
	// verbatim. It is CONTENT: the audience projection withholds it from a
	// reader who may not read the subject and body, and both destructive
	// paths — retention and noise redaction — null it with the rest of the text.
	Raw *map[string]any
	// DurationSeconds is how long a meeting or call lasted; the mapping
	// refuses it on any other kind.
	DurationSeconds *int
	// The address headers an importer stated, normalized and each address in
	// one role only. Capture derives these from the message it holds; an
	// importer holds only what its source system kept, so it states them.
	EmailFrom string
	EmailTo   []string
	EmailCc   []string
	// RFCMessageID is this message's own RFC 5322 identity, brackets stripped —
	// what a later capture of the same message resolves against.
	RFCMessageID string
	// The calendar identity, which is a PAIR: a recurring series shares one UID
	// across every occurrence, so the UID names the series and the instance
	// names the occurrence within it.
	ICalUID      string
	ICalInstance string
	Links        []ActivityLinkInput
	Source       string
	// Origin says who caused this row to exist, and the recency clocks in the
	// schema read it: the two system origins are excluded from every
	// last_activity_at, because neither the system asking about a silent deal
	// nor the installation mailing somebody a notice it owes them is the other
	// side breaking their silence. Empty means OriginHuman.
	Origin string
}

// The origins an activity can have.
//
// Two of them are the system writing rather than a contact, and neither counts
// as the record being touched: OriginSystemRemediation marks work the product
// files about a record — a forecast-assurance review task — and OriginSystemNotice
// marks a message the installation owes somebody, such as the confirm-details
// link. A Go query that needs to exclude them calls auth.OriginIsEngagement
// rather than naming an origin itself; the last_activity_of_* functions carry
// the same clause in SQL.
//
// Held by: TestEveryRecencyReadingExcludesTheSystemOrigins (backend/gates/recencyorigins_test.go)
const (
	OriginHuman             = "human"
	OriginAgent             = "agent"
	OriginSystemRemediation = "system_remediation"
	OriginSystemNotice      = "system_notice"
)

// LogActivity writes the activity + links; the last_activity_at clocks
// (data-model §7) are maintained in the schema, not here. Idempotent on
// (source_system, source_id): replaying a capture returns the existing
// activity.
func (s *Store) LogActivity(ctx context.Context, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	var out crmcontracts.Activity
	created := true
	err := s.tx(ctx, func(tx pgx.Tx) error {
		var err error
		out, created, err = s.logActivityAndReadTranscript(ctx, tx, in)
		return err
	})
	return out, created, err
}

// LogActivityTx is LogActivity's transaction-accepting variant (the C5
// shared-tx shape): a caller that must commit an activity note atomically
// with a sibling module's own write (the extraction accept-write's deal
// update, compose/extractionaccept.go) drives it inside the ONE
// transaction it already opened, so a note failure rolls the sibling
// write back too, instead of letting LogActivity open (and commit) a
// second transaction of its own.
func (s *Store) LogActivityTx(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	if err := auth.Require(ctx, "activity", principal.ActionCreate); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	return s.logActivityAndReadTranscript(ctx, tx, in)
}

// logActivityAndReadTranscript is the ONE place the write and the reading are
// joined, so every door reaches both.
//
// It sits here rather than in each entry point because there are three of
// them: LogActivity opens its own transaction, LogActivityTx runs inside a
// caller's, and both are reached from the tool surface, from REST, and from
// the extension core. The first cut hooked only LogActivity, which meant
// POST /v1/activities and the extension's own logging stored a transcript and
// silently never read it — the exact defect this feature exists to close,
// reintroduced on two of its three doors.
func (s *Store) logActivityAndReadTranscript(
	ctx context.Context, tx pgx.Tx, in LogActivityInput,
) (crmcontracts.Activity, bool, error) {
	if in.RequestActivityID != nil {
		return s.takeEmailRequest(ctx, tx, in)
	}
	out, created, err := logActivityInTx(ctx, tx, in)
	if err != nil {
		return out, created, err
	}
	return out, created, s.readTranscriptOnLanding(ctx, tx, out, created)
}

// taskAssignee decides who a new task belongs to.
//
// A task somebody writes for themselves and does not assign is THEIRS. The
// column used to keep the NULL and the "my work" queue compensated by treating
// every unassigned task as the reader's own, which put each automation-created
// follow-up on every colleague's queue at once — the lead was owned, the task
// it minted was not, and "mine" quietly meant "mine plus everybody's".
//
// Owning it at the point of writing is the half of that fix which keeps the
// self-written task where its author expects it, so the queue can go back to
// meaning exactly what it says.
//
// Only for a HUMAN creating a TASK. A system principal writing on nobody's
// behalf leaves the column NULL, which is what routes automation work to the
// unassigned queue instead of to whoever the workflow ran as; and a caller who
// named an assignee is obeyed.
func taskAssignee(ctx context.Context, in LogActivityInput) *ids.UserID {
	if in.AssigneeID != nil || in.Kind != "task" {
		return in.AssigneeID
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return nil
	}
	owner := ids.From[ids.UserKind](actor.UserID)
	return &owner
}

// meetingHost is who HELD a meeting, which is not the same question as who
// typed it up.
//
// A meeting names the one place an activity says which of US was there, and it
// is what a rep's week is counted from. Left unset it falls to captured_by, so
// a colleague who minutes somebody else's meeting takes it into their own week
// — the case the attribution audit named, and the one the connector import
// already answers because a calendar knows whose it was.
//
// A caller may name somebody else: that IS the minuting case, and the host is a
// label rather than an authority — what the caller may read was decided before
// this field is filled in. Silence defaults to the acting human, because a
// meeting somebody logs with no host named is almost always their own; a system
// or agent principal has no week to count it into and leaves it null rather
// than inventing one.
func meetingHost(ctx context.Context, in LogActivityInput) *ids.UserID {
	if in.HostUserID != nil || in.Kind != KindMeeting {
		return in.HostUserID
	}
	actor, ok := principal.Actor(ctx)
	if !ok || actor.Type != principal.PrincipalHuman || actor.UserID == ids.Nil {
		return nil
	}
	host := ids.From[ids.UserKind](actor.UserID)
	return &host
}

// logActivityInTx is LogActivity's transactional body, shared by the
// store-opened (LogActivity) and caller-opened (LogActivityTx) entry
// points.
func logActivityInTx(ctx context.Context, tx pgx.Tx, in LogActivityInput) (crmcontracts.Activity, bool, error) {
	by, err := storekit.CapturedBy(ctx)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	occurredAt := time.Now().UTC()
	if in.OccurredAt != nil {
		occurredAt = in.OccurredAt.UTC()
	}
	assignee := taskAssignee(ctx, in)
	host := meetingHost(ctx, in)
	// Asked at the CREATE door too, not only at the patch. A task minted onto an
	// agent seat is one that never reaches a queue, and it used to be refused
	// only if somebody later tried to move it — which is after it has been sat
	// in nobody's list for however long it took to notice.
	//
	// Checked against the resolved assignee rather than the input, so the
	// self-assignment above is covered by the same question. An invited
	// colleague may be given a NEW task when the caller named them over HTTP
	// (invitedAssignee); automatic writers keep the active-only check.
	checkAssignee := ensureAssigneeCanHoldWork
	if in.invitedAssignee {
		checkAssignee = ensureNewTaskAssignee
	}
	if err := checkAssignee(ctx, tx, assignee); err != nil {
		return crmcontracts.Activity{}, false, err
	}

	replay, err := replayedActivity(ctx, tx, in)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	if replay != nil {
		moved, err := replayMovedTheMeeting(ctx, tx, *replay, in)
		return moved, false, err
	}
	if bound, found, err := recognizedMessage(ctx, tx, in); err != nil || found {
		return bound, false, err
	}

	id := ids.New[ids.ActivityKind]()
	origin := in.Origin
	if origin == "" {
		origin = OriginHuman
	}
	counterparty, err := counterpartyFor(ctx, tx, in)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	// Attested outbound is the one write that turns "does this workspace
	// correspond with them" from no to yes, and the verdict engine acts on that
	// answer inside its own transaction. The two serialize on one key; capture's
	// sink takes it for the same reason on the rows a connector files.
	if in.CounterpartyOutboundAttested && counterparty != "" {
		if err := storekit.LockWriteIdentity(ctx, tx, correspondence.LockEntity,
			correspondence.LockIdentity(counterparty)); err != nil {
			return crmcontracts.Activity{}, false, err
		}
	}
	_, err = tx.Exec(ctx,
		`INSERT INTO activity (id, kind, channel_provider, subject, body, occurred_at, direction, meeting_status,
		                       due_at, remind_at, assignee_id, host_user_id, claims_host_slot, source_system, source_id, source, captured_by,
		                       thread_key, counterparty_email, counterparty_outbound_attested, origin,
		                       source_activity_id, raw, duration_seconds)
		 VALUES ($1, $2, NULLIF($3, ''), $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, NULLIF($18, ''),
		         NULLIF($19, ''), $20, $21, $22, $23, $24)`,
		// NULLIF on channel_provider: the column FKs into channel_provider, and
		// '' names no provider, so anything without a transport stores NULL.
		id, in.Kind, in.ChannelProvider, in.Subject, in.Body, occurredAt, in.Direction, in.MeetingStatus,
		in.DueAt, in.RemindAt, assignee, host, in.ClaimsHostSlot, in.SourceSystem, in.SourceID, in.Source, by,
		in.ThreadKey, counterparty, in.CounterpartyOutboundAttested, origin,
		in.SourceActivityID, in.Raw, in.DurationSeconds)
	if err != nil {
		if storekit.IsUniqueViolation(err) {
			return crmcontracts.Activity{}, false, apperrors.ErrConflict
		}
		return crmcontracts.Activity{}, false, err
	}

	if err := writeActivitySatellites(ctx, tx, id, in, occurredAt, by); err != nil {
		return crmcontracts.Activity{}, false, err
	}
	out, err := readActivity(ctx, tx, id, storekit.LiveOnly)
	if err != nil {
		return crmcontracts.Activity{}, false, err
	}
	return out, true, nil
}

// writeActivitySatellites writes the rows that ride with a new activity and
// have no meaning without it: its first meeting transition, its links, who was
// in it, where it came from, and the first-touch stamp. All in the caller's
// transaction, because an activity that reached the timeline without them is a
// row nobody can read properly.
func writeActivitySatellites(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, in LogActivityInput,
	occurredAt time.Time, by string,
) error {
	// The first transition, where this capture named a status. A meeting
	// arrives `booked` far more often than not, and that booking is the fact
	// every "how many did we book this period" question counts.
	if err := recordMeetingTransition(ctx, tx, meetingTransition{
		ActivityID:     id,
		Status:         meetingStatusOrNone(in.MeetingStatus),
		ScheduledStart: &occurredAt,
		SourceSystem:   in.SourceSystem,
		SourceID:       in.SourceID,
	}); err != nil {
		return err
	}
	if err := insertActivityLinks(ctx, tx, id, in.Kind, in.Links); err != nil {
		return err
	}
	// Who was in it (ACT-DDL-3). After the links, because the counterparty is
	// whichever contact they name — and they have just been through the
	// row-scope gate, so nothing here needs to re-check them.
	if err := stampLoggedParticipants(ctx, tx, id, in.Kind, in.Direction, in.Links); err != nil {
		return err
	}
	if err := recordImportedProvenance(ctx, tx, id, in, by); err != nil {
		return err
	}
	return recordInitialActivity(ctx, tx, id, in)
}

// replayedActivity resolves the (source_system, source_id) idempotency
// key: replaying a capture returns the existing activity. The replay
// path returns a record, so it is a read and carries the read's row
// scope: replaying someone else's external key must not hand over their
// activity. Out of scope answers the same 409 the unique-index race
// does — the key is taken, the record is not disclosed.
// replayMovedTheMeeting applies the one thing a redelivery of the same natural
// key may legitimately change: how the meeting went.
//
// A replay is otherwise the same fact arriving twice and writes nothing — that
// is what makes capture idempotent, and it is why the subject a rep corrected
// and the body they cleaned up survive the provider sending its own version
// again. The status is different in kind: booked → held / no_show / canceled is
// a vocabulary that EXISTS to change over time, and nothing on this side of
// the connector can know it moved.
//
// Left untouched, the row went on rendering an upcoming meeting that had been
// cancelled, the call answered 200 saying so, and no activity.updated fired —
// so the lead ladder and everything else downstream never re-read it.
//
// Through updateActivityInTx rather than an UPDATE here, so the move takes the
// write lock, records the transition (once, because it changed), and carries
// the same bounded delta a human's PATCH does. A held row refuses this write
// like any other, which is the point of the hold — the capture reports the
// refusal rather than claiming a success it did not have.
func replayMovedTheMeeting(
	ctx context.Context, tx pgx.Tx, replay crmcontracts.Activity, in LogActivityInput,
) (crmcontracts.Activity, error) {
	if in.MeetingStatus == nil || *in.MeetingStatus == meetingStatusString(replay.MeetingStatus) {
		return replay, nil
	}
	return updateActivityInTx(ctx, tx, ids.From[ids.ActivityKind](ids.UUID(replay.Id)), UpdateActivityInput{
		MeetingStatus: in.MeetingStatus,
	})
}

func replayedActivity(ctx context.Context, tx pgx.Tx, in LogActivityInput) (*crmcontracts.Activity, error) {
	if in.SourceSystem == nil || in.SourceID == nil {
		return nil, nil
	}
	var existing ids.ActivityID
	err := tx.QueryRow(ctx,
		`SELECT id FROM activity
		   WHERE source_system = $1 AND source_id = $2`,
		*in.SourceSystem, *in.SourceID).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	// The row exists — the SELECT above just found it — so the only way
	// readActivity's own row-scope gate can answer ErrNotFound here is that
	// the key belongs to someone out of scope.
	out, err := readActivity(ctx, tx, existing, storekit.IncludeArchived)
	if errors.Is(err, apperrors.ErrNotFound) {
		return nil, apperrors.ErrConflict
	}
	if err != nil {
		return nil, err
	}
	return &out, nil
}
