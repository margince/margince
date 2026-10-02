// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

// RateSource is who wrote a price: a person, the price sync, or provisioning.
// The sync never rewrites a model whose price in force is RateSourceManual.
type RateSource string

const (
	RateSourceManual    RateSource = "manual"
	RateSourceCatalogue RateSource = "catalogue"
	RateSourceSeed      RateSource = "seed"
)

func knownRateSource(s RateSource) bool {
	return s == RateSourceManual || s == RateSourceCatalogue || s == RateSourceSeed
}
