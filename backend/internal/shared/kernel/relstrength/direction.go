// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import (
	"fmt"
	"strings"
)

// Which way a relationship runs. The contact page, the company roll-up, the
// strength fold and the colleague graph ask "did they write" here. One contact
// then cannot read two-way on one card and outbound only on the next. Each
// reader composes these with InteractionCountsSQL.

// Direction is which sides of a relationship have written.
type Direction string

// The four readings, as the contract spells them.
const (
	DirectionTwoWay       Direction = "two_way"
	DirectionInboundOnly  Direction = "inbound_only"
	DirectionOutboundOnly Direction = "outbound_only"
	DirectionNone         Direction = "none"
)

// Directions lists every reading, for the contract enum check.
func Directions() []Direction {
	return []Direction{DirectionTwoWay, DirectionInboundOnly, DirectionOutboundOnly, DirectionNone}
}

// Sides is whether each side has a qualifying message in the reader's window.
type Sides struct {
	TheyWrote bool
	WeWrote   bool
}

// Direction is the one place the word is decided.
func (s Sides) Direction() Direction {
	switch {
	case s.TheyWrote && s.WeWrote:
		return DirectionTwoWay
	case s.TheyWrote:
		return DirectionInboundOnly
	case s.WeWrote:
		return DirectionOutboundOnly
	}
	return DirectionNone
}

// TheyWroteSQL admits a message the contact sent us; a colleague's reply on
// their thread is not theirs. Older mail that lost its `from` row keeps the
// sender as its counterparty. contact is a SQL expression.
func TheyWroteSQL(contact, activity string) string {
	return "(" + activity + ".direction = 'inbound' AND (" + SenderSQL(contact, activity) + " OR " +
		fmt.Sprintf(senderlessCounterpartySQL, contact, activity) + "))"
}

const senderlessCounterpartySQL = `(NOT EXISTS (SELECT 1 FROM activity_participant af
		 WHERE af.activity_id = %[2]s.id AND af.role = 'from')
		AND EXISTS (SELECT 1 FROM contact_email ce
		 WHERE ce.contact_id = %[1]s AND ce.archived_at IS NULL AND ce.email = %[2]s.counterparty_email))`

// ContactTookPartSQL admits an interaction the contact took part in: a message
// they sent, or a meeting or call they were in. Callers already admit only
// meetings and calls that took place, through InteractionCountsAtSQL.
func ContactTookPartSQL(contact, activity string) string {
	return "(" + TheyWroteSQL(contact, activity) + " OR (" + activity + ".kind IN ('meeting','call') AND " +
		fmt.Sprintf(contactWasInSQL, contact, activity, InteractionRolesSQL) + "))"
}

// contactWasInSQL: the contact's own row on the meeting or call stands and did
// not decline. A meeting logged by hand names no party, so its link is the
// evidence. One whose parties name other contacts and not this one is not theirs.
const contactWasInSQL = `(EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = %[2]s.id AND tp.contact_id = %[1]s AND tp.role IN %[3]s
		   AND tp.response IS DISTINCT FROM 'declined')
	   OR NOT EXISTS (SELECT 1 FROM activity_participant tp
		 WHERE tp.activity_id = %[2]s.id AND tp.contact_id IS NOT NULL))`

// WeWroteSQL admits a message we sent. Which contacts it reached is the
// caller's walk, because reaching someone is not authorship.
func WeWroteSQL(activity string) string {
	return "(" + activity + ".direction = 'outbound')"
}

// FoldColumnsSQL lists the activity columns a fold over a derived row set reads:
// what TheyWroteSQL, InteractionUnitSQL and the occurredday predicates ask of
// the activity. A derived table projects these, never a whole row.
func FoldColumnsSQL(activity string) string {
	columns := []string{"id", "kind", "thread_key", "occurred_at", "occurred_precision", "direction", "counterparty_email"}
	for i, column := range columns {
		columns[i] = activity + "." + column
	}
	return strings.Join(columns, ", ")
}

// AfterEngagedSQL admits an interaction of a fold row after the moment in
// column engaged, or any one when that is NULL. Both sides are stored
// occurred_at values of the same rows, so they compare as stored.
func AfterEngagedSQL(activity, engaged string) string {
	return "(" + activity + "." + engaged + " IS NULL OR " + activity + ".occurred_at > " + activity + "." + engaged + " -- occurred_at raw: two stored stamps of one fold, ordered\n)"
}
