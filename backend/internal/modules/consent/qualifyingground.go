// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

// What on the record implies that correspondence is lawful, when the subject
// has recorded no answer of their own.
//
// Split from verdict.go for the file-length ceiling, and the seam is the honest
// one: that file decides a VERDICT from a person's own answer and this one
// finds the IMPLIED ground beneath it. The two are asked in that order —
// correspondenceVerdict reads the recorded state first and only reaches here
// when there is none — so keeping them apart keeps the precedence visible.

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// latestQualifyingEvent finds the most recent thing on the record that makes
// correspondence lawful.
//
// It looks in two places, and the order is the point. A TYPED row wins: an
// in-person exchange or an inquiry is something a named human recorded, and it
// carries the note or the reference a dispute asks for. Failing that the
// captured timeline answers for itself — an inbound message from this person IS
// the qualifying event, which is what "deterministic and derivable from
// captured data" means (ADR-0098 D2).
//
// Deriving rather than requiring a written row is what keeps the rule honest on
// day one: every mailbox this product has ever captured already contains the
// evidence, and a model that only recognised events recorded after this build
// shipped would tell a rep they may not answer somebody who wrote to them last
// week.
//
// SINCE BOUNDS BOTH SOURCES, and it is the reply window
// (authorizewindows.go). An event older than it stops supporting an unprompted
// message: somebody who asked a question two years ago and never came back has
// not agreed to an ongoing relationship, and treating one inbound as permanent
// authority is what let unrelated cold outreach ride on a single old message.
//
// What this does NOT bound is a same-thread reply. That path never reaches
// here — resolveCategory answers CategoryReplyToInbound from the anchor before
// the legacy verdict is consulted — so a rep answering a months-old thread is
// unaffected. The window governs contact the subject did not prompt.
func latestQualifyingEvent(ctx context.Context, tx pgx.Tx, personID string, since time.Time) (QualifyingEvent, eventSource, error) {
	event, found, err := recordedQualifyingEvent(ctx, tx, personID, since)
	if err != nil {
		return QualifyingEvent{}, sourceNone, err
	}
	if found {
		return event, sourceRecorded, nil
	}
	event, found, err = inboundQualifyingEvent(ctx, tx, personID, since)
	if err != nil {
		return QualifyingEvent{}, sourceNone, err
	}
	if found {
		return event, sourceDerived, nil
	}
	// And a MEETING, last of the three because it is the newest arm rather than
	// the weakest — a meeting is stronger evidence than an inbound message. The
	// order costs nothing: each arm answers about a different record, so at most
	// one of them describes any given fact, and reaching this one means neither
	// a human nor the mail timeline had already answered.
	event, found, err = meetingQualifyingEvent(ctx, tx, personID, since)
	if err != nil {
		return QualifyingEvent{}, sourceNone, err
	}
	if found {
		return event, sourceDerived, nil
	}
	return QualifyingEvent{}, sourceNone, nil
}

// eventSource says where a qualifying event came from, and therefore whether
// the transmit path still owes it a stamp.
//
// One value rather than two adjacent bools: `found` and `derived` are the same
// type in neighbouring positions, so nothing but care stopped a future arm
// returning them the wrong way round — and returning them the wrong way round
// means either stamping a duplicate of a row that already exists, or relying on
// a basis that was never written down.
type eventSource int

const (
	// sourceNone: nothing on the record makes correspondence lawful.
	sourceNone eventSource = iota
	// sourceRecorded: a stored row already proves it, so there is nothing to stamp.
	sourceRecorded
	// sourceDerived: read off the timeline, and the transmit path must record it
	// before relying on it (Art 5(2)).
	sourceDerived
)

