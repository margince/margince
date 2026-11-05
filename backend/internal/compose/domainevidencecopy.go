// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

type domainEvidenceCopy struct{ langcopy.Spoken }

var domainEvidence = struct {
	Classified        langcopy.Phrase
	ClassifiedReason  langcopy.Phrase
	LegalName         langcopy.Phrase
	StatedName        langcopy.Phrase
	NotCompany        langcopy.Phrase
	VendorQuestion    langcopy.Phrase
	NothingRead       langcopy.Phrase
	NoCompany         langcopy.Phrase
	NotAllowed        langcopy.Phrase
	ForwardUnreadable langcopy.Phrase
	ScriptUnreadable  langcopy.Phrase
	NoText            langcopy.Phrase
	FailureUnknown    langcopy.Phrase
	Exhausted         langcopy.Phrase
	Company           langcopy.Phrase
	Personal          langcopy.Phrase
	Provider          langcopy.Phrase
	Parked            langcopy.Phrase
	Unclear           langcopy.Phrase
}{
	Classified:        langcopy.Phrase{En: "The site is classified as %s.", De: "Die Website wird als %s eingestuft.", Vi: "Trang web được phân loại là %s."},
	ClassifiedReason:  langcopy.Phrase{En: "The site is classified as %s: %s", De: "Die Website wird als %s eingestuft: %s", Vi: "Trang web được phân loại là %s: %s"},
	LegalName:         langcopy.Phrase{En: "The site's legal notice names %q.", De: "Die Rechtshinweise der Website nennen %q.", Vi: "Thông tin pháp lý trên trang web nêu tên %q."},
	StatedName:        langcopy.Phrase{En: "The site states the company name %q.", De: "Die Website nennt den Firmennamen %q.", Vi: "Trang web nêu tên công ty là %q."},
	NotCompany:        langcopy.Phrase{En: "This site says the domain does not belong to a company, so the read stopped after its landing page.", De: "Laut Website gehört die Domain zu keinem Unternehmen. Die Prüfung endete daher nach der Startseite.", Vi: "Trang web cho biết tên miền không thuộc một công ty, nên việc đọc dừng lại sau trang đầu."},
	VendorQuestion:    langcopy.Phrase{En: "This site reads as a mail or hosting vendor, so the read stopped after its landing page. A colleague decides whether its staff get a company.", De: "Die Website wirkt wie ein Mail- oder Hosting-Anbieter. Die Prüfung endete daher nach der Startseite. Ein Kollege entscheidet, ob seine Mitarbeitenden ein Unternehmen bekommen.", Vi: "Trang web có vẻ là nhà cung cấp email hoặc hosting, nên việc đọc dừng lại sau trang đầu. Một đồng nghiệp quyết định có tạo công ty cho nhân viên của nó hay không."},
	NothingRead:       langcopy.Phrase{En: "No site could be read for this domain, so the company question was answered from what the workspace already knew.", De: "Für diese Domain konnte keine Website gelesen werden. Die Unternehmensfrage wurde anhand der bereits bekannten Informationen beantwortet.", Vi: "Không đọc được trang web nào cho tên miền này. Câu hỏi về công ty được giải quyết dựa trên thông tin đã biết."},
	NoCompany:         langcopy.Phrase{En: "The site was read and named no company, so the question was answered from what the workspace already knew.", De: "Die Website wurde gelesen, nannte aber kein Unternehmen. Die Frage wurde anhand der bereits bekannten Informationen beantwortet.", Vi: "Trang web đã được đọc nhưng không nêu tên công ty. Câu hỏi được giải quyết dựa trên thông tin đã biết."},
	NotAllowed:        langcopy.Phrase{En: "Automatic enrichment is off, so the company question was answered without reading the site.", De: "Die automatische Anreicherung ist deaktiviert. Die Unternehmensfrage wurde ohne Lesen der Website beantwortet.", Vi: "Tự động bổ sung thông tin đang tắt. Câu hỏi về công ty được giải quyết mà không đọc trang web."},
	ForwardUnreadable: langcopy.Phrase{En: "The landing page forwards to an address that could not be read.", De: "Die Startseite leitet zu einer Adresse weiter, die nicht gelesen werden konnte.", Vi: "Trang đầu chuyển hướng đến một địa chỉ không đọc được."},
	ScriptUnreadable:  langcopy.Phrase{En: "The landing page is a JavaScript application shell this reader cannot render.", De: "Die Startseite benötigt JavaScript und kann von diesem Leser nicht dargestellt werden.", Vi: "Trang đầu là ứng dụng JavaScript mà trình đọc này không thể hiển thị."},
	NoText:            langcopy.Phrase{En: "The landing page carries no readable text.", De: "Die Startseite enthält keinen lesbaren Text.", Vi: "Trang đầu không có văn bản đọc được."},
	FailureUnknown:    langcopy.Phrase{En: "The site could not be read, and nothing said why.", De: "Die Website konnte nicht gelesen werden. Ein Grund wurde nicht angegeben.", Vi: "Không đọc được trang web và không có lý do được ghi lại."},
	Exhausted:         langcopy.Phrase{En: "Every attempt to read this site failed, so the question was answered from what the workspace already knew.", De: "Alle Versuche, diese Website zu lesen, sind fehlgeschlagen. Die Frage wurde anhand der bereits bekannten Informationen beantwortet.", Vi: "Mọi lần thử đọc trang web đều thất bại. Câu hỏi được giải quyết dựa trên thông tin đã biết."},
	Company:           langcopy.Phrase{En: "a company", De: "Unternehmen", Vi: "công ty"},
	Personal:          langcopy.Phrase{En: "a personal site", De: "persönliche Website", Vi: "trang cá nhân"},
	Provider:          langcopy.Phrase{En: "a service provider", De: "Dienstanbieter", Vi: "nhà cung cấp dịch vụ"},
	Parked:            langcopy.Phrase{En: "a parked domain", De: "geparkte Domain", Vi: "tên miền chưa sử dụng"},
	Unclear:           langcopy.Phrase{En: "unclear", De: "unklar", Vi: "chưa rõ"},
}

func domainEvidenceCopyFor(lang textlang.Lang) domainEvidenceCopy {
	return domainEvidenceCopy{langcopy.For(string(lang))}
}

func (said domainEvidenceCopy) siteKind(kind string) string {
	switch kind {
	case siteKindCompany:
		return said.Say(domainEvidence.Company)
	case siteKindPersonal:
		return said.Say(domainEvidence.Personal)
	case siteKindProvider:
		return said.Say(domainEvidence.Provider)
	case siteKindParked:
		return said.Say(domainEvidence.Parked)
	case siteKindUnclear:
		return said.Say(domainEvidence.Unclear)
	default:
		return kind
	}
}

func (w *siteDeepReadWorker) domainEvidence(ctx context.Context) domainEvidenceCopy {
	return domainEvidenceCopyFor(installationLanguage(w.pool).Resolve(ctx))
}
