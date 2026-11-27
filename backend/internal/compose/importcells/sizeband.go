// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package importcells

import (
	"cmp"
	"maps"
	"slices"
	"strconv"
	"strings"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// sizeBands is the closed size_band vocabulary, read off the generated
// contract so a band added to crm.yaml leaves no second list behind.
// Held by: TestEveryClosedVocabularyOverAContractEnumHoldsAllOfIt (backend/gates/contractvocabulary_test.go)
var sizeBands = map[string]bool{
	string(crmcontracts.CompanySizeBandN110):      true,
	string(crmcontracts.CompanySizeBandN1150):     true,
	string(crmcontracts.CompanySizeBandN51200):    true,
	string(crmcontracts.CompanySizeBandN201500):   true,
	string(crmcontracts.CompanySizeBandN5011000):  true,
	string(crmcontracts.CompanySizeBandN10015000): true,
	string(crmcontracts.CompanySizeBandN5000):     true,
}

// IsSizeBand is whether a company import takes band as its size_band.
func IsSizeBand(band string) bool {
	return sizeBands[band]
}

// SizeBandVocabulary renders the bands from the smallest company to the
// largest, by the head count each band starts at.
func SizeBandVocabulary() []string {
	bands := slices.Collect(maps.Keys(sizeBands))
	slices.SortFunc(bands, func(a, b string) int {
		return cmp.Or(cmp.Compare(firstHeadCount(a), firstHeadCount(b)), cmp.Compare(a, b))
	})
	return bands
}

// firstHeadCount is the number a band opens with: 11 for "11-50", 5000 for "5000+".
func firstHeadCount(band string) int {
	end := strings.IndexFunc(band, func(r rune) bool { return r < '0' || r > '9' })
	if end < 0 {
		end = len(band)
	}
	count, err := strconv.Atoi(band[:end])
	if err != nil {
		// A band that opens with no number sorts first, where a reader sees it.
		return -1
	}
	return count
}
