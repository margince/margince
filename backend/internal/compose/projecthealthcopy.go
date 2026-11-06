// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The words the project health job writes: the one-sentence reason a reading
// carries, and the owner's notice. Both are stored once and read by everyone
// who can see the project, so they follow the installation's base language,
// in the shape signalsummarycopy.go keeps.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// projectHealthCopy is one language's set. Every field is required.
type projectHealthCopy struct {
	// quiet, overdue, quietAndOverdue and active are the reasons. %d is the
	// quiet rule's window in days.
	quiet           string
	overdue         string
	quietAndOverdue string
	active          string
	// noticeSubject heads the owner's notice. %s is the project's name, then
	// the state's word, in that order.
	noticeSubject string
	// noticeUndo follows the reason in the notice body.
	noticeUndo string
	// The state words, as the notice subject says them.
	onTrack, atRisk, offTrack string
}

var projectHealthByLang = map[textlang.Lang]projectHealthCopy{
	textlang.English: {
		quiet:           "Nothing has been filed on this project for at least %d days.",
		overdue:         "An open task on this project is past its due date.",
		quietAndOverdue: "Nothing has been filed on this project for at least %d days, and an open task on it is past its due date.",
		active:          "Activity is being filed on this project and no open task is past its due date.",
		noticeSubject:   "%s is now %s",
		noticeUndo:      "The AI set this from the project's activity. Open the project to undo it.",
		onTrack:         "on track", atRisk: "at risk", offTrack: "off track",
	},
	textlang.German: {
		quiet:           "Zu diesem Projekt wurde seit mindestens %d Tagen nichts abgelegt.",
		overdue:         "Eine offene Aufgabe in diesem Projekt ist überfällig.",
		quietAndOverdue: "Zu diesem Projekt wurde seit mindestens %d Tagen nichts abgelegt, und eine offene Aufgabe darin ist überfällig.",
		active:          "Zu diesem Projekt wird Aktivität abgelegt, und keine offene Aufgabe ist überfällig.",
		noticeSubject:   "%s ist jetzt %s",
		noticeUndo:      "Die KI hat das aus der Aktivität des Projekts abgeleitet. Öffne das Projekt, um es rückgängig zu machen.",
		onTrack:         "im Plan", atRisk: "gefährdet", offTrack: "außer Plan",
	},
	textlang.Vietnamese: {
		quiet:           "Không có gì được lưu cho dự án này trong ít nhất %d ngày.",
		overdue:         "Một công việc đang mở của dự án này đã quá hạn.",
		quietAndOverdue: "Không có gì được lưu cho dự án này trong ít nhất %d ngày, và một công việc đang mở của dự án đã quá hạn.",
		active:          "Dự án này vẫn có hoạt động được lưu và không có công việc đang mở nào quá hạn.",
		noticeSubject:   "%s hiện %s",
		noticeUndo:      "AI đã đặt tình trạng này dựa trên hoạt động của dự án. Mở dự án để hoàn tác.",
		onTrack:         "đúng kế hoạch", atRisk: "có rủi ro", offTrack: "chệch kế hoạch",
	},
}

// projectHealthCopyFor answers the set for lang, falling back to English for a
// language the product does not ship.
func projectHealthCopyFor(lang textlang.Lang) projectHealthCopy {
	if said, ok := projectHealthByLang[lang]; ok {
		return said
	}
	return projectHealthByLang[textlang.English]
}

func (words projectHealthCopy) state(state crmcontracts.ProjectHealthState) string {
	switch state {
	case crmcontracts.ProjectHealthStateOffTrack:
		return words.offTrack
	case crmcontracts.ProjectHealthStateAtRisk:
		return words.atRisk
	default:
		return words.onTrack
	}
}
