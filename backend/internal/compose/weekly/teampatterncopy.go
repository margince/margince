// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

var patternWords = struct {
	HelpOne       langcopy.Phrase
	HelpMany      langcopy.Phrase
	BreachOne     langcopy.Phrase
	BreachMany    langcopy.Phrase
	Kept          langcopy.Phrase
	WonOne        langcopy.Phrase
	WonMany       langcopy.Phrase
	KeptAll       langcopy.Phrase
	MeetingOne    langcopy.Phrase
	MeetingMany   langcopy.Phrase
	Quiet         langcopy.Phrase
	ForecastOne   langcopy.Phrase
	ForecastMany  langcopy.Phrase
	RegressedOne  langcopy.Phrase
	RegressedMany langcopy.Phrase
	Provisional   langcopy.Phrase
	NextStep      langcopy.Phrase
}{
	HelpOne:       langcopy.Phrase{En: "Asked for help on 1 commitment", De: "Für eine Zusage um Hilfe gebeten", Vi: "Đã yêu cầu hỗ trợ cho một cam kết"},
	HelpMany:      langcopy.Phrase{En: "Asked for help on %d commitments", De: "Für %d Zusagen um Hilfe gebeten", Vi: "Đã yêu cầu hỗ trợ cho %d cam kết"},
	BreachOne:     langcopy.Phrase{En: "1 lead went past the response target", De: "Ein Lead hat das Antwortziel überschritten", Vi: "Một khách hàng tiềm năng đã quá hạn phản hồi"},
	BreachMany:    langcopy.Phrase{En: "%d leads went past the response target", De: "%d Leads haben das Antwortziel überschritten", Vi: "%d khách hàng tiềm năng đã quá hạn phản hồi"},
	Kept:          langcopy.Phrase{En: "Kept %d of %d commitments", De: "Zusagen eingehalten: %d von %d", Vi: "Đã hoàn thành %d trên %d cam kết"},
	WonOne:        langcopy.Phrase{En: "Won 1 deal — worth asking how", De: "Einen Deal gewonnen: nach dem Vorgehen fragen", Vi: "Chốt một deal: nên hỏi cách thực hiện"},
	WonMany:       langcopy.Phrase{En: "Won %d deals — worth asking how", De: "%d Deals gewonnen: nach dem Vorgehen fragen", Vi: "Chốt %d deal: nên hỏi cách thực hiện"},
	KeptAll:       langcopy.Phrase{En: "Kept every commitment (%d)", De: "Alle Zusagen eingehalten (%d)", Vi: "Đã hoàn thành mọi cam kết (%d)"},
	MeetingOne:    langcopy.Phrase{En: "1 meeting has no linked follow-up recorded", De: "Für eine Besprechung ist kein verknüpfter Folgeschritt erfasst", Vi: "Một cuộc họp chưa có bước theo dõi được liên kết"},
	MeetingMany:   langcopy.Phrase{En: "%d meetings have no linked follow-up recorded", De: "Für %d Besprechungen ist kein verknüpfter Folgeschritt erfasst", Vi: "%d cuộc họp chưa có bước theo dõi được liên kết"},
	Quiet:         langcopy.Phrase{En: "No priority indicated by the recorded metrics", De: "Die erfassten Kennzahlen zeigen keine Priorität an", Vi: "Các chỉ số đã ghi nhận không cho thấy ưu tiên nào"},
	ForecastOne:   langcopy.Phrase{En: "Forecast downgraded on 1 deal — review the recovery plan", De: "Prognose für einen Deal herabgestuft: Plan zur Verbesserung prüfen", Vi: "Dự báo của một deal bị hạ: xem lại kế hoạch cải thiện"},
	ForecastMany:  langcopy.Phrase{En: "Forecast downgraded on %d deals — review the recovery plan", De: "Prognose für %d Deals herabgestuft: Plan zur Verbesserung prüfen", Vi: "Dự báo của %d deal bị hạ: xem lại kế hoạch cải thiện"},
	RegressedOne:  langcopy.Phrase{En: "1 deal moved backwards — review the recovery plan", De: "Ein Deal ist zurückgefallen: Plan zur Verbesserung prüfen", Vi: "Một deal bị lùi giai đoạn: xem lại kế hoạch cải thiện"},
	RegressedMany: langcopy.Phrase{En: "%d deals moved backwards — review the recovery plan", De: "%d Deals sind zurückgefallen: Plan zur Verbesserung prüfen", Vi: "%d deal bị lùi giai đoạn: xem lại kế hoạch cải thiện"},
	Provisional:   langcopy.Phrase{En: "%d of %d open deals have a provisional close date — confirm the forecast", De: "%d von %d offenen Deals haben ein vorläufiges Abschlussdatum: Prognose bestätigen", Vi: "%d trên %d deal đang mở có ngày chốt tạm thời: xác nhận dự báo"},
	NextStep:      langcopy.Phrase{En: "%d of %d open deals have a next step — agree an owner and date", De: "%d von %d offenen Deals haben einen nächsten Schritt: Zuständigkeit und Datum vereinbaren", Vi: "%d trên %d deal đang mở có bước tiếp theo: thống nhất người phụ trách và ngày"},
}

func (said teamCopy) countPhrase(one, many langcopy.Phrase, count int) string {
	if count == 1 {
		return said.Say(one)
	}
	return fmt.Sprintf(said.Say(many), count)
}
