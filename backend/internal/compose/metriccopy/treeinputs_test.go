// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package metriccopy

import (
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Go's test cache never rechecks a file outside this module, so without this a
// replayed pass would outlive a change to the frontend fixture these tests read.
func TestTheTestCacheKeysOnTheFrontendFixture(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../../frontend"); err != nil {
		t.Fatal(err)
	}
}
