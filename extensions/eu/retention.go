// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package eu

import "github.com/margince/margince/backend/pkg/extension/jurisdiction"

// memberStateFloors are the statutory retention floors of each member state
// other than Germany. A correspondence floor is listed only where the law
// itself names business letters; elsewhere an email is kept only when it is
// an accounting voucher, which the accounting class already covers. Where tax
// law keeps the same records longer than commercial law, the longer period is
// the floor. A law that counts from the end of the financial year is anchored
// at calendar-year end, which never ends a floor early for a calendar year.
// Slovenia is absent: its accounting period is set by a standard, not a
// statute, and no period could be confirmed.
func memberStateFloors() map[jurisdiction.Code][]jurisdiction.RetentionClass {
	return map[jurisdiction.Code][]jurisdiction.RetentionClass{
		"at": {
			correspondence(7, "Unternehmensgesetzbuch (UGB) §212; Bundesabgabenordnung (BAO) §132"),
			accounting(7, "Unternehmensgesetzbuch (UGB) §212; Bundesabgabenordnung (BAO) §132(1)"),
		},
		"be": {accounting(10, "Code des impôts sur les revenus 1992, art. 315; Code TVA, art. 60")},
		"bg": {accounting(10, "Закон за счетоводството, чл. 12, ал. 1")},
		"hr": {accounting(11, "Zakon o računovodstvu (NN 85/24), čl. 10 st. 2")},
		"cy": {accounting(6, "Companies Law, Cap. 113, s.141; Assessment and Collection of Taxes Law 4/1978, s.30")},
		"cz": {accounting(10, "Zákon o dani z přidané hodnoty 235/2004 Sb., §35; Zákon o účetnictví 563/1991 Sb., §31")},
		"dk": {accounting(5, "Bogføringsloven (lov nr. 700 af 24.5.2022), §12")},
		"ee": {accounting(7, "Raamatupidamise seadus, §12")},
		"fi": {
			correspondence(6, "Kirjanpitolaki 1336/1997, 2 luku 10 §"),
			accounting(6, "Kirjanpitolaki 1336/1997, 2 luku 10 §"),
		},
		"fr": {accounting(10, "Code de commerce, art. L123-22")},
		"gr": {accounting(5, "Ν. 4308/2014, άρθρο 7")},
		"hu": {accounting(8, "2000. évi C. törvény a számvitelről, 169. §")},
		"ie": {accounting(6, "Companies Act 2014, s.285; Taxes Consolidation Act 1997, s.886")},
		"it": {
			correspondence(10, "Codice civile, artt. 2214 e 2220"),
			accounting(10, "Codice civile, art. 2220"),
		},
		"lv": {accounting(5, "Grāmatvedības likums, 28. pants")},
		"lt": {accounting(10, "Buhalterinės apskaitos įstatymas, 19 straipsnis")},
		"lu": {
			correspondence(10, "Code de commerce, art. 14 et 16"),
			accounting(10, "Code de commerce, art. 16"),
		},
		"mt": {
			correspondence(5, "Commercial Code (Cap. 13), art. 20"),
			accounting(10, "Companies Act (Cap. 386), art. 163"),
		},
		"nl": {accounting(7, "Burgerlijk Wetboek Boek 2, art. 10; Algemene wet inzake rijksbelastingen, art. 52")},
		"pl": {accounting(5, "Ustawa o rachunkowości, art. 74")},
		"pt": {
			correspondence(10, "Código Comercial, art. 40"),
			accounting(10, "Código Comercial, art. 40; Código do IRC, art. 123"),
		},
		"ro": {accounting(5, "Legea contabilității 82/1991, art. 25")},
		"sk": {accounting(10, "Zákon o účtovníctve 431/2002 Z.z., §35")},
		"es": {
			correspondence(6, "Código de Comercio, art. 30"),
			accounting(6, "Código de Comercio, art. 30"),
		},
		"se": {accounting(7, "Bokföringslagen (1999:1078), 7 kap. 2 §")},
	}
}

func correspondence(years int, law string) jurisdiction.RetentionClass {
	return yearEndFloor(jurisdiction.CommercialCorrespondence, years, law)
}

func accounting(years int, law string) jurisdiction.RetentionClass {
	return yearEndFloor(jurisdiction.AccountingRecords, years, law)
}

func yearEndFloor(name jurisdiction.RetentionClassName, years int, law string) jurisdiction.RetentionClass {
	return jurisdiction.RetentionClass{
		Name: name, Keep: jurisdiction.Period{Years: years}, Anchor: jurisdiction.AnchorCalendarYearEnd, Law: law,
	}
}

// floors is a member state's retention, read from memberStateFloors.
type floors []jurisdiction.RetentionClass

func (f floors) Classes() []jurisdiction.RetentionClass { return f }
