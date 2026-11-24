// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package us

import "github.com/margince/margince/backend/pkg/extension/jurisdiction"

// countryFloors are the statutory retention floors of each country. No federal
// law names business letters, so the United States has no correspondence floor.
// The tax rule keeps records for as long as they may matter to an assessment, and
// the assessment period runs 3 years from filing, 6 where income is understated
// by a quarter and 7 for a bad-debt or worthless-security claim. Seven years
// from the end of the year holds across all three.
func countryFloors() map[jurisdiction.Code][]jurisdiction.RetentionClass {
	return map[jurisdiction.Code][]jurisdiction.RetentionClass{
		"us": {{
			Name: jurisdiction.AccountingRecords, Keep: jurisdiction.Period{Years: 7},
			Anchor: jurisdiction.AnchorCalendarYearEnd,
			Law:    "26 CFR 1.6001-1(e); 26 U.S.C. 6501(a), (e) and 6511(d)(1)",
		}},
	}
}

// floors is a country's retention, read from countryFloors.
type floors []jurisdiction.RetentionClass

func (f floors) Classes() []jurisdiction.RetentionClass { return f }
