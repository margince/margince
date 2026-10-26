// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

// Whether a human wrote an activity, or a system sent it. Every exchange
// reader asks this through InteractionCountsSQL. The facts are the ones
// capture stores on the row. A row with none of them counts, so mail of
// unknown provenance is never dropped without proof.

import "slices"

// AutomaticKind names what sent an activity no human wrote. Empty means a
// human wrote it.
type AutomaticKind string

// The automatic kinds, in the order Automatic checks them.
const (
	// AutomaticProduct is a row the product wrote itself: its own mail to a
	// contact, or work it filed about a record.
	AutomaticProduct AutomaticKind = "product"
	// AutomaticCalendar is a mail carrying a calendar method: an invitation, a
	// response to one, or a cancellation.
	AutomaticCalendar AutomaticKind = "calendar"
	// AutomaticSendingSystem is mail whose sender is a sending system: a
	// file-share notice, a notification, a delivery report.
	AutomaticSendingSystem AutomaticKind = "sending_system"
)

// AutomaticKinds lists every automatic kind, for the contract enum check.
func AutomaticKinds() []AutomaticKind {
	return []AutomaticKind{AutomaticProduct, AutomaticCalendar, AutomaticSendingSystem}
}

// systemOrigins are the activity origins that mean the product wrote the row.
var systemOrigins = []string{"system_remediation", "system_notice"}

const kindEmail = "email"

// Evidence is what capture stored about who sent one activity row.
type Evidence struct {
	Kind            string
	Origin          string
	CalendarMethod  string
	SenderIsMachine bool
}

// Automatic is WrittenByHumanSQL for a reader holding the row in Go, naming
// the reason instead of answering a bool.
func (e Evidence) Automatic() AutomaticKind {
	switch {
	case slices.Contains(systemOrigins, e.Origin):
		return AutomaticProduct
	case e.Kind != kindEmail:
		return ""
	case e.CalendarMethod != "":
		return AutomaticCalendar
	case e.SenderIsMachine:
		return AutomaticSendingSystem
	}
	return ""
}

// OriginIsEngagementSQL admits a row the product did not write itself. The
// recency readers reach it through auth.OriginIsEngagement.
func OriginIsEngagementSQL(alias string) string {
	return alias + ".origin NOT IN (" + sqlList(systemOrigins) + ")"
}

// WrittenByHumanSQL admits an activity row a human wrote. A call, meeting or
// chat message passes unless the product wrote it. An email also needs no
// calendar method, and a sender that is not a sending system.
func WrittenByHumanSQL(alias string) string {
	return "(" + OriginIsEngagementSQL(alias) + " AND (" + alias + ".kind <> '" + kindEmail + "' OR (NOT " +
		alias + ".sender_is_machine AND " + alias + ".calendar_method IS NULL)))"
}

// DirectionClockSQL admits a row to a "they last wrote" or "we last wrote"
// clock: any directed kind, not a called-off meeting, written by a human.
func DirectionClockSQL(alias string) string {
	return "(" + NotCalledOffSQL(alias) + " AND " + WrittenByHumanSQL(alias) + ")"
}
