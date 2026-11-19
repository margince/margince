// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package apac

import "github.com/margince/margince/backend/pkg/extension/jurisdiction"

// countryFloors are the statutory retention floors of each country. None of
// these laws names business letters as such, so no country has a
// correspondence floor: Japan and Korea keep only "important" business
// documents, which a floor on every email would overstate. A law that counts
// from the end of the financial year is anchored at calendar-year end; one
// that counts from the transaction is anchored at its date. The Philippines
// is absent: only its tax code keeps records, from the filing deadline, which
// neither anchor can state.
func countryFloors() map[jurisdiction.Code][]jurisdiction.RetentionClass {
	return map[jurisdiction.Code][]jurisdiction.RetentionClass{
		"au": {fromRecord(7, "Corporations Act 2001, s286")},
		"nz": {yearEnd(7, "Companies Act 1993, s189 and s194; Tax Administration Act 1994, s22")},
		"jp": {yearEnd(10, "Companies Act (Act No. 86 of 2005), art. 432")},
		"kr": {yearEnd(10, "Commercial Act, art. 33")},
		"cn": {yearEnd(30, "Accounting Archives Management Measures 2015, art. 14")},
		"in": {yearEnd(8, "Companies Act 2013, s128(5)")},
		"sg": {yearEnd(5, "Companies Act 1967, s199; Income Tax Act 1947, s67")},
		"my": {fromRecord(7, "Companies Act 2016, s245; Income Tax Act 1967, s82")},
		"th": {yearEnd(5, "Accounting Act B.E. 2543 (2000), s14")},
		"id": {yearEnd(10, "Law 8/1997 on Company Documents, art. 11")},
		"hk": {yearEnd(7, "Companies Ordinance (Cap. 622), s373 and s377")},
		"tw": {yearEnd(10, "Business Entity Accounting Act, art. 38")},
		"vn": {yearEnd(10, "Accounting Law 88/2015/QH13, art. 41; Decree 174/2016/ND-CP, art. 13")},
	}
}

func yearEnd(years int, law string) jurisdiction.RetentionClass {
	return jurisdiction.RetentionClass{
		Name: jurisdiction.AccountingRecords, Keep: jurisdiction.Period{Years: years},
		Anchor: jurisdiction.AnchorCalendarYearEnd, Law: law,
	}
}

func fromRecord(years int, law string) jurisdiction.RetentionClass {
	return jurisdiction.RetentionClass{
		Name: jurisdiction.AccountingRecords, Keep: jurisdiction.Period{Years: years},
		Anchor: jurisdiction.AnchorOccurrence, Law: law,
	}
}

// floors is a country's retention, read from countryFloors.
type floors []jurisdiction.RetentionClass

func (f floors) Classes() []jurisdiction.RetentionClass { return f }
