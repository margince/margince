// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package company360

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

type suggestionCopy struct{ langcopy.Spoken }

var suggestionWords = struct {
	OriginStalledDeal   langcopy.Phrase
	OriginOpenDeal      langcopy.Phrase
	OriginContractEnded langcopy.Phrase
	NoReplyReason       langcopy.Phrase
	NoReplyTitle        langcopy.Phrase
	StalledReason       langcopy.Phrase
	StalledTitle        langcopy.Phrase
	ConflictReason      langcopy.Phrase
	ConflictTitle       langcopy.Phrase
	NextCount           langcopy.Phrase
	NextSingle          langcopy.Phrase
	NextOne             langcopy.Phrase
	NextSome            langcopy.Phrase
	NextAll             langcopy.Phrase
	NextDeal            langcopy.Phrase
	NextAccount         langcopy.Phrase
	NextNamedAccount    langcopy.Phrase
	Prospect            langcopy.Phrase
	Opportunity         langcopy.Phrase
	Onboarding          langcopy.Phrase
	Live                langcopy.Phrase
	Customer            langcopy.Phrase
}{
	OriginStalledDeal:   langcopy.Phrase{En: "Open deal, last worked", De: "Offener Deal, zuletzt bearbeitet", Vi: "Giao dịch đang mở, lần xử lý gần nhất"},
	OriginOpenDeal:      langcopy.Phrase{En: "Open deal", De: "Offener Deal", Vi: "Giao dịch đang mở"},
	OriginContractEnded: langcopy.Phrase{En: "Read from their correspondence", De: "Aus ihrer Korrespondenz", Vi: "Trích từ thư trao đổi của họ"},
	NoReplyReason:       langcopy.Phrase{En: "You reached out %d days ago and nobody has come back.", De: "Ihr letzter Kontakt liegt %d Tage zurück, ohne Antwort.", Vi: "Bạn đã liên hệ cách đây %d ngày nhưng chưa nhận được phản hồi."},
	NoReplyTitle:        langcopy.Phrase{En: "Follow up: no reply in %d days", De: "Nachfassen: seit %d Tagen keine Antwort", Vi: "Theo dõi: chưa có phản hồi sau %d ngày"},
	StalledReason:       langcopy.Phrase{En: "%q has had no activity long enough to count as stalled.", De: "Bei %q gab es so lange keine Aktivität, dass der Deal als festgefahren gilt.", Vi: "%q không có hoạt động trong thời gian đủ lâu để được coi là đình trệ."},
	StalledTitle:        langcopy.Phrase{En: "Move %q — stalled", De: "%q voranbringen – festgefahren", Vi: "Thúc đẩy %q – đang đình trệ"},
	ConflictReason:      langcopy.Phrase{En: "Their correspondence says the contract ended, but this account is still filed as %s.", De: "Laut Korrespondenz ist der Vertrag beendet, aber dieses Unternehmen wird noch als %s geführt.", Vi: "Thư trao đổi cho biết hợp đồng đã kết thúc, nhưng công ty này vẫn được ghi nhận là %s."},
	ConflictTitle:       langcopy.Phrase{En: "Check the stage against what they wrote", De: "Den Status mit der Korrespondenz abgleichen", Vi: "Đối chiếu trạng thái với nội dung trao đổi"},
	NextSingle:          langcopy.Phrase{En: "One deal is open here and no task says what happens next.", De: "Hier ist ein Deal offen, aber keine Aufgabe beschreibt den nächsten Schritt.", Vi: "Có một giao dịch đang mở nhưng chưa có nhiệm vụ nào nêu bước tiếp theo."},
	NextCount:           langcopy.Phrase{En: "%d deals are open here and no task says what happens next.", De: "Hier sind %d Deals offen, aber keine Aufgabe beschreibt den nächsten Schritt.", Vi: "Có %d giao dịch đang mở nhưng chưa có nhiệm vụ nào nêu bước tiếp theo."},
	NextOne:             langcopy.Phrase{En: "%s is open and no task says what happens next.", De: "%s ist offen, aber keine Aufgabe beschreibt den nächsten Schritt.", Vi: "%s đang mở nhưng chưa có nhiệm vụ nào nêu bước tiếp theo."},
	NextSome:            langcopy.Phrase{En: "%d deals are open here, including %s, and no task says what happens next.", De: "Hier sind %d Deals offen, darunter %s, aber keine Aufgabe beschreibt den nächsten Schritt.", Vi: "Có %d giao dịch đang mở, gồm %s, nhưng chưa có nhiệm vụ nào nêu bước tiếp theo."},
	NextAll:             langcopy.Phrase{En: "%s are open and no task says what happens next.", De: "%s sind offen, aber keine Aufgabe beschreibt den nächsten Schritt.", Vi: "%s đang mở nhưng chưa có nhiệm vụ nào nêu bước tiếp theo."},
	NextDeal:            langcopy.Phrase{En: "Agree the next step on %q", De: "Den nächsten Schritt für %q vereinbaren", Vi: "Thống nhất bước tiếp theo cho %q"},
	NextAccount:         langcopy.Phrase{En: "Agree the next step with this account", De: "Den nächsten Schritt mit diesem Unternehmen vereinbaren", Vi: "Thống nhất bước tiếp theo với công ty này"},
	NextNamedAccount:    langcopy.Phrase{En: "Agree the next step with %s", De: "Den nächsten Schritt mit %s vereinbaren", Vi: "Thống nhất bước tiếp theo với %s"},
	Prospect:            langcopy.Phrase{En: "prospect", De: "Interessent", Vi: "khách hàng tiềm năng"},
	Opportunity:         langcopy.Phrase{En: "opportunity", De: "Verkaufschance", Vi: "cơ hội bán hàng"},
	Onboarding:          langcopy.Phrase{En: "onboarding", De: "Onboarding", Vi: "đang triển khai"},
	Live:                langcopy.Phrase{En: "live", De: "Live", Vi: "đang vận hành"},
	Customer:            langcopy.Phrase{En: "customer", De: "Kunde", Vi: "khách hàng"},
}

func suggestionCopyFor(lang textlang.Lang) suggestionCopy {
	return suggestionCopy{langcopy.For(string(lang))}
}

func (said suggestionCopy) lifecycle(value string) string {
	switch crmcontracts.CompanyLifecycle(value) {
	case crmcontracts.CompanyLifecycleProspect:
		return said.Say(suggestionWords.Prospect)
	case crmcontracts.CompanyLifecycleOpportunity:
		return said.Say(suggestionWords.Opportunity)
	case crmcontracts.CompanyLifecycleOnboarding:
		return said.Say(suggestionWords.Onboarding)
	case crmcontracts.CompanyLifecycleLive:
		return said.Say(suggestionWords.Live)
	case crmcontracts.CompanyLifecycleCustomer:
		return said.Say(suggestionWords.Customer)
	default:
		return value
	}
}
