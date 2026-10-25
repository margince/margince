// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

// Which way a relationship runs. The contact page, the company roll-up, the
// strength fold and the colleague graph all ask "did they write" and "did we
// write" here, so one contact cannot read two-way on one card and outbound
// only on the next. Each reader composes these with InteractionCountsSQL.

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

// DirectionOf is the one place the word is decided, from whether each side
// has a qualifying message in the reader's window.
func DirectionOf(theyWrote, weWrote bool) Direction {
	switch {
	case theyWrote && weWrote:
		return DirectionTwoWay
	case theyWrote:
		return DirectionInboundOnly
	case weWrote:
		return DirectionOutboundOnly
	}
	return DirectionNone
}

// TheyWroteSQL admits a message the contact wrote to us: inbound, with the
// contact as its sender. A colleague's reply on a thread the contact is only
// copied on is not the contact writing. contact is a SQL expression.
func TheyWroteSQL(contact, activity string) string {
	return "(" + activity + ".direction = 'inbound' AND " + SenderSQL(contact, activity) + ")"
}

// WeWroteSQL admits a message we sent. Which contacts it reached is the
// caller's walk, because reaching someone is not authorship.
func WeWroteSQL(activity string) string {
	return "(" + activity + ".direction = 'outbound')"
}
