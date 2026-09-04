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
