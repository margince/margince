// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A meeting's guests are the rosters of the calendars that list it. Each row a
// roster writes names that calendar's seat in roster_seat. The next sync of the
// same calendar adds who joined and retires who left. Rows a member, an
// import, mail or another calendar wrote carry no such seat and stay.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/authz"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// manualAudienceReason is activities.ReasonManual: a member set the audience.
// Spelled here because a module never imports a sibling.
const manualAudienceReason = "manual"

// MeetingRoster is the guest list one calendar holds for a meeting on file.
type MeetingRoster struct {
	// Seat is the colleague whose calendar listed these parties.
	Seat    ids.UUID
	Parties connector.Parties
	// Complete says the provider listed every guest. Only a complete roster may
	// retire a row; a partial copy or an older stored one only adds.
	Complete bool
	// Current says this is the calendar's copy as of now, so its answers replace
	// the stored ones. A stored original only fills an answer no row holds.
	Current bool
}

// rosterStamp says which calendar roster writes the rows, if any, and whether a
// listed colleague may be bound to their seat. A declined party never is.
type rosterStamp struct {
	seat      *ids.UUID
	bindSeats bool
}

// LaterGuests is what SettleMeetingRosterTx changed on one meeting. LeftOut is
// how many outside guests past the cap the meeting does not name.
type LaterGuests struct {
	Added, Retired, Links, Answered, LeftOut int
	leftOutMoved                             bool
}

// Changed reports whether the meeting's guests moved.
func (g LaterGuests) Changed() bool {
	return g.Added > 0 || g.Retired > 0 || g.Links > 0 || g.Answered > 0 || g.leftOutMoved
}

// stampBirthParties writes a new activity's further parties. A calendar's
// rows carry the seat whose roster listed them, so a later sync can settle them.
func stampBirthParties(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord, fields ActivityFields) error {
	attested := ParticipantListAttested(rec)
	var roster *ids.UUID
	if seat := actorUserID(ctx); fields.Kind == meetingKind && attested && seat != ids.Nil {
		roster = &seat
	}
	if _, err := stampParties(ctx, tx, id, fields.Kind, fields.ChannelProvider, attested, rec.Participants,
		rosterStamp{seat: roster, bindSeats: true}); err != nil {
		return err
	}
	if _, err := stampSeatsPastCap(ctx, tx, id, fields.Kind, attested, rec.WithheldParties, roster); err != nil {
		return err
	}
	crowd, err := stampGuestsPastCap(ctx, tx, id, fields.Kind, attested, rec.WithheldParties, roster)
	if err != nil || crowd.leftOut == 0 {
		return err
	}
	_, err = recordGuestsLeftOut(ctx, tx, id, crowd.leftOut)
	return err
}

// SettleMeetingRosterTx brings one calendar's rows on a live meeting in line
// with its roster, files the meeting under new contacts and audits the change.
// decider is the seat's own authority, which judges what a link may name.
//
// A declined colleague, or one added after a member narrowed the audience,
// is recorded by address alone: shown on the meeting, granted nothing.
func SettleMeetingRosterTx(
	ctx, decider context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, evidence map[string]any,
) (LaterGuests, error) {
	listed := append(append([]connector.MessageParticipant{}, roster.Parties.Participants...), roster.Parties.Withheld...)
	if roster.Seat == ids.Nil || len(listed) == 0 {
		return LaterGuests{}, nil
	}
	if _, err := storekit.LockRow(ctx, tx, "activity", activityID.UUID, storekit.LiveOnly); err != nil {
		if errors.Is(err, apperrors.ErrNotFound) {
			return LaterGuests{}, nil
		}
		return LaterGuests{}, fmt.Errorf("capture: locking a meeting to settle its guests: %w", err)
	}
	narrowed, open, err := audienceNarrowedByAMember(ctx, tx, activityID)
	if err != nil || !open {
		return LaterGuests{}, err
	}
	settled, resolvable, err := settleRosterRows(ctx, tx, activityID, roster, listed, !narrowed)
	if err != nil {
		return LaterGuests{}, err
	}
	if resolvable > 0 {
		if settled.Links, err = fileMeetingUnderGuests(decider, tx, activityID, roster.Seat); err != nil {
			return LaterGuests{}, err
		}
	}
	if !settled.Changed() {
		return settled, nil
	}
	// Counts only: an address in an audit image would outlive an erasure of the row.
	auditID, err := storekit.AuditEventWithEvidence(ctx, tx, "update", "activity", activityID.UUID, map[string]any{
		"guests_added": settled.Added, "guests_retired": settled.Retired, "guest_links_added": settled.Links,
		"guest_answers_changed": settled.Answered, "guests_left_out": settled.LeftOut,
	}, evidence)
	if err != nil {
		return LaterGuests{}, fmt.Errorf("capture: auditing the guests settled on a meeting: %w", err)
	}
	// The interaction graph refolds the meeting's participants on this event.
	return settled, storekit.EmitEvent(ctx, tx, auditID, activityID.UUID, crmcontracts.PublicEventActivityUpdated{})
}

