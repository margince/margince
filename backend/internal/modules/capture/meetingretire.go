// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/correspondence"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// calendarProvidersArg binds calendarProviderNames for anotherLiveRosterSQL.
const calendarProvidersArg = "calendar_providers"

// calendarLiveSQL holds when seat holds a live calendar connection. providers
// is the placeholder that binds calendarProviderNames.
func calendarLiveSQL(seat, providers string) string {
	return `EXISTS (SELECT 1 FROM capture_connection seat_calendar WHERE seat_calendar.user_id=` + seat + `
 AND seat_calendar.provider=ANY(` + providers + `) AND seat_calendar.status IN ('connected','error'))`
}

// anotherLiveRosterSQL holds for a row that a seat besides the syncing one
// lists, when that seat holds a live calendar connection. A seat without one
// settles nothing again, so it keeps no row; its own sync takes the row back.
func anotherLiveRosterSQL(participant string) string {
	return `EXISTS (SELECT 1 FROM unnest(` + storekit.RosterSeatsSQL(participant) + `) AS listing(seat)
 WHERE listing.seat<>@` + rosterSeatArg + ` AND ` + calendarLiveSQL("listing.seat", "@"+calendarProvidersArg) + `)`
}

// nameRosterOnListedRows adds the syncing seat to the roster rows its current
// calendar lists. A row no roster lists is a member's, an import's or mail's.
// It stays out, so a roster that never wrote it does not retire it later.
//
// A colleague's own connected calendar decides their row alone, so nobody joins
// it. A colleague's address-only row stays with the calendar that wrote it.
// Another copy may not show why they hold no seat, and could seat them again.
func nameRosterOnListedRows(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, listed []connector.MessageParticipant,
) error {
	if !roster.Current || roster.Left || len(listed) == 0 {
		return nil
	}
	addresses, _ := listedAndDeclined(listed)
	args := []any{activityID, roster.Seat, addresses, storedDomainsOf(addresses)}
	colleague, err := calendarSeatMatch(ctx, tx, meetingKind, true, &args)
	if err != nil {
		return err
	}
	args = append(args, calendarProviderNames())
	providers := fmt.Sprintf("$%d", len(args))
	seats := storekit.RosterSeatsSQL("ap")
	_, err = tx.Exec(ctx, `
 UPDATE activity_participant ap SET `+storekit.StoreRosterSeatsSQL(storekit.RosterSeatsWithSQL("ap", "$2::uuid"))+`
 WHERE ap.activity_id=$1 AND ap.address=ANY($3::text[])
 AND `+storekit.OnACalendarRosterSQL("ap")+` AND NOT `+storekit.RosterListsSQL("ap", "$2")+`
 AND NOT (ap.user_id=ANY(`+seats+`) AND `+calendarLiveSQL("ap.user_id", providers)+`)
 AND NOT (ap.user_id IS NULL AND ap.contact_id IS NULL
  AND EXISTS (SELECT 1 FROM app_user u, unnest($3::text[], $4::text[]) AS inp(address, domain_folded)
   WHERE inp.address=ap.address AND `+colleague+`))`, args...)
	if err != nil {
		return fmt.Errorf("capture: naming a calendar on the guests it lists: %w", err)
	}
	return nil
}

func listedAndDeclined(listed []connector.MessageParticipant) (addresses, declined []string) {
	addresses = make([]string, 0, len(listed))
	declined = make([]string, 0, len(listed))
	for _, p := range listed {
		address := correspondence.Fold(p.Email)
		addresses = append(addresses, address)
		if p.Declined() {
			declined = append(declined, address)
		}
	}
	return addresses, declined
}

// retireOffTheRoster takes this calendar off the rows its roster no longer
// grants, and deletes a row once no live calendar lists it. A colleague who
// declined loses their seat row at once. So does anybody the organizer's
// complete roster dropped: the organizer's calendar speaks for the meeting.
// A row a member relinked is never retired, nor the syncing seat's own row or
// the host's. A seat that left is handled by retireTheSeatThatLeft.
func retireOffTheRoster(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, roster MeetingRoster, listed []connector.MessageParticipant,
) (int, error) {
	if !roster.Current {
		return 0, nil
	}
	if roster.Left {
		return retireTheSeatThatLeft(ctx, tx, activityID, roster.Seat)
	}
	organizer := false
	if roster.Complete {
		var err error
		if organizer, err = SeatOrganizesMeetingTx(ctx, tx, activityID, roster.Seat); err != nil {
			return 0, err
		}
	}
	addresses, declined := listedAndDeclined(listed)
	lists := storekit.RosterListsSQL("ap", "@seat")
	tag, err := tx.Exec(ctx, `
 DELETE FROM activity_participant ap USING activity a
 WHERE a.id=ap.activity_id AND ap.activity_id=@activity AND ap.relinked_at IS NULL AND ap.address IS NOT NULL
 AND `+storekit.OnACalendarRosterSQL("ap")+`
 AND ap.user_id IS DISTINCT FROM @seat AND ap.user_id IS DISTINCT FROM a.host_user_id
 AND ((ap.user_id IS NOT NULL AND ap.address=ANY(@declined) AND `+lists+`)
 OR (@complete AND NOT ap.address=ANY(@addresses) AND ((@organizer AND `+lists+`) OR NOT `+anotherLiveRosterSQL("ap")+`)))`, pgx.StrictNamedArgs{
		capturedActivityObject: activityID, rosterSeatArg: roster.Seat, "declined": declined, "complete": roster.Complete, rosterAddressesArg: addresses,
		"organizer": organizer, calendarProvidersArg: calendarProviderNames(),
	})
	if err != nil {
		return 0, fmt.Errorf("capture: retiring the guests a calendar no longer lists: %w", err)
	}
	retired := int(tag.RowsAffected())
	if !roster.Complete {
		return retired, nil
	}
	if err := leaveTheRowsOthersList(ctx, tx, activityID, roster.Seat, addresses); err != nil {
		return 0, err
	}
	if !organizer {
		return retired, nil
	}
	removed, err := retireSeatsTheOrganizerRemoved(ctx, tx, activityID, roster.Seat, addresses)
	return retired + removed, err
}

