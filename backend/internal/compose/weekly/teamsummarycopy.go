// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

type teamCopy struct{ langcopy.Spoken }

func teamCopyFor(language string) teamCopy { return teamCopy{langcopy.For(language)} }

var teamWords = struct {
	Counted     langcopy.Phrase
	CountedOne  langcopy.Phrase
	Help        langcopy.Phrase
	Strong      langcopy.Phrase
	AtRisk      langcopy.Phrase
	Holds       langcopy.Phrase
	Common      langcopy.Phrase
	Month       langcopy.Phrase
	Quarter     langcopy.Phrase
	ManagerCall langcopy.Phrase
	Median      langcopy.Phrase
}{
	Counted:     langcopy.Phrase{En: "%d of %d members counted; %d deals won and %d lost.", De: "Erfasste Teammitglieder: %d von %d; %d Deals gewonnen und %d verloren.", Vi: "Đã tính %d trên %d thành viên; chốt %d deal và thua %d."},
	CountedOne:  langcopy.Phrase{En: "%d of %d members counted; 1 deal won and %d lost.", De: "Erfasste Teammitglieder: %d von %d; ein Deal gewonnen und %d verloren.", Vi: "Đã tính %d trên %d thành viên; chốt một deal và thua %d."},
	Help:        langcopy.Phrase{En: "Asked for help: %s.", De: "Um Hilfe gebeten: %s.", Vi: "Đã yêu cầu hỗ trợ: %s."},
	Strong:      langcopy.Phrase{En: "Strong week: %s.", De: "Starke Woche: %s.", Vi: "Tuần làm việc tốt: %s."},
	AtRisk:      langcopy.Phrase{En: "%s: coverage at risk against %s (%d%%); a replan is recommended.", De: "%s: Deckung gegenüber %s gefährdet (%d%%); eine neue Planung wird empfohlen.", Vi: "%s: mức bao phủ có rủi ro so với %s (%d%%); nên lập lại kế hoạch."},
	Holds:       langcopy.Phrase{En: "%s: coverage holds against %s (%d%%).", De: "%s: Deckung gegenüber %s ausreichend (%d%%).", Vi: "%s: mức bao phủ đáp ứng %s (%d%%)."},
	Common:      langcopy.Phrase{En: "Most common pattern: %s (%d members).", De: "Häufigstes Muster: %s (%d Teammitglieder).", Vi: "Mẫu phổ biến nhất: %s (%d thành viên)."},
	Month:       langcopy.Phrase{En: "Month", De: "Monat", Vi: "Tháng"},
	Quarter:     langcopy.Phrase{En: "Quarter", De: "Quartal", Vi: "Quý"},
	ManagerCall: langcopy.Phrase{En: "the manager's call", De: "der Einschätzung der Führungskraft", Vi: "dự báo của người quản lý"},
	Median:      langcopy.Phrase{En: "the historical median", De: "dem historischen Median", Vi: "trung vị lịch sử"},
}

var focusWords = map[string]langcopy.Phrase{
	FocusHelpRequested:           {En: "help requested", De: "Hilfe angefragt", Vi: "yêu cầu hỗ trợ"},
	FocusLeadsBreached:           {En: "leads breached", De: "Antwortziel für Leads überschritten", Vi: "khách hàng tiềm năng quá hạn phản hồi"},
	FocusCommitmentsMissed:       {En: "commitments missed", De: "Zusagen nicht eingehalten", Vi: "cam kết chưa hoàn thành"},
	FocusDealsAtRisk:             {En: "deals at risk", De: "Deals gefährdet", Vi: "deal có rủi ro"},
	FocusForecastDowngrades:      {En: "forecast downgrades", De: "Prognosen herabgestuft", Vi: "dự báo bị hạ"},
	FocusStageRegressions:        {En: "stage regressions", De: "Rückschritte im Dealverlauf", Vi: "giai đoạn bị lùi"},
	FocusCloseDatesProvisional:   {En: "close dates provisional", De: "Vorläufige Abschlussdaten", Vi: "ngày chốt tạm thời"},
	FocusNoNextStepCoverage:      {En: "no next step coverage", De: "Fehlende nächste Schritte", Vi: "thiếu bước tiếp theo"},
	FocusStrongWeek:              {En: "strong week", De: "Starke Woche", Vi: "tuần làm việc tốt"},
	FocusMeetingsWithoutNextStep: {En: "meetings without next step", De: "Besprechungen ohne nächsten Schritt", Vi: "cuộc họp chưa có bước tiếp theo"},
	FocusQuietWeek:               {En: "quiet week", De: "Ruhige Woche", Vi: "tuần ít hoạt động"},
}

func (said teamCopy) focusWord(kind string) string {
	if phrase, ok := focusWords[kind]; ok {
		return said.Say(phrase)
	}
	return strings.ReplaceAll(kind, "_", " ")
}

func (said teamCopy) basisWord(basis string) string {
	switch basis {
	case "manager_call":
		return said.Say(teamWords.ManagerCall)
	case "historical_median":
		return said.Say(teamWords.Median)
	}
	return basis
}

func (said teamCopy) horizonWord(kind string) string {
	switch kind {
	case "month":
		return said.Say(teamWords.Month)
	case "quarter":
		return said.Say(teamWords.Quarter)
	}
	return strings.ReplaceAll(kind, "_", " ")
}
