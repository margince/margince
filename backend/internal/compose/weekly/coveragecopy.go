// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import "github.com/margince/margince/backend/internal/shared/kernel/langcopy"

var coverageWords = struct{ None, After, During langcopy.Phrase }{
	None:   langcopy.Phrase{En: "No %s was recorded in this scope, so this figure is not a measurement.", De: "Im betrachteten Bereich gibt es keinen Eintrag vom Typ „%s“. Diese Zahl ist daher kein Messergebnis.", Vi: "Chưa ghi nhận bản ghi loại %s trong phạm vi này, nên con số này không phải kết quả đo lường."},
	After:  langcopy.Phrase{En: "The first %s in this scope was recorded after this week ended, so this figure is not a measurement.", De: "Der erste Eintrag vom Typ „%s“ im betrachteten Bereich wurde nach Ende dieser Woche erfasst. Diese Zahl ist daher kein Messergebnis.", Vi: "Bản ghi loại %s đầu tiên trong phạm vi này được ghi nhận sau khi tuần này kết thúc, nên con số này không phải kết quả đo lường."},
	During: langcopy.Phrase{En: "The first %s in this scope was recorded during this week. Only what happened from then on is counted.", De: "Der erste Eintrag vom Typ „%s“ im betrachteten Bereich wurde während dieser Woche erfasst. Gezählt wird nur, was seitdem geschehen ist.", Vi: "Bản ghi loại %s đầu tiên trong phạm vi này được ghi nhận trong tuần này. Chỉ tính những gì xảy ra từ thời điểm đó."},
}

var coverageNouns = map[string]langcopy.Phrase{
	"deal":         {En: "deal", De: "Deal", Vi: "giao dịch"},
	"task":         {En: "task", De: "Aufgabe", Vi: "công việc"},
	"meeting":      {En: "meeting", De: "Besprechung", Vi: "cuộc họp"},
	"inbound lead": {En: "inbound lead", De: "eingehender Lead", Vi: "khách hàng tiềm năng đến"},
	planNoun:       {En: "weekly plan", De: "Wochenplan", Vi: "kế hoạch tuần"},
}
