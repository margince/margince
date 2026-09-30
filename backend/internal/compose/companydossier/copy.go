// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

// The dossier's field labels, in each language the product speaks.
//
// The dossier's deterministic floor states a stored value behind a label, and
// the label was English whatever the installation's base language was — so a
// German installation read a German dossier when the model answered and an
// English one when it did not, both stored and both surfacing as the same
// object.
//
// Label and value are joined with a colon and never grammatically: the values
// are whatever a human accepted off a site read, often in the company's OWN
// language, and a colon is the only join that is true of both a noun phrase and
// a whole sentence.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// phrase is one label in every language; the primitive it is built from is
// shared, because six private copies of one three-field struct is six places
// for a fallback to differ.
type phrase = langcopy.Phrase

// dossierLabels answers every profile field the floor can state, in all three
// languages. Held by TestEveryShippedLanguageLabelsTheDossier — fieldSentence
// SKIPS a field it has no label for, so a missing translation carries one
// statement fewer rather than one statement in English.
var dossierLabels = map[crmcontracts.CompanyProfileFieldField]phrase{
	crmcontracts.CompanyProfileFieldFieldIcp: {
		En: "Ideal customer", De: "Idealer Kunde", Vi: "Khách hàng lý tưởng",
	},
	crmcontracts.CompanyProfileFieldFieldIndustry: {
		En: "Industry", De: "Branche", Vi: "Ngành",
	},
	crmcontracts.CompanyProfileFieldFieldCustomerPains: {
		En: "Customer pains", De: "Probleme der Kunden", Vi: "Vấn đề của khách hàng",
	},
	crmcontracts.CompanyProfileFieldFieldDesiredOutcomes: {
		En: "Desired outcomes", De: "Gewünschte Ergebnisse", Vi: "Kết quả mong muốn",
	},
	crmcontracts.CompanyProfileFieldFieldOfferSummary: {
		En: "What they offer", De: "Was sie anbieten", Vi: "Họ cung cấp gì",
	},
	crmcontracts.CompanyProfileFieldFieldBuyingCenter: {
		En: "Buying centre", De: "Entscheidergremium", Vi: "Nhóm ra quyết định mua",
	},
	crmcontracts.CompanyProfileFieldFieldBuyingIntents: {
		En: "Buying intents", De: "Kaufabsichten", Vi: "Ý định mua",
	},
	crmcontracts.CompanyProfileFieldFieldSalesMotion: {
		En: "How they sell", De: "Wie sie verkaufen", Vi: "Họ bán theo cách nào",
	},
	crmcontracts.CompanyProfileFieldFieldCommonObjections: {
		En: "Common objections", De: "Häufige Einwände", Vi: "Phản đối thường gặp",
	},
	crmcontracts.CompanyProfileFieldFieldUsp: {
		En: "What sets them apart", De: "Was sie auszeichnet", Vi: "Điều làm họ nổi bật",
	},
	crmcontracts.CompanyProfileFieldFieldValueProposition: {
		En: "Value proposition", De: "Nutzenversprechen", Vi: "Giá trị mang lại",
	},
	crmcontracts.CompanyProfileFieldFieldLegalName: {
		En: "Legal name", De: "Firmenname", Vi: "Tên pháp lý",
	},
	crmcontracts.CompanyProfileFieldFieldRegisterVat: {
		En: "Registration", De: "Registereintrag", Vi: "Đăng ký kinh doanh",
	},
	crmcontracts.CompanyProfileFieldFieldRegisteredAddress: {
		En: "Registered address", De: "Eingetragene Anschrift", Vi: "Địa chỉ đăng ký",
	},
	crmcontracts.CompanyProfileFieldFieldHistory: {
		En: "History", De: "Historie", Vi: "Lịch sử",
	},
}

// labelFor answers one field's label in a language, falling back to English for
// one this build does not speak.
func labelFor(field crmcontracts.CompanyProfileFieldField, lang string) (string, bool) {
	p, ok := dossierLabels[field]
	if !ok {
		return "", false
	}
	if textlang.Known(lang) {
		return p.In(textlang.Lang(lang)), true
	}
	return p.In(textlang.English), true
}
