// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

type momentCopy struct{ langcopy.Spoken }

func momentCopyFor(language string) momentCopy { return momentCopy{langcopy.For(language)} }

var momentWords = struct {
	TaskUndated    langcopy.Phrase
	OweThem        langcopy.Phrase
	OweNamed       langcopy.Phrase
	PromiseUndated langcopy.Phrase
	NothingOwed    langcopy.Phrase
	NothingDue     langcopy.Phrase
	Log            langcopy.Phrase
	OpenContact    langcopy.Phrase
	OpenDeal       langcopy.Phrase
	OpenTaskList   langcopy.Phrase
}{
	TaskUndated:    langcopy.Phrase{En: "Open task with no due date. Stays open until somebody does it or closes it.", De: "Offene Aufgabe ohne Fälligkeitsdatum. Sie bleibt offen, bis jemand sie erledigt oder schließt.", Vi: "Nhiệm vụ đang mở chưa có hạn. Nhiệm vụ vẫn mở cho đến khi có người thực hiện hoặc đóng lại."},
	OweThem:        langcopy.Phrase{En: "You owe them: %s", De: "Sie schulden ihnen: %s", Vi: "Bạn cần thực hiện cho họ: %s"},
	OweNamed:       langcopy.Phrase{En: "You owe %s: %s", De: "Sie schulden %s: %s", Vi: "Bạn cần thực hiện cho %s: %s"},
	PromiseUndated: langcopy.Phrase{En: "Promised with no due date. Stays open until somebody does it or closes it.", De: "Zugesagt ohne Fälligkeitsdatum. Die Zusage bleibt offen, bis jemand sie erfüllt oder schließt.", Vi: "Đã cam kết nhưng chưa có hạn. Cam kết vẫn mở cho đến khi có người thực hiện hoặc đóng lại."},
	NothingOwed:    langcopy.Phrase{En: "Nothing is owed to this account", De: "Keine offenen Zusagen gegenüber diesem Unternehmen", Vi: "Không có cam kết nào chưa thực hiện với công ty này"},
	NothingDue:     langcopy.Phrase{En: "No promise to this account is open or coming due.", De: "Keine Zusage gegenüber diesem Unternehmen ist offen oder wird demnächst fällig.", Vi: "Không có cam kết nào với công ty này đang mở hoặc sắp đến hạn."},
	Log:            langcopy.Phrase{En: "Log something", De: "Aktivität erfassen", Vi: "Ghi lại hoạt động"},
	OpenContact:    langcopy.Phrase{En: "Open the contact", De: "Kontakt öffnen", Vi: "Mở liên hệ"},
	OpenDeal:       langcopy.Phrase{En: "Open the deal", De: "Deal öffnen", Vi: "Mở giao dịch"},
	OpenTaskList:   langcopy.Phrase{En: "Open it from the task list", De: "Über die Aufgabenliste öffnen", Vi: "Mở từ danh sách nhiệm vụ"},
}
