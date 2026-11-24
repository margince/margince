// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package us

import "github.com/margince/margince/backend/pkg/extension/jurisdiction"

// countryFloors are the statutory retention floors of each country. No federal
// law names business letters, so the United States has no correspondence floor.
// Tax records matter until an assessment lapses: 3 years from filing, up to 7
// for a bad-debt claim. The return is filed the year after the record's year,
// so 8 years from that year's end is 7 years after the return. State law is
// not modelled.
func countryFloors() map[jurisdiction.Code][]jurisdiction.RetentionClass {
	return map[jurisdiction.Code][]jurisdiction.RetentionClass{
		"us": {{
			Name: jurisdiction.AccountingRecords, Keep: jurisdiction.Period{Years: 8},
			Anchor: jurisdiction.AnchorCalendarYearEnd,
			Law:    "26 CFR 1.6001-1(e); 26 U.S.C. 6501(a), (e) and 6511(d)(1)",
		}},
	}
}

// floors is a country's retention, read from countryFloors.
type floors []jurisdiction.RetentionClass

func (f floors) Classes() []jurisdiction.RetentionClass { return f }
