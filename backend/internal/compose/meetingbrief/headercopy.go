// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingbrief

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

//nolint:dupl // These distinct translations share a table shape, not behavior.
var headerWords = struct {
	WithStatus     langcopy.Phrase
	StatusMissing  langcopy.Phrase
	PromisedOurs   langcopy.Phrase
	PromisedTheirs langcopy.Phrase
	Target         langcopy.Phrase
	Close          langcopy.Phrase
	Meeting        langcopy.Phrase
	With           langcopy.Phrase
	Phase          langcopy.Phrase
	Move           langcopy.Phrase
	MoveTarget     langcopy.Phrase
	Ours           langcopy.Phrase
	Theirs         langcopy.Phrase
	Asked          langcopy.Phrase
	Due            langcopy.Phrase
	Source         langcopy.Phrase
}{
	WithStatus:     langcopy.Phrase{En: "%s — status: %s.", De: "%s — Status: %s.", Vi: "%s — trạng thái: %s."},
	StatusMissing:  langcopy.Phrase{En: "not recorded", De: "nicht erfasst", Vi: "chưa ghi nhận"},
	PromisedOurs:   langcopy.Phrase{En: "We promised %s: %s", De: "Wir haben %s zugesagt: %s", Vi: "Chúng ta đã hứa với %s: %s"},
	PromisedTheirs: langcopy.Phrase{En: "%s promised us: %s", De: "%s hat uns zugesagt: %s", Vi: "%s đã hứa với chúng ta: %s"},
	Target:         langcopy.Phrase{En: "target %s", De: "Zieltermin %s", Vi: "mục tiêu %s"},
	Close:          langcopy.Phrase{En: "close %s", De: "Abschluss %s", Vi: "chốt %s"},
	Meeting:        langcopy.Phrase{En: "Meeting", De: "Besprechung", Vi: "Cuộc họp"},
	With:           langcopy.Phrase{En: "%s with %s, %s.", De: "%s mit %s, %s.", Vi: "%s với %s, %s."},
	Phase:          langcopy.Phrase{En: "its current phase", De: "die aktuelle Phase", Vi: "giai đoạn hiện tại"},
	Move:           langcopy.Phrase{En: "Move %s on from %s.", De: "Bringe %s über %s hinaus voran.", Vi: "Đưa %s tiến lên từ %s."},
	MoveTarget:     langcopy.Phrase{En: "Move %s on from %s, against a target of %s.", De: "Bringe %s über %s hinaus voran, mit dem Zieltermin %s.", Vi: "Đưa %s tiến lên từ %s, hướng tới mục tiêu ngày %s."},
	Ours:           langcopy.Phrase{En: "We owe %s: %s", De: "Wir schulden %s: %s", Vi: "Chúng ta còn nợ %s: %s"},
	Theirs:         langcopy.Phrase{En: "%s owes us: %s", De: "%s schuldet uns: %s", Vi: "%s còn nợ chúng ta: %s"},
	Asked:          langcopy.Phrase{En: "%s asked: %s", De: "%s hat gefragt: %s", Vi: "%s đã hỏi: %s"},
	Due:            langcopy.Phrase{En: " (due %s)", De: " (fällig am %s)", Vi: " (hạn %s)"},
	Source:         langcopy.Phrase{En: ", from %s", De: ", aus %s", Vi: ", từ %s"},
}

var claimStatusWords = map[crmcontracts.ConversationClaimStatus]langcopy.Phrase{
	crmcontracts.ConversationClaimStatusOpen:      {En: "open", De: "offen", Vi: "chưa hoàn tất"},
	crmcontracts.ConversationClaimStatusDone:      {En: "done", De: "erledigt", Vi: "đã hoàn tất"},
	crmcontracts.ConversationClaimStatusDismissed: {En: "dismissed", De: "verworfen", Vi: "đã loại bỏ"},
}

func (said briefCopy) claimStatus(status string) string {
	if status == "" {
		return said.Say(headerWords.StatusMissing)
	}
	if phrase, ok := claimStatusWords[crmcontracts.ConversationClaimStatus(status)]; ok {
		return said.Say(phrase)
	}
	return status
}
