// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Go's test cache never rechecks a file outside this module, so without this a
// replayed pass would outlive a change to the ones these tests read.
func TestTheTestCacheKeysOnTheTreeOutsideThisModule(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../../../config"); err != nil {
		t.Fatal(err)
	}
}
