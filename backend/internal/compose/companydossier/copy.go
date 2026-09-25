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
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// phrase is one label in every language, kept together so a translator reads
// the three side by side.
type phrase struct{ en, de, vi string }

func (p phrase) in(lang textlang.Lang) string {
	switch lang {
	case textlang.German:
		return p.de
	case textlang.Vietnamese:
		return p.vi
	default:
		return p.en
	}
}

// dossierLabels answers every profile field the floor can state, in all three
// languages. Held by TestEveryShippedLanguageLabelsTheDossier — fieldSentence
// SKIPS a field it has no label for, so a missing translation carries one
// statement fewer rather than one statement in English.
var dossierLabels = map[crmcontracts.CompanyProfileFieldField]phrase{
	crmcontracts.CompanyProfileFieldFieldIcp: {
		en: "Ideal customer", de: "Idealer Kunde", vi: "Khách hàng lý tưởng",
	},
	crmcontracts.CompanyProfileFieldFieldIndustry: {
		en: "Industry", de: "Branche", vi: "Ngành",
	},
	crmcontracts.CompanyProfileFieldFieldCustomerPains: {
		en: "Customer pains", de: "Probleme der Kunden", vi: "Vấn đề của khách hàng",
	},
	crmcontracts.CompanyProfileFieldFieldDesiredOutcomes: {
		en: "Desired outcomes", de: "Gewünschte Ergebnisse", vi: "Kết quả mong muốn",
	},
	crmcontracts.CompanyProfileFieldFieldOfferSummary: {
		en: "What they offer", de: "Was sie anbieten", vi: "Họ cung cấp gì",
	},
	crmcontracts.CompanyProfileFieldFieldBuyingCenter: {
		en: "Buying centre", de: "Entscheidergremium", vi: "Nhóm ra quyết định mua",
	},
	crmcontracts.CompanyProfileFieldFieldBuyingIntents: {
		en: "Buying intents", de: "Kaufabsichten", vi: "Ý định mua",
	},
	crmcontracts.CompanyProfileFieldFieldSalesMotion: {
		en: "How they sell", de: "Wie sie verkaufen", vi: "Họ bán theo cách nào",
	},
	crmcontracts.CompanyProfileFieldFieldCommonObjections: {
		en: "Common objections", de: "Häufige Einwände", vi: "Phản đối thường gặp",
	},
	crmcontracts.CompanyProfileFieldFieldUsp: {
		en: "What sets them apart", de: "Was sie auszeichnet", vi: "Điều làm họ nổi bật",
	},
	crmcontracts.CompanyProfileFieldFieldValueProposition: {
		en: "Value proposition", de: "Nutzenversprechen", vi: "Giá trị mang lại",
	},
	crmcontracts.CompanyProfileFieldFieldLegalName: {
		en: "Legal name", de: "Firmenname", vi: "Tên pháp lý",
	},
	crmcontracts.CompanyProfileFieldFieldRegisterVat: {
		en: "Registration", de: "Registereintrag", vi: "Đăng ký kinh doanh",
	},
	crmcontracts.CompanyProfileFieldFieldRegisteredAddress: {
		en: "Registered address", de: "Eingetragene Anschrift", vi: "Địa chỉ đăng ký",
	},
	crmcontracts.CompanyProfileFieldFieldHistory: {
		en: "History", de: "Historie", vi: "Lịch sử",
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
		return p.in(textlang.Lang(lang)), true
	}
	return p.in(textlang.English), true
}
