// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package network

import "github.com/margince/margince/backend/internal/shared/kernel/relstrength"

// twoWayExchange is the claim that a colleague and a contact wrote to each
// other. Reach, the graph's route evidence and the intro ranking all ask it
// here. Each would otherwise read "this colleague wrote" from its own count.
func twoWayExchange(inbound int, sent *int, outbound int) bool {
	sides := relstrength.Sides{TheyWrote: inbound > 0, WeWrote: ourSide(sent, outbound) > 0}
	return sides.Direction() == relstrength.DirectionTwoWay
}

// ourSide is what this colleague sent, not every outbound mail they were on:
// a colleague copied both ways has written nothing. The sent count is nil on
// a row folded before it existed, and for a reader without activity read.
// Both keep the outbound reading, which for that reader withholds nothing.
func ourSide(sent *int, outbound int) int {
	if sent != nil {
		return *sent
	}
	return outbound
}