// leaveTheRowsOthersList takes the syncing seat off each row its complete
// roster dropped that another calendar still lists. The row and its answer stay.
func leaveTheRowsOthersList(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID, addresses []string) error {
	_, err := tx.Exec(ctx, `
 UPDATE activity_participant ap SET `+storekit.StoreRosterSeatsSQL(`array_remove(`+storekit.RosterSeatsSQL("ap")+`, @seat)`)+`
 FROM activity a
 WHERE a.id=ap.activity_id AND ap.activity_id=@activity AND ap.relinked_at IS NULL AND ap.address IS NOT NULL
 AND ap.user_id IS DISTINCT FROM @seat AND ap.user_id IS DISTINCT FROM a.host_user_id
 AND NOT ap.address=ANY(@addresses) AND `+storekit.RosterListsSQL("ap", "@seat")+`
 AND `+anotherLiveRosterSQL("ap"), pgx.StrictNamedArgs{
		capturedActivityObject: activityID, rosterSeatArg: seat, rosterAddressesArg: addresses, calendarProvidersArg: calendarProviderNames(),
	})
	if err != nil {
		return fmt.Errorf("capture: taking a calendar off the guests it no longer lists: %w", err)
	}
	return nil
}

// retireTheSeatThatLeft drops every calendar row bound to a seat whose own copy
// of the meeting is gone, whichever rosters list it. It also drops each other
// colleague's row that no live calendar lists any more, save the host's.
//
// An outside guest's row stays as the record of who was invited. The seat comes
// off the rows other calendars list.
func retireTheSeatThatLeft(ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID) (int, error) {
	tag, err := tx.Exec(ctx, `
 DELETE FROM activity_participant ap USING activity a
 WHERE a.id=ap.activity_id AND ap.activity_id=@activity AND ap.relinked_at IS NULL AND `+storekit.OnACalendarRosterSQL("ap")+`
 AND (ap.user_id=@seat OR (ap.user_id IS NOT NULL AND ap.user_id IS DISTINCT FROM a.host_user_id
  AND `+storekit.RosterListsSQL("ap", "@seat")+` AND NOT `+anotherLiveRosterSQL("ap")+`))`, pgx.StrictNamedArgs{
		capturedActivityObject: activityID, rosterSeatArg: seat, calendarProvidersArg: calendarProviderNames(),
	})
	if err != nil {
		return 0, fmt.Errorf("capture: retiring the seat that left a meeting: %w", err)
	}
	if _, err := tx.Exec(ctx, `
 UPDATE activity_participant ap SET `+storekit.StoreRosterSeatsSQL(`array_remove(`+storekit.RosterSeatsSQL("ap")+`, $2)`)+`
 WHERE ap.activity_id=$1 AND ap.relinked_at IS NULL AND `+storekit.RosterListsSQL("ap", "$2")+`
 AND cardinality(`+storekit.RosterSeatsSQL("ap")+`) > 1`, activityID, seat); err != nil {
		return 0, fmt.Errorf("capture: taking the seat that left off a meeting's guests: %w", err)
	}
	return int(tag.RowsAffected()), nil
}

// retireSeatsTheOrganizerRemoved deletes a colleague's own calendar rows once
// the organizer's complete roster no longer lists any address of theirs. Their
// own calendar may report the removal much later, or never. A seat row a
// member relinked to a contact stays: the member chose it, and no sync undoes
// that choice.
func retireSeatsTheOrganizerRemoved(
	ctx context.Context, tx pgx.Tx, activityID ids.ActivityID, seat ids.UUID, addresses []string,
) (int, error) {
	args := []any{activityID, seat, addresses, storedDomainsOf(addresses)}
	listedSeat, err := calendarSeatMatch(ctx, tx, meetingKind, true, &args)
	if err != nil {
		return 0, err
	}
	tag, err := tx.Exec(ctx, `
 DELETE FROM activity_participant ap USING activity a
 WHERE a.id=ap.activity_id AND ap.activity_id=$1 AND ap.relinked_at IS NULL
 AND `+storekit.RosterListsSQL("ap", "ap.user_id")+` AND ap.user_id<>$2 AND ap.user_id IS DISTINCT FROM a.host_user_id
 AND NOT EXISTS (SELECT 1 FROM app_user u, unnest($3::text[], $4::text[]) AS inp(address, domain_folded)
  WHERE u.id=ap.user_id AND (inp.address=ap.address OR `+listedSeat+`))`, args...)
	if err != nil {
		return 0, fmt.Errorf("capture: retiring the colleagues an organizer removed from a meeting: %w", err)
	}
	return int(tag.RowsAffected()), nil
}
