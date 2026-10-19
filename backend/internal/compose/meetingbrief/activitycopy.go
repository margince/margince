// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var activityWords = map[crmcontracts.ActivityKind]langcopy.Phrase{
	crmcontracts.ActivityKindCall:    {En: "call", De: "Anruf", Vi: "cuộc gọi"},
	crmcontracts.ActivityKindEmail:   {En: "email", De: "E-Mail", Vi: "email"},
	crmcontracts.ActivityKindMeeting: {En: "meeting", De: "Besprechung", Vi: "cuộc họp"},
	crmcontracts.ActivityKindMessage: {En: "message", De: "Nachricht", Vi: "tin nhắn"},
	crmcontracts.ActivityKindNote:    {En: "note", De: "Notiz", Vi: "ghi chú"},
	crmcontracts.ActivityKindTask:    {En: "task", De: "Aufgabe", Vi: "công việc"},
}

func (said briefCopy) subjectOrKind(activity ActIn) string {
	if activity.Subject != "" {
		return activity.Subject
	}
	if phrase, ok := activityWords[crmcontracts.ActivityKind(activity.Kind)]; ok {
		return said.Say(phrase)
	}
	return activity.Kind
}
