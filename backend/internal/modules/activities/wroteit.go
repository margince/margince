// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import crmcontracts "github.com/margince/margince/backend/internal/contracts"

// WroteItOurSide answers who held the pen on one row.
//
// The row's voice names the side when it can. A colleague writing from one of
// our own domains arrives inbound, and direction alone calls that mail theirs.
//
// A machine and an unresolved sender name no side, so those read the transport.
// known is false when neither can answer.
func WroteItOurSide(activity crmcontracts.Activity) (ours, known bool) {
	if voice := activity.Voice; voice != nil && voice.Author != nil {
		switch voice.Author.Kind {
		case crmcontracts.ActivityAuthorKindColleague:
			return true, true
		case crmcontracts.ActivityAuthorKindContactRecord:
			return false, true
		}
	}
	if activity.Direction == nil {
		return false, false
	}
	switch *activity.Direction {
	case crmcontracts.ActivityDirectionOutbound:
		return true, true
	case crmcontracts.ActivityDirectionInbound:
		return false, true
	}
	return false, false
}
