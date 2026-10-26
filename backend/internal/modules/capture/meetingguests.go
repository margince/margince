// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A later pull of an event still owes its meeting every guest the calendar lists.
// So does an event matched onto a meeting another door filed, such as an import.

import (
	"context"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// LaterGuests is what RecordLaterMeetingGuestsTx added to one meeting.
type LaterGuests struct {
	Participants int
	Links        int
}

// Changed reports whether the meeting gained anybody.
func (g LaterGuests) Changed() bool { return g.Participants > 0 || g.Links > 0 }

// RecordLaterMeetingGuestsTx adds the calendar's parties an existing meeting does
// not name yet, files it under those who are contacts, and audits the change.
//
// A known address is skipped: the cohort repair promotes that row, or a person
// relinked it, and a second row would undo either.
func RecordLaterMeetingGuestsTx(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID,
	parties connector.Parties, partyListIsAttested bool, evidence map[string]any,
) (LaterGuests, error) {
	if !partyListIsAttested || (len(parties.Participants) == 0 && len(parties.Withheld) == 0) {
		return LaterGuests{}, nil
	}
	before, err := countMeetingRows(ctx, tx, `SELECT count(*) FROM activity_participant WHERE activity_id = $1`, activityID)
	if err != nil {
		return LaterGuests{}, err
	}
	fresh, err := partiesNotOnMeeting(ctx, tx, activityID, parties.Participants)
	if err != nil {
		return LaterGuests{}, err
	}
	if err := StampFurtherParticipants(ctx, tx, activityID, meetingKind, "", true, fresh); err != nil {
		return LaterGuests{}, err
	}
	if err := StampSeatsPastTheCap(ctx, tx, activityID, meetingKind, true, parties.Withheld); err != nil {
		return LaterGuests{}, err
	}
	after, err := countMeetingRows(ctx, tx, `SELECT count(*) FROM activity_participant WHERE activity_id = $1`, activityID)
	if err != nil || after == before {
		return LaterGuests{}, err
	}
	links, err := fileMeetingUnderGuests(ctx, tx, activityID, seat)
	if err != nil {
		return LaterGuests{}, err
	}
	added := LaterGuests{Participants: after - before, Links: links}
	// Counts only: the addresses are on the rows, and an audit image outlives an erasure of them.
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", activityID.UUID,
		map[string]any{"guests_added": added.Participants, "guest_links_added": added.Links}, evidence)
	if err != nil {
		return LaterGuests{}, fmt.Errorf("capture: auditing the guests added to a meeting: %w", err)
	}
	// The interaction graph refolds the meeting's participants on this event.
	return added, storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventActivityUpdated{})
}

// partiesNotOnMeeting keeps the parties no row of the meeting holds by address.
func partiesNotOnMeeting(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, parties []connector.MessageParticipant,
) ([]connector.MessageParticipant, error) {
	rows, err := tx.Query(ctx, `
		SELECT DISTINCT address FROM activity_participant
		 WHERE activity_id = $1 AND address IS NOT NULL`, activityID)
	if err != nil {
		return nil, fmt.Errorf("capture: reading who a meeting already names: %w", err)
	}
	addresses, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("capture: reading who a meeting already names: %w", err)
	}
	onMeeting := make(map[string]bool, len(addresses))
	for _, address := range addresses {
		onMeeting[address] = true
	}
	var fresh []connector.MessageParticipant
	for _, p := range parties {
		if !onMeeting[strings.ToLower(strings.TrimSpace(p.Email))] {
			fresh = append(fresh, p)
		}
	}
	return fresh, nil
}

// fileMeetingUnderGuests files an existing meeting within its remaining link ceiling.
func fileMeetingUnderGuests(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID) (int, error) {
	existing, err := countMeetingRows(ctx, tx, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, activityID)
	if err != nil {
		return 0, err
	}
	private, err := storedMessageIsPrivateTx(ctx, tx, activityID, seat)
	if err != nil {
		return 0, err
	}
	return fileUnderParticipants(tx, activityID, participantFiling{
		decider: ctx,
		budget:  max(maxDerivedMeetingLinks-existing-projectLinkReservation, 0),
		private: private,
		write:   insertParticipantLink(ctx, tx, activityID),
	})
}

func countMeetingRows(ctx context.Context, tx pgx.Tx, query string, activityID ids.ActivityID) (int, error) {
	var n int
	if err := tx.QueryRow(ctx, query, activityID).Scan(&n); err != nil {
		return 0, fmt.Errorf("capture: counting what a meeting holds: %w", err)
	}
	return n, nil
}

// stampLaterGuests is RecordLaterMeetingGuestsTx for a calendar replay.
func (s *Sink) stampLaterGuests(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord) error {
	if fields, ok := rec.Fields.(ActivityFields); !ok || fields.Kind != meetingKind {
		return nil
	}
	parties := connector.Parties{Participants: rec.Participants, Withheld: rec.WithheldParties}
	added, err := RecordLaterMeetingGuestsTx(ctx, tx, id, actorUserID(ctx), parties, ParticipantListAttested(rec), nil)
	if err != nil || !added.Changed() || s.nameParticipants == nil || !namesSomebody(rec.Participants) {
		return err
	}
	return s.nameParticipants(ctx, tx, id)
}
