// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// Filing a captured message under the contacts who were on it, beyond the one
// the ladder judged.
//
// Mail reaches its records through the COUNTERPARTY: one address, one ensure,
// one link. That is right for the party the message is with, and it is the
// whole filing a message ever got — so a contact who was merely cc'd was
// stamped as a participant and filed nowhere. The message never reached their
// timeline, and nothing that reads through activity_link could see it.
//
// It cost more than a missing row. Ordinary business correspondence is lawful
// under Art 6(1)(f) when the person has written to us, and consent reads that
// from an INBOUND activity linked to them (consent.inboundQualifyingEvent). A
// cc'd contact's inbound mail therefore qualified nobody: the send gate refused
// mail to a person whose replies were sitting in the workspace, unlinked. The
// two repair sweeps could not reach it either — one keys on counterparty_email,
// which names somebody else on these messages, and the other is gated on
// meetings.
//
// So this is the meeting arm's rule (sinkmeetinglinks.go) applied to the
// transports that carry a counterparty, and it is deliberately the SAME rule:
// people already resolved on the participant rows, nobody created, the merge
// redirect settled, the visibility probe made, the 25-link ceiling respected.
//
// TWO DIFFERENCES, both load-bearing:
//
// A SUPPRESSED message is filed under nobody. The ladder judged its sender
// infrastructure or noise, and a newsletter that happens to name a colleague is
// not correspondence with them. Filing it would put a bulk sender's mail on a
// contact's timeline and hand it a qualifying event the judgement just refused.
//
// The count is NOT reported to the audience limiter — see the caller for what
// that would cost. These links say where a message is FILED; they say nothing
// about who may read it.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// linkResolvedMailParticipants files a captured message under every participant
// the workspace already has a live person for.
//
// It answers nothing, and that is the point. The meeting arm returns a count
// because the audience limiter reads it: a meeting filed under an attendee has
// a record standing behind it, so it is not the link-less mail the limiter
// holds. A cc'd contact is not that claim — the message may still be filed
// under nobody the ladder judged — so this arm must not feed that decision.
// Returning a count is how a later reader wires it in by accident, so there is
// none to wire.
//
// Held by: TestASuppressedMessageIsFiledUnderNobodyItCopies and
// TestFilingAConfidentialMessageDoesNotWidenWhoMayReadIt
// (backend/internal/compose/integration/capture/capture_participantlink_integration_test.go)
func (s *Sink) linkResolvedMailParticipants(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, existing int,
) error {
	people, err := mailParticipantsWithPeople(ctx, tx, activityID)
	if err != nil {
		return err
	}
	// The budget is what the row can still take rather than a fresh 25: the
	// ceiling is on the activity, and the counterparty's own link is already on
	// it by the time this runs.
	budget := maxDerivedMeetingLinks - existing
	written := 0
	for _, person := range people {
		if written >= budget {
			break
		}
		// A connector may not plant a link to a record its granting human could
		// not see (H1). Not-found and denied are skipped rather than failed: the
		// message happened, the participant row already records that this person
		// was on it, and refusing the capture over a filing decision would throw
		// away a message we read successfully.
		if err := auth.EnsureLinkTarget(ctx, tx, "person", person.UUID); err != nil {
			if errors.Is(err, apperrors.ErrNotFound) || errors.Is(err, apperrors.ErrPermissionDenied) {
				continue
			}
			return fmt.Errorf("capture: mail link target %s: %w", person, err)
		}
		tag, err := tx.Exec(ctx, `
			INSERT INTO activity_link (activity_id, entity_type, person_id)
			VALUES ($1, 'person', $2)
			ON CONFLICT DO NOTHING`, activityID, person)
		if err != nil {
			return fmt.Errorf("capture: filing a message under its participant: %w", err)
		}
		written += int(tag.RowsAffected())
	}
	return nil
}

// mailParticipantsWithPeople answers the people this message's participant rows
// resolved to, ordered by id.
//
// No role ordering, unlike the meeting arm: a meeting's organizer is the most
// useful filing when the cap cuts the list, and mail has no equivalent — the
// sender is already filed by the ladder, and a To is not worth more than a Cc.
// The order is still deterministic so a replay files the same message the same
// way.
//
// The id is settled against a merge here, for the reason every writer of
// activity_link settles it: no reader walks merged_into_id, so a link written
// to a retired person leaves the message on a record nobody opens. The redirect
// is followed BEFORE liveness is judged, because a merge archives the source and
// points it at the survivor in one write — testing the source's own archived_at
// would drop a participant whose record simply moved.
func mailParticipantsWithPeople(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID,
) ([]ids.PersonID, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT survivor.id AS person_id
		  FROM activity_participant ap
		  JOIN person p ON p.id = ap.person_id
		  JOIN person survivor ON survivor.id = coalesce(p.merged_into_id, p.id)
		 WHERE ap.activity_id = $1 AND ap.person_id IS NOT NULL
		   AND survivor.archived_at IS NULL
		 ORDER BY person_id`, activityID)
	if err != nil {
		return nil, fmt.Errorf("capture: reading a message's resolved participants: %w", err)
	}
	defer rows.Close()
	var out []ids.PersonID
	for rows.Next() {
		var person ids.PersonID
		if err := rows.Scan(&person); err != nil {
			return nil, fmt.Errorf("capture: reading a message's resolved participants: %w", err)
		}
		out = append(out, person)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading a message's resolved participants: %w", err)
	}
	return out, nil
}
