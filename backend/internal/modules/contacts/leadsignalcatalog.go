// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// LeadSignalBand is one answer a rep may give for a factor. Min and Max are
// inclusive whole numbers, nil where the band has no numeric meaning or no
// bound on that side. CompanySizeBands lists the company size_band values that
// land in this band, for a factor a company record can answer.
type LeadSignalBand struct {
	ID               string
	Points           int
	Min, Max         *int
	CompanySizeBands []string
}

// LeadSignalFactor is one scoring question and its bands. CompanyField names
// the company column that can answer it, empty when only a rep can.
type LeadSignalFactor struct {
	ID           string
	CompanyField string
	Bands        []LeadSignalBand
}

const (
	sizeBandTiny  = "1-10"
	sizeBandSmall = "11-50"
	sizeBandMid   = "51-200"
)

// LeadSignalCatalog defines the manual scoring vocabulary. The validation,
// the HTTP catalog and the lead form all read it.
// It is a list so a later admin-chosen set can load it from a stored setting
// without changing a reader. The order is the order the form asks in.
func LeadSignalCatalog() []LeadSignalFactor {
	return []LeadSignalFactor{
		{ID: "web_traffic", Bands: []LeadSignalBand{
			{ID: "low", Points: 0, Max: new(9_999)},
			{ID: "medium", Points: 4, Min: new(10_000), Max: new(100_000)},
			{ID: "high", Points: 8, Min: new(100_001)},
		}},
		{ID: "employees", CompanyField: filterSizeBand, Bands: []LeadSignalBand{
			{ID: sizeBandTiny, Points: 0, Min: new(1), Max: new(10), CompanySizeBands: []string{sizeBandTiny}},
			{ID: sizeBandSmall, Points: 4, Min: new(11), Max: new(50), CompanySizeBands: []string{sizeBandSmall}},
			{ID: sizeBandMid, Points: 8, Min: new(51), Max: new(200), CompanySizeBands: []string{sizeBandMid}},
			{ID: "201+", Points: 10, Min: new(201), CompanySizeBands: []string{"201-500", "501-1000", "1001-5000", "5000+"}},
		}},
		{ID: "budget_hint", Bands: []LeadSignalBand{
			{ID: "none", Points: -4},
			{ID: "unknown", Points: 0},
			{ID: "some", Points: 4},
			{ID: "confirmed", Points: 10},
		}},
	}
}

// leadSignalPoints answers what a band adds to the score. factorKnown and
// bandValid separate the two refusals a caller names differently.
func leadSignalPoints(factor, band string) (points int, factorKnown, bandValid bool) {
	for _, f := range LeadSignalCatalog() {
		if f.ID != factor {
			continue
		}
		for _, b := range f.Bands {
			if b.ID == band {
				return b.Points, true, true
			}
		}
		return 0, true, false
	}
	return 0, false, false
}