// RecordDerivedQualifyingEvent stamps a derived qualifying event onto the
// record, so what authorized a send is a fact somebody can look up rather than
// a computation this build happened to make.
//
// ADR-0098 D2 requires the flip be "stamped with which event and when", and
// Art 5(2) is why: a lawful basis nobody wrote down is an assertion, and the
// controller carries the burden of showing it. Deriving the event at read time
// answers the question correctly; it does not answer it accountably.
//
// Only the TRANSMIT path calls this. A preview authorizes nothing, and writing
// a legal fact because somebody opened a composer would record a basis for a
// message that was never sent.
//
// The insert is idempotent on the source record — the same inbound message
// re-derived on the next send must not stack a second row claiming a second
// event happened — and the guarantee is the database's unique index, not a
// check this function performs and a concurrent caller races past.
func RecordDerivedQualifyingEvent(ctx context.Context, tx pgx.Tx, personID string, event QualifyingEvent, capturedBy string) error {
	// ON CONFLICT, not NOT EXISTS: two concurrent sends to the same person both
	// pass a read-then-write check and both insert. The unique index on the
	// source record is what actually makes this idempotent.
	_, err := tx.Exec(ctx, `
		INSERT INTO consent_qualifying_event
			(person_id, kind, source_entity_type, source_entity_id,
			 occurred_at, source, captured_by)
		VALUES ($1, $2, $3, $4, $5, 'derived', $6)
		ON CONFLICT (person_id, source_entity_type, source_entity_id)
		  WHERE source_entity_id IS NOT NULL
		  DO NOTHING`,
		personID, event.Kind, event.SourceEntityType, event.SourceEntityID,
		event.OccurredAt, capturedBy)
	if err != nil {
		return fmt.Errorf("consent: stamp the qualifying event that allowed this send: %w", err)
	}
	return nil
}

// recordedQualifyingEvent reads a row a human or an integration wrote.
func recordedQualifyingEvent(ctx context.Context, tx pgx.Tx, personID string, since time.Time) (QualifyingEvent, bool, error) {
	var event QualifyingEvent
	var sourceType, sourceID, note *string
	err := tx.QueryRow(ctx, `
		SELECT kind, occurred_at, source_entity_type, source_entity_id, note
		FROM consent_qualifying_event
		WHERE person_id = $1 AND occurred_at >= $2
		ORDER BY occurred_at DESC
		LIMIT 1`, personID, since).Scan(&event.Kind, &event.OccurredAt, &sourceType, &sourceID, &note)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualifyingEvent{}, false, nil
	}
	if err != nil {
		return QualifyingEvent{}, false, fmt.Errorf("read the qualifying event: %w", err)
	}
	if sourceType != nil {
		event.SourceEntityType = *sourceType
	}
	if sourceID != nil {
		event.SourceEntityID = *sourceID
	}
	if note != nil {
		event.Note = *note
	}
	return event, true, nil
}

// inboundQualifyingEvent derives the event from the captured timeline: they
// WROTE to us, and the message itself is the proof.
//
// Authorship, not filing. The activity is reached through activity_link — the
// same table every person-scoped timeline read walks, so this cannot count a
// message the record does not show — but a link is a FILING and says only that
// the message belongs on this person's record. Being copied on a message
// somebody else wrote is not the subject initiating correspondence, and
// counting it would let anyone manufacture a lawful basis for writing to a
// third party by putting them in Cc. So the participant row has to name them as
// the author too, which is the same test authorizeevidence.go applies for the
// same reason (authorIsTheSubject).
//
// Both halves are load-bearing. Without the link this would count a message
// nobody filed under the person; without the authorship test it counts one they
// merely received. Capture files a message under every participant it resolves
// (capture/sinkmaillinks.go), so the link alone stopped meaning authorship the
// moment a cc'd contact could be filed under.
func inboundQualifyingEvent(ctx context.Context, tx pgx.Tx, personID string, since time.Time) (QualifyingEvent, bool, error) {
	var event QualifyingEvent
	var activityID string
	err := tx.QueryRow(ctx, `
		SELECT a.id, a.occurred_at
		FROM activity a
		JOIN activity_link l ON l.activity_id = a.id AND l.person_id = $1
		JOIN activity_participant p ON p.activity_id = a.id
		     AND p.role = 'from' AND p.person_id = $1::uuid
		WHERE a.direction = 'inbound' AND a.archived_at IS NULL
		  AND a.occurred_at >= $2
		ORDER BY a.occurred_at DESC
		LIMIT 1`, personID, since).Scan(&activityID, &event.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualifyingEvent{}, false, nil
	}
	if err != nil {
		return QualifyingEvent{}, false, fmt.Errorf("read the inbound qualifying message: %w", err)
	}
	event.Kind = "inbound_message"
	event.SourceEntityType = "activity"
	event.SourceEntityID = activityID
	return event, true, nil
}

