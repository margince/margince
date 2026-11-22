// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Go's test cache never rechecks a file outside this module. Declared, a change
// to the presets or the tables these tests read replays no stale pass.
func TestTheTestCacheKeysOnTheTreeOutsideThisModule(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../backend", "../../../config", "../../../frontend"); err != nil {
		t.Fatal(err)
	}
}
