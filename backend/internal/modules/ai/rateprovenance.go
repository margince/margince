// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import "errors"

// RateSource is who wrote a price: an admin by hand, the price sync, or provisioning.
// The sync never rewrites a model whose price in force is RateSourceManual.
type RateSource string

// The three writers a price can have, and the column and audit key naming one.
const (
	RateSourceManual    RateSource = "manual"
	RateSourceCatalogue RateSource = "catalogue"
	RateSourceSeed      RateSource = "seed"

	rateSourceField = "source"
)

// errHandSetSinceRead refuses a sync write over a price an admin typed after the
// sync read the sheet; SyncPrices skips the model rather than failing the run.
var errHandSetSinceRead = errors.New("ai: a hand-set price was written since the sync read the sheet")

func knownRateSource(s RateSource) bool {
	return s == RateSourceManual || s == RateSourceCatalogue || s == RateSourceSeed
}
