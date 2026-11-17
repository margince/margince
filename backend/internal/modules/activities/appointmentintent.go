// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"regexp"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

// appointmentChange matches an instruction to move an appointment, in each
// language the composer ships. It only names what the sender says the message
// is; the consent engine still needs an agreed appointment with the recipient.
var appointmentChange = regexp.MustCompile(`(?i)` + strings.Join([]string{
	`\breschedul`,
	`\b(move|postpone|shift|push back)\b.{0,30}\b(appointment|meeting|call|session)`,
	`\b(change|find|propose|suggest)\b.{0,20}\b(new|another|different)\b.{0,10}\b(time|date|slot)`,
	`\bverschieb`,
	`\btermin\b.{0,30}(ändern|verlegen|umlegen|tauschen)`,
	`\b(neuen|anderen)\s+termin`,
	`\bumplanen`,
	`dời\s+(lịch|cuộc\s+hẹn|cuộc\s+họp|buổi)`,
	`đổi\s+(lịch|giờ|ngày|thời\s+gian)`,
	`hoãn\s+(lịch|cuộc\s+hẹn|cuộc\s+họp|buổi)`,
	`lùi\s+(lịch|cuộc\s+hẹn|cuộc\s+họp|buổi)`,
}, "|"))

// draftedReason is what a drafting instruction says the message is: a follow-up
// on the agreed appointment when it asks to move one, with the anchor as the
// evidence when the anchor is that meeting. Nothing otherwise, because silence
// leaves the reason to the thread or to the sender.
func draftedReason(intent string, anchor ids.UUID, kind crmcontracts.ActivityKind) (commsauthz.Category, commsauthz.Evidence) {
	if !appointmentChange.MatchString(intent) {
		return "", commsauthz.Evidence{}
	}
	if kind == crmcontracts.ActivityKindMeeting {
		return commsauthz.CategoryRequestedFollowup, commsauthz.Evidence{ActivityID: anchor}
	}
	return commsauthz.CategoryRequestedFollowup, commsauthz.Evidence{}
}
