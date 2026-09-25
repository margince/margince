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

// dossierLabels is keyed by every language in textlang.Shipped, held by
// TestEveryShippedLanguageLabelsTheDossier.
var dossierLabels = map[textlang.Lang]map[crmcontracts.CompanyProfileFieldField]string{
	textlang.English: {
		crmcontracts.CompanyProfileFieldFieldIcp:               "Ideal customer",
		crmcontracts.CompanyProfileFieldFieldIndustry:          "Industry",
		crmcontracts.CompanyProfileFieldFieldCustomerPains:     "Customer pains",
		crmcontracts.CompanyProfileFieldFieldDesiredOutcomes:   "Desired outcomes",
		crmcontracts.CompanyProfileFieldFieldOfferSummary:      "What they offer",
		crmcontracts.CompanyProfileFieldFieldBuyingCenter:      "Buying centre",
		crmcontracts.CompanyProfileFieldFieldBuyingIntents:     "Buying intents",
		crmcontracts.CompanyProfileFieldFieldSalesMotion:       "How they sell",
		crmcontracts.CompanyProfileFieldFieldCommonObjections:  "Common objections",
		crmcontracts.CompanyProfileFieldFieldUsp:               "What sets them apart",
		crmcontracts.CompanyProfileFieldFieldValueProposition:  "Value proposition",
		crmcontracts.CompanyProfileFieldFieldLegalName:         "Legal name",
		crmcontracts.CompanyProfileFieldFieldRegisterVat:       "Registration",
		crmcontracts.CompanyProfileFieldFieldRegisteredAddress: "Registered address",
		crmcontracts.CompanyProfileFieldFieldHistory:           "History",
	},
	textlang.German: {
		crmcontracts.CompanyProfileFieldFieldIcp:               "Idealer Kunde",
		crmcontracts.CompanyProfileFieldFieldIndustry:          "Branche",
		crmcontracts.CompanyProfileFieldFieldCustomerPains:     "Probleme der Kunden",
		crmcontracts.CompanyProfileFieldFieldDesiredOutcomes:   "Gewünschte Ergebnisse",
		crmcontracts.CompanyProfileFieldFieldOfferSummary:      "Was sie anbieten",
		crmcontracts.CompanyProfileFieldFieldBuyingCenter:      "Entscheidergremium",
		crmcontracts.CompanyProfileFieldFieldBuyingIntents:     "Kaufabsichten",
		crmcontracts.CompanyProfileFieldFieldSalesMotion:       "Wie sie verkaufen",
		crmcontracts.CompanyProfileFieldFieldCommonObjections:  "Häufige Einwände",
		crmcontracts.CompanyProfileFieldFieldUsp:               "Was sie auszeichnet",
		crmcontracts.CompanyProfileFieldFieldValueProposition:  "Nutzenversprechen",
		crmcontracts.CompanyProfileFieldFieldLegalName:         "Firmenname",
		crmcontracts.CompanyProfileFieldFieldRegisterVat:       "Registereintrag",
		crmcontracts.CompanyProfileFieldFieldRegisteredAddress: "Eingetragene Anschrift",
		crmcontracts.CompanyProfileFieldFieldHistory:           "Historie",
	},
	textlang.Vietnamese: {
		crmcontracts.CompanyProfileFieldFieldIcp:               "Khách hàng lý tưởng",
		crmcontracts.CompanyProfileFieldFieldIndustry:          "Ngành",
		crmcontracts.CompanyProfileFieldFieldCustomerPains:     "Vấn đề của khách hàng",
		crmcontracts.CompanyProfileFieldFieldDesiredOutcomes:   "Kết quả mong muốn",
		crmcontracts.CompanyProfileFieldFieldOfferSummary:      "Họ cung cấp gì",
		crmcontracts.CompanyProfileFieldFieldBuyingCenter:      "Nhóm ra quyết định mua",
		crmcontracts.CompanyProfileFieldFieldBuyingIntents:     "Ý định mua",
		crmcontracts.CompanyProfileFieldFieldSalesMotion:       "Họ bán theo cách nào",
		crmcontracts.CompanyProfileFieldFieldCommonObjections:  "Phản đối thường gặp",
		crmcontracts.CompanyProfileFieldFieldUsp:               "Điều làm họ nổi bật",
		crmcontracts.CompanyProfileFieldFieldValueProposition:  "Giá trị mang lại",
		crmcontracts.CompanyProfileFieldFieldLegalName:         "Tên pháp lý",
		crmcontracts.CompanyProfileFieldFieldRegisterVat:       "Đăng ký kinh doanh",
		crmcontracts.CompanyProfileFieldFieldRegisteredAddress: "Địa chỉ đăng ký",
		crmcontracts.CompanyProfileFieldFieldHistory:           "Lịch sử",
	},
}

// labelsFor answers the dossier's labels for a language code, falling back to
// English for one this build does not speak.
func labelsFor(lang string) map[crmcontracts.CompanyProfileFieldField]string {
	if labels, ok := dossierLabels[textlang.Lang(lang)]; ok {
		return labels
	}
	return dossierLabels[textlang.English]
}
