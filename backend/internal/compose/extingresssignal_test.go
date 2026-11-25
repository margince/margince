// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/pkg/extension"
)

// A unit past its signal ceiling is refused before any query runs.
func TestAUnitThatLandsTooManySignalsIsRefused(t *testing.T) {
	const unit = "flooding-unit"
	for range extensionSignalLandingsPerMinute {
		extensionSignalLandings.Allow(unit)
	}
	r := &callRuntime{unit: unit}
	rec := extension.Record{Signal: &extension.SignalFields{}}
	if _, err := r.landSignal(t.Context(), rec, extension.IngressSource{}); !errors.Is(err, extension.ErrRateLimited) {
		t.Fatalf("err = %v, want ErrRateLimited", err)
	}
}
