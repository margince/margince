// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import "fmt"

// RosterSeatsSQL is the seats whose calendar rosters list a participant row, as
// an array that is never null. A row from before the set holds one seat in
// roster_seat until the copy task moves it, so the two columns read as one.
func RosterSeatsSQL(participant string) string {
	return fmt.Sprintf(`array_remove(%[1]s.roster_seats || %[1]s.roster_seat, NULL)`, participant)
}

// OnACalendarRosterSQL holds for a participant row some calendar roster lists.
func OnACalendarRosterSQL(participant string) string {
	return `cardinality(` + RosterSeatsSQL(participant) + `) > 0`
}

// RosterListsSQL holds when the roster of seat lists the participant row.
func RosterListsSQL(participant, seat string) string {
	return seat + ` = ANY(` + RosterSeatsSQL(participant) + `)`
}

// RosterSeatsWithSQL is the row's seats and more, each seat once. more is a
// seat or an array of seats.
func RosterSeatsWithSQL(participant, more string) string {
	return `ARRAY(SELECT DISTINCT listing.seat FROM unnest(` + RosterSeatsSQL(participant) + ` || ` + more + `) AS listing(seat) ORDER BY 1)`
}

// StoreRosterSeatsSQL is the SET clause that stores seats as a row's rosters.
// It clears the older column, so a row never states its rosters twice.
func StoreRosterSeatsSQL(seats string) string {
	return `roster_seats = NULLIF(` + seats + `, '{}'), roster_seat = NULL`
}

// RosterOfSeatSQL is the roster set of a new row that seat's calendar lists.
// A null seat gives a row no roster lists.
func RosterOfSeatSQL(seat string) string {
	return `NULLIF(array_remove(ARRAY[` + seat + `::uuid], NULL), '{}')`
}