// meetingQualifyingEvent derives the event from a meeting the subject was in.
//
// A meeting is stronger evidence than an inbound message, and the product could
// not see one. An email can be unsolicited — anybody may write to us — while a
// meeting means both sides put time in a calendar. So a partner we had invited,
// and were meeting next week, was refused as somebody who "has never written to
// you", and the invitation itself made it worse: the classifier read a machine
// generated calendar mail as transactional and judged the person noise on it.
//
// ATTENDANCE, from the participant rows, not the counterparty. A meeting names
// no counterparty at all — attendance is a LIST, so the calendar mapper leaves
// the field unset — which is exactly why the mail-shaped derivation above could
// never answer for one.
//
// FOUR BOUNDS, and every one of them is what stops this being a way to write
// your own permission slip. A qualifying event makes it LAWFUL to mail somebody,
// and everything below is caller-supplied unless it is bounded.
//
// A CONNECTOR must have captured it. `POST /activities` takes kind, occurred_at
// and links from the request body, and the log path stamps a participant row for
// every linked person (activities/participantlog.go) — so any seat that can see
// a contact could otherwise log a "meeting" naming them and mail them on the
// strength of it. A connector-captured meeting came from a calendar the mailbox
// owner actually holds. This is the same boundary capture's own noise sweep
// draws, for the same reason.
//
// And it must NAME NOBODY, which is the bound the connector clause cannot carry.
// A connector reports the kind per message and the extension ingress copies it
// off a third-party unit's record with no vocabulary check in front of it
// (compose/extingress.go), so a message a connector really did capture can wear
// this kind and still be mail. Mail is what the arm above weighs, on the
// authorship test this one deliberately does not apply — so without this clause
// a message somebody was merely copied on, relabelled, is a permission slip to
// write to everyone who was on it. Counterparty-less is the shape no caller
// forges by choosing a string: attendance is a list, and the calendar mapper is
// what leaves the field unset.
//
// The meeting must not have been DECLINED or abandoned. meeting_status carries
// `no_show` and `canceled`, and neither is a meeting that happened: an invitation
// somebody declined is the opposite of evidence they welcome contact. NULL is
// admitted — the calendar mappers record no acceptance state, so most captured
// meetings carry none, and refusing those would refuse the whole feature.
//
// And it must not be dated beyond the horizon. A future meeting is the case this
// arm exists for, but "future" has to mean the diary rather than the next
// century: the derived row is stamped and later read back without revalidating
// its source, so a meeting dated 2099 would authorize sending forever.
//
// No authorship test, unlike the inbound arm — that is a difference rather than
// an omission. That arm needs role 'from' because a filing link says only that a
// message belongs on somebody's record, and being copied on somebody else's mail
// initiates nothing. Attendance carries no such ambiguity: every attendee of a
// meeting a connector captured is a party to it.
func meetingQualifyingEvent(ctx context.Context, tx pgx.Tx, personID string, since time.Time) (QualifyingEvent, bool, error) {
	var event QualifyingEvent
	var activityID string
	err := tx.QueryRow(ctx, `
		SELECT a.id, a.occurred_at
		FROM activity a
		JOIN activity_participant p ON p.activity_id = a.id AND p.person_id = $1::uuid
		WHERE a.kind = 'meeting' AND a.archived_at IS NULL
		  AND a.counterparty_email IS NULL
		  AND a.captured_by LIKE 'connector:%'
		  AND (a.meeting_status IS NULL OR a.meeting_status NOT IN ('no_show', 'canceled'))
		  AND a.occurred_at >= $2
		  AND a.occurred_at <= now() + $3::interval
		ORDER BY a.occurred_at DESC
		LIMIT 1`, personID, since, meetingHorizonInterval).Scan(&activityID, &event.OccurredAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return QualifyingEvent{}, false, nil
	}
	if err != nil {
		return QualifyingEvent{}, false, fmt.Errorf("read the qualifying meeting: %w", err)
	}
	event.Kind = KindMeeting
	event.SourceEntityType = "activity"
	event.SourceEntityID = activityID
	return event, true, nil
}

// meetingHorizonInterval is how far into the diary a meeting still says a
// relationship is live.
//
// A quarter, because that is the outer edge of a real booking — a demo, a
// renewal review, a conference — and the derived row outlives its source: it is
// stamped once and read back afterwards without revalidating the activity, so a
// meeting dated far enough ahead would authorize sending indefinitely. The bound
// is what keeps "a meeting in the diary" from meaning "a date somebody typed".
const meetingHorizonInterval = "90 days"
