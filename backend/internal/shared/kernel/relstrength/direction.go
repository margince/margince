// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

import "fmt"

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

// WeWroteSQL admits a message we sent. Which contacts it reached is the
// caller's walk, because reaching someone is not authorship.
func WeWroteSQL(activity string) string {
	return "(" + activity + ".direction = 'outbound')"
}
