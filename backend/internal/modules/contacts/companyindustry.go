// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"strings"
	"unicode"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// industryEntry is one code of the fixed industry list: how each language
// words it and the other spellings that mean it.
type industryEntry struct {
	code    crmcontracts.CompanyIndustry
	label   langcopy.Phrase
	aliases []string
}

// industryTable decides what a company's industry value means. The
// column holds one of these codes or nothing; every writer decides the code
// through ResolveIndustry and every reader words it through IndustryLabel.
//
// Held against crm.yaml by TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt
// (backend/gates/contractvocabulary_test.go) and, for the labels, by
// backend/gates/companyindustry_test.go.
var industryTable = []industryEntry{
	{crmcontracts.CompanyIndustryAgriculture, langcopy.Phrase{En: "Agriculture", De: "Landwirtschaft", Vi: "Nông nghiệp"}, []string{"farming", "agribusiness", "Land- und Forstwirtschaft", "Agrar"}},
	{crmcontracts.CompanyIndustryAutomotive, langcopy.Phrase{En: "Automotive", De: "Automobilindustrie", Vi: "Ô tô"}, []string{"automobile", "car industry", "Automobil", "Kfz", "Kfz-Branche", "Ô tô xe máy"}},
	{crmcontracts.CompanyIndustryAerospaceDefense, langcopy.Phrase{En: "Aerospace and Defense", De: "Luft- und Raumfahrt, Verteidigung", Vi: "Hàng không vũ trụ và quốc phòng"}, []string{"aerospace", "defense", "defence", "aviation", "Luftfahrt", "Rüstung", "Verteidigung"}},
	{crmcontracts.CompanyIndustryChemicalsMaterials, langcopy.Phrase{En: "Chemicals and Materials", De: "Chemie und Werkstoffe", Vi: "Hóa chất và vật liệu"}, []string{"chemicals", "chemistry", "materials", "Chemie", "Chemische Industrie", "Hóa chất"}},
	{crmcontracts.CompanyIndustryConstruction, langcopy.Phrase{En: "Construction", De: "Bauwesen", Vi: "Xây dựng"}, []string{"building", "civil engineering", "Bau", "Bauindustrie", "Baugewerbe", "Hoch- und Tiefbau"}},
	{crmcontracts.CompanyIndustryConsumerGoodsRetail, langcopy.Phrase{En: "Consumer Goods and Retail", De: "Konsumgüter und Handel", Vi: "Hàng tiêu dùng và bán lẻ"}, []string{"retail", "consumer goods", "e-commerce", "Einzelhandel", "Handel", "Konsumgüter", "Bán lẻ", "Hàng tiêu dùng"}},
	{crmcontracts.CompanyIndustryEducation, langcopy.Phrase{En: "Education", De: "Bildung", Vi: "Giáo dục"}, []string{"school", "university", "training", "e-learning", "Schule", "Hochschule", "Ausbildung", "Bildungswesen"}},
	{crmcontracts.CompanyIndustryEnergyUtilities, langcopy.Phrase{En: "Energy and Utilities", De: "Energie und Versorgung", Vi: "Năng lượng và tiện ích"}, []string{"energy", "utilities", "oil and gas", "renewable energy", "Energie", "Energiewirtschaft", "Energieversorgung", "Versorger", "Năng lượng"}},
	{crmcontracts.CompanyIndustryEntertainmentMedia, langcopy.Phrase{En: "Entertainment and Media", De: "Unterhaltung und Medien", Vi: "Giải trí và truyền thông"}, []string{"entertainment", "media", "publishing", "gaming", "Medien", "Verlagswesen", "Unterhaltung", "Truyền thông"}},
	{crmcontracts.CompanyIndustryFinancialServices, langcopy.Phrase{En: "Financial Services", De: "Finanzdienstleistungen", Vi: "Dịch vụ tài chính"}, []string{"finance", "banking", "bank", "fintech", "asset management", "Finanzen", "Finanzwesen", "Bankwesen", "Banken", "Ngân hàng", "Tài chính"}},
	{crmcontracts.CompanyIndustryFoodBeverage, langcopy.Phrase{En: "Food and Beverage", De: "Lebensmittel und Getränke", Vi: "Thực phẩm và đồ uống"}, []string{"food", "beverage", "food and beverages", "Lebensmittelindustrie", "Ernährungsindustrie", "Getränke", "Thực phẩm"}},
	{crmcontracts.CompanyIndustryGovernmentPublicSector, langcopy.Phrase{En: "Government and Public Sector", De: "Öffentlicher Sektor", Vi: "Chính phủ và khu vực công"}, []string{"government", "public sector", "public administration", "Öffentliche Verwaltung", "Behörde", "Verwaltung", "Chính phủ"}},
	{crmcontracts.CompanyIndustryHealthcare, langcopy.Phrase{En: "Healthcare", De: "Gesundheitswesen", Vi: "Y tế"}, []string{"health", "medical", "hospital", "medical devices", "Gesundheit", "Medizin", "Medizintechnik", "Krankenhaus", "Chăm sóc sức khỏe", "Sức khỏe"}},
	{crmcontracts.CompanyIndustryHospitalityTourism, langcopy.Phrase{En: "Hospitality and Tourism", De: "Gastgewerbe und Tourismus", Vi: "Khách sạn và du lịch"}, []string{"hospitality", "tourism", "travel", "hotels", "restaurants", "Gastronomie", "Hotellerie", "Tourismus", "Reisen", "Du lịch"}},
	{crmcontracts.CompanyIndustryInsurance, langcopy.Phrase{En: "Insurance", De: "Versicherung", Vi: "Bảo hiểm"}, []string{"insurer", "insurtech", "Versicherungen", "Versicherungswesen"}},
	{crmcontracts.CompanyIndustryItServices, langcopy.Phrase{En: "IT Services", De: "IT-Dienstleistungen", Vi: "Dịch vụ CNTT"}, []string{"IT", "information technology", "IT consulting", "managed services", "IT-Beratung", "IT-Services", "Informationstechnologie", "Dịch vụ công nghệ thông tin", "Công nghệ thông tin"}},
	{crmcontracts.CompanyIndustrySoftware, langcopy.Phrase{En: "Software", De: "Software", Vi: "Phần mềm"}, []string{"SaaS", "B2B SaaS", "software development", "Softwareentwicklung", "Softwarehersteller"}},
	{crmcontracts.CompanyIndustryLegalProfessionalServices, langcopy.Phrase{En: "Legal and Professional Services", De: "Recht und professionelle Dienstleistungen", Vi: "Dịch vụ pháp lý và chuyên nghiệp"}, []string{"legal", "law", "law firm", "consulting", "professional services", "accounting", "Recht", "Rechtsberatung", "Unternehmensberatung", "Beratung", "Steuerberatung", "Wirtschaftsprüfung", "Tư vấn"}},
	{crmcontracts.CompanyIndustryLogisticsTransport, langcopy.Phrase{En: "Logistics and Transport", De: "Logistik und Transport", Vi: "Logistics và vận tải"}, []string{"logistics", "transport", "transportation", "shipping", "supply chain", "freight", "Logistik", "Transportwesen", "Spedition", "Verkehr", "Vận tải"}},
	{crmcontracts.CompanyIndustryManufacturing, langcopy.Phrase{En: "Manufacturing", De: "Fertigung", Vi: "Sản xuất"}, []string{"industrial manufacturing", "mechanical engineering", "machinery", "Maschinenbau", "Fertigungsindustrie", "Produktion", "Industrie", "Anlagenbau", "Chế tạo"}},
	{crmcontracts.CompanyIndustryMarketingAdvertising, langcopy.Phrase{En: "Marketing and Advertising", De: "Marketing und Werbung", Vi: "Tiếp thị và quảng cáo"}, []string{"marketing", "advertising", "ad agency", "public relations", "Werbung", "Werbebranche", "Werbeagentur", "Marketing und Kommunikation", "Quảng cáo", "Tiếp thị"}},
	{crmcontracts.CompanyIndustryMiningMetals, langcopy.Phrase{En: "Mining and Metals", De: "Bergbau und Metalle", Vi: "Khai khoáng và kim loại"}, []string{"mining", "metals", "steel", "Bergbau", "Metallindustrie", "Stahl", "Khai khoáng"}},
	{crmcontracts.CompanyIndustryNonprofit, langcopy.Phrase{En: "Nonprofit", De: "Gemeinnützige Organisation", Vi: "Phi lợi nhuận"}, []string{"non-profit", "not for profit", "charity", "NGO", "foundation", "Gemeinnützig", "Stiftung", "Verein", "Tổ chức phi lợi nhuận"}},
	{crmcontracts.CompanyIndustryPharmaBiotech, langcopy.Phrase{En: "Pharma and Biotech", De: "Pharma und Biotechnologie", Vi: "Dược phẩm và công nghệ sinh học"}, []string{"pharma", "pharmaceuticals", "biotech", "biotechnology", "life sciences", "Pharmaindustrie", "Pharmazie", "Biotechnologie", "Dược phẩm"}},
	{crmcontracts.CompanyIndustryRealEstate, langcopy.Phrase{En: "Real Estate", De: "Immobilien", Vi: "Bất động sản"}, []string{"property", "realty", "Immobilienwirtschaft", "Immobilienbranche", "Immobilienverwaltung"}},
	{crmcontracts.CompanyIndustryTelecommunications, langcopy.Phrase{En: "Telecommunications", De: "Telekommunikation", Vi: "Viễn thông"}, []string{"telecom", "telco", "telecoms", "Telekom", "Mobilfunk"}},
}

