// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package draftreason reads what a drafting instruction says a message is. The
// web composer's drafts and an agent's draft_email both ask it, so one
// instruction names one reason on every path.
package draftreason

import (
	"regexp"
	"strings"

	"github.com/margince/margince/backend/internal/platform/evidencefold"
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

// ForInstruction is the reason an instruction names: a requested follow-up on
// the agreed appointment when it asks to move one, and none otherwise.
func ForInstruction(intent string) commsauthz.Category {
	if appointmentChange.MatchString(evidencefold.Normalize(intent)) {
		return commsauthz.CategoryRequestedFollowup
	}
	return ""
}