// settleRosterRows retires who left, adds who joined, names a crowd's outside
// guests and records each answer. resolvable counts the added rows a link may follow.
func settleRosterRows(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, listed []connector.MessageParticipant, bindSeats bool,
) (LaterGuests, int, error) {
	retired, err := retireOffTheRoster(ctx, tx, activityID, roster, listed)
	if err != nil {
		return LaterGuests{}, 0, err
	}
	resolvable, err := addFromTheRoster(ctx, tx, activityID, roster, bindSeats)
	if err != nil {
		return LaterGuests{}, 0, err
	}
	seat := roster.Seat
	crowd, err := stampGuestsPastCap(ctx, tx, activityID, meetingKind, true, roster.Parties.Withheld, &seat)
	if err != nil {
		return LaterGuests{}, 0, err
	}
	answered, err := settleAnswers(ctx, tx, activityID, roster, listed)
	if err != nil {
		return LaterGuests{}, 0, err
	}
	moved, err := recordGuestsLeftOut(ctx, tx, activityID, crowd.leftOut)
	if err != nil {
		return LaterGuests{}, 0, err
	}
	return LaterGuests{
		Added: resolvable + crowd.written, Retired: retired, Answered: answered, LeftOut: crowd.leftOut, leftOutMoved: moved,
	}, resolvable, nil
}

// audienceNarrowedByAMember answers whether a member narrowed the meeting's
// audience, and open is false for a restricted meeting, which takes no guests.
func audienceNarrowedByAMember(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID) (narrowed, open bool, err error) {
	err = tx.QueryRow(ctx, `
		SELECT audience <> 'workspace' AND audience_reason IS NOT DISTINCT FROM $2
		  FROM activity WHERE id = $1 AND restricted_at IS NULL`, activityID, manualAudienceReason).Scan(&narrowed)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, false, nil
	}
	if err != nil {
		return false, false, fmt.Errorf("capture: reading who narrowed a meeting's audience: %w", err)
	}
	return narrowed, true, nil
}

