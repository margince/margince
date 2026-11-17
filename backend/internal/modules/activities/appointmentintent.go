// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"regexp"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/evidencefold"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// appointmentChange matches an instruction to move an appointment, in each
// language the composer ships. The verb has to name the meeting itself, so a
// deal or a delivery that moves does not count. It only names what the sender
// says the message is; the consent engine still needs an agreed appointment.
var appointmentChange = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\breschedul`,
	`\b(move|moving|postpone|postponing|shift|shifting|push|pushing|bring|bringing)\s+(the\s+|our\s+|my\s+|this\s+|that\s+|today'?s\s+|tomorrow'?s\s+)?(appointment|meeting|call|session|demo)\b`,
	`\b(move|moving|push|pushing)\s+it\s+(to|back|forward|out)\b`,
	`\b(find|propose|suggest|pick|offer)\s+(a\s+)?(new|another|different)\s+(time|date|slot)\b`,
	`\btermin\S*\s+(\S+\s+){0,4}(verschieben|verlegen|umlegen|ändern|tauschen)`,
	`terminverschiebung|terminänderung`,
	`\b(meeting|gespräch|besprechung|call)\S*\s+(\S+\s+){0,4}verschieben`,
	`\b(neuen|anderen|neuer)\s+termin`,
	`dời\s+(lịch|cuộc\s+hẹn|cuộc\s+họp|buổi\s+hẹn|buổi\s+họp)`,
	`đổi\s+(lịch\s+hẹn|lịch\s+họp|giờ\s+hẹn|giờ\s+họp|ngày\s+hẹn|ngày\s+họp)`,
	`(hoãn|lùi)\s+(lịch|cuộc\s+hẹn|cuộc\s+họp|buổi\s+hẹn|buổi\s+họp)`,
}, "|"))

// draftedReason is what a drafting instruction says the message is: a follow-up
// on the agreed appointment when it asks to move one, with the anchor as the
// evidence when the anchor is that meeting. Nothing otherwise, because silence
// leaves the reason to the thread or to the sender.
func draftedReason(intent string, anchor ids.UUID, kind crmcontracts.ActivityKind) (commsauthz.Category, commsauthz.Evidence) {
	if !appointmentChange.MatchString(evidencefold.Normalize(intent)) {
		return "", commsauthz.Evidence{}
	}
	if kind == crmcontracts.ActivityKindMeeting {
		return commsauthz.CategoryRequestedFollowup, commsauthz.Evidence{ActivityID: anchor}
	}
	return commsauthz.CategoryRequestedFollowup, commsauthz.Evidence{}
}