// industryKey folds a spelling to the letters and digits in it, so "Health
// care", "health-care" and "HEALTHCARE" are one key.
func industryKey(raw string) string {
	var b strings.Builder
	for _, r := range strings.ToLower(raw) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

// industryKeys maps every spelling of the table to its code. A spelling two
// codes claim is returned in clashes and keeps the first code.
func industryKeys(table []industryEntry) (keys map[string]string, clashes []string) {
	keys = make(map[string]string, len(table)*8)
	claim := func(spelling, code string) {
		key := industryKey(spelling)
		if key == "" {
			return
		}
		if held, taken := keys[key]; taken {
			if held != code {
				clashes = append(clashes, spelling)
			}
			return
		}
		keys[key] = code
	}
	for _, e := range table {
		code := string(e.code)
		claim(code, code)
		for _, label := range []string{e.label.En, e.label.De, e.label.Vi} {
			claim(label, code)
		}
		for _, alias := range e.aliases {
			claim(alias, code)
		}
	}
	return keys, clashes
}

var industryByKey, _ = industryKeys(industryTable)

// ResolveIndustry answers the code a text means. The match is exact on the
// folded spelling, with no fuzzy step and no model, so one text always gives
// one code; a phrase that names no code, such as a list of products, gives none.
func ResolveIndustry(raw string) (string, bool) {
	code, ok := industryByKey[industryKey(raw)]
	return code, ok
}

// IndustryLabel words a code in one language, and answers "" for anything that
// is not a code so a stale value is never shown as a label.
func IndustryLabel(code string, lang textlang.Lang) string {
	for _, e := range industryTable {
		if string(e.code) == code {
			return e.label.In(lang)
		}
	}
	return ""
}

// IndustryWords is what a reader sees for a stored value: the label of a code,
// and the value as stored for anything that is no code (a company not yet
// repaired), so a reader never sees a blank where a value exists.
func IndustryWords(stored string, lang textlang.Lang) string {
	if label := IndustryLabel(stored, lang); label != "" {
		return label
	}
	return stored
}

// IndustryCodes lists the fixed codes in table order.
func IndustryCodes() []string {
	codes := make([]string, len(industryTable))
	for i, e := range industryTable {
		codes[i] = string(e.code)
	}
	return codes
}

// industryWrite is the decision for one industry value a writer received.
type industryWrite struct {
	// code is the value to store, nil when the text names no code.
	code *string
	// note is the source note the description keeps when the text named no
	// code, "" otherwise.
	note string
	// given is false when the caller sent nothing worth deciding on.
	given bool
}

// resolveIndustryWrite is the funnel every writer of the industry column goes
// through. An unmappable text stores no code and returns the note that keeps it.
func resolveIndustryWrite(raw *string) industryWrite {
	if raw == nil {
		return industryWrite{}
	}
	text := strings.TrimSpace(*raw)
	if text == "" {
		return industryWrite{}
	}
	if code, ok := ResolveIndustry(text); ok {
		return industryWrite{code: &code, given: true}
	}
	return industryWrite{note: industrySourceNote(text), given: true}
}