// retireOffTheRoster removes this calendar's rows the roster no longer grants:
// a colleague's seat once they declined, and anybody a complete roster dropped.
// The syncing seat's own row and the host's are never retired.
func retireOffTheRoster(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, listed []connector.MessageParticipant,
) (int, error) {
	addresses := make([]string, 0, len(listed))
	declined := make([]string, 0, len(listed))
	for _, p := range listed {
		address := strings.ToLower(strings.TrimSpace(p.Email))
		addresses = append(addresses, address)
		if p.Declined() {
			declined = append(declined, address)
		}
	}
	tag, err := tx.Exec(ctx, `
		DELETE FROM activity_participant ap
		 USING activity a
		 WHERE a.id = ap.activity_id AND ap.activity_id = $1 AND ap.roster_seat = $2
		   AND ap.address IS NOT NULL
		   AND ap.user_id IS DISTINCT FROM $2 AND ap.user_id IS DISTINCT FROM a.host_user_id
		   AND ((ap.user_id IS NOT NULL AND ap.address = ANY($3)) OR ($4 AND NOT ap.address = ANY($5)))`,
		activityID, roster.Seat, declined, roster.Complete, addresses)
	if err != nil {
		return 0, fmt.Errorf("capture: retiring the guests a calendar no longer lists: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// addFromTheRoster writes the listed parties the meeting does not hold yet,
// within the party cap, and answers how many guests that added.
//
// A known address is skipped: the cohort repair promotes that row, or a member
// relinked it. This calendar's own address-only row is offered again while
// seats may be bound, so a guest who stopped declining gets their seat back.
func addFromTheRoster(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, bindSeats bool) (int, error) {
	held, err := heldAddresses(ctx, tx, activityID, roster.Seat)
	if err != nil {
		return 0, err
	}
	var guests int
	if err := tx.QueryRow(ctx, `
		SELECT count(*) FROM activity_participant WHERE activity_id = $1 AND role IN ($2, $3) AND NOT past_party_cap`,
		activityID, connector.ParticipantRoleOrganizer, connector.ParticipantRoleAttendee).Scan(&guests); err != nil {
		return 0, fmt.Errorf("capture: counting a meeting's guests: %w", err)
	}
	budget := max(connector.MaxParticipants-guests, 0)
	// A new address spends the cap; a row offered again for its seat replaces one.
	offer := func(parties []connector.MessageParticipant) []connector.MessageParticipant {
		var out []connector.MessageParticipant
		for _, p := range parties {
			bare, known := held[strings.ToLower(strings.TrimSpace(p.Email))]
			switch {
			case !known && budget > 0:
				budget--
				out = append(out, p)
			case known && bare && bindSeats && !p.Declined():
				out = append(out, p)
			}
		}
		return out
	}
	seat := roster.Seat
	added, err := stampParties(ctx, tx, activityID, meetingKind, "", true, offer(roster.Parties.Participants),
		rosterStamp{seat: &seat, bindSeats: bindSeats})
	if err != nil {
		return 0, err
	}
	if bindSeats {
		seats, err := stampSeatsPastCap(ctx, tx, activityID, meetingKind, true, offer(roster.Parties.Withheld), &seat)
		if err != nil {
			return 0, err
		}
		added += seats
	}
	superseded, err := retireSupersededRosterRows(ctx, tx, activityID, roster.Seat)
	return max(added-superseded, 0), err
}

// heldAddresses answers each address the meeting holds, and whether its only
// rows are this calendar's own address-only ones.
func heldAddresses(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID) (map[string]bool, error) {
	rows, err := tx.Query(ctx, `
		SELECT address,
		       count(*) = count(CASE WHEN roster_seat IS NOT DISTINCT FROM $2 AND user_id IS NULL AND contact_id IS NULL THEN 1 END)
		  FROM activity_participant WHERE activity_id = $1 AND address IS NOT NULL
		 GROUP BY address`, activityID, seat)
	if err != nil {
		return nil, fmt.Errorf("capture: reading who a meeting already names: %w", err)
	}
	defer rows.Close()
	held := map[string]bool{}
	for rows.Next() {
		var address string
		var bare bool
		if err := rows.Scan(&address, &bare); err != nil {
			return nil, fmt.Errorf("capture: reading who a meeting already names: %w", err)
		}
		held[address] = bare
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("capture: reading who a meeting already names: %w", err)
	}
	return held, nil
}

// retireSupersededRosterRows deletes this calendar's address-only rows that a
// resolved row for the same address and role now stands beside.
func retireSupersededRosterRows(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID) (int, error) {
	tag, err := tx.Exec(ctx, `
		DELETE FROM activity_participant bare
		 WHERE bare.activity_id = $1 AND bare.roster_seat = $2
		   AND bare.user_id IS NULL AND bare.contact_id IS NULL AND bare.address IS NOT NULL
		   AND EXISTS (SELECT 1 FROM activity_participant resolved
		                WHERE resolved.activity_id = bare.activity_id AND resolved.role = bare.role
		                  AND resolved.address = bare.address
		                  AND (resolved.user_id IS NOT NULL OR resolved.contact_id IS NOT NULL))`,
		activityID, seat)
	if err != nil {
		return 0, fmt.Errorf("capture: retiring the guest rows a resolved one replaced: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// fileMeetingUnderGuests files an existing meeting within its remaining link
// ceiling, under the records decider may see.
func fileMeetingUnderGuests(decider context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID) (int, error) {
	var existing int
	if err := tx.QueryRow(decider, `SELECT count(*) FROM activity_link WHERE activity_id = $1`, activityID).Scan(&existing); err != nil {
		return 0, fmt.Errorf("capture: counting a meeting's links: %w", err)
	}
	private, err := storedMessageIsPrivateTx(decider, tx, activityID, seat)
	if err != nil {
		return 0, err
	}
	return fileUnderParticipants(tx, activityID, participantFiling{
		decider: decider,
		budget:  max(maxDerivedMeetingLinks-existing-projectLinkReservation, 0),
		private: private,
		write:   insertParticipantLink(decider, tx, activityID),
	})
}

// stampLaterGuests settles a calendar replay's roster on the meeting it reached.
func (s *Sink) stampLaterGuests(ctx context.Context, tx pgx.Tx, id ids.ActivityID, rec connector.NormalizedRecord) error {
	if fields, ok := rec.Fields.(ActivityFields); !ok || fields.Kind != meetingKind || !ParticipantListAttested(rec) {
		return nil
	}
	roster := MeetingRoster{
		Seat:     actorUserID(ctx),
		Parties:  connector.Parties{Participants: rec.Participants, Withheld: rec.WithheldParties},
		Complete: !rec.PartiesIncomplete,
		Current:  true,
	}
	settled, err := SettleMeetingRosterTx(ctx, ctx, tx, id, roster, nil)
	if err != nil || settled.Added == 0 || s.nameParticipants == nil || !namesSomebody(rec.Participants) {
		return err
	}
	return s.nameParticipants(ctx, tx, id)
}

// SettleStoredMeetingRosterTx adds a stored event's guests to the meeting it
// matched, as the seat whose calendar stored it and past the gates live capture
// runs. A stored original is the event's first version, so it only adds.
func SettleStoredMeetingRosterTx(
	ctx context.Context, tx pgx.Tx, authority authz.Resolver, connectorName string,
	activityID ids.ActivityID, seat ids.UUID, rec connector.NormalizedRecord, evidence map[string]any,
) (LaterGuests, error) {
	decider, err := connectorActing(ctx, authority, connectorName, ids.From[ids.UserKind](seat))
	if errors.Is(err, apperrors.ErrNotFound) {
		return LaterGuests{}, nil
	}
	if err != nil {
		return LaterGuests{}, err
	}
	internal, err := internalOnlyTx(decider, tx, rec)
	if err != nil || internal {
		return LaterGuests{}, err
	}
	excluded, err := excludedTx(decider, tx, rec)
	if err != nil || excluded != "" {
		return LaterGuests{}, err
	}
	roster := MeetingRoster{Seat: seat, Parties: connector.Parties{Participants: rec.Participants, Withheld: rec.WithheldParties}}
	return SettleMeetingRosterTx(ctx, decider, tx, activityID, roster, evidence)
}
