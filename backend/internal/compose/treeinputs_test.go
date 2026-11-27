// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Go's test cache never rechecks a file outside this module. Without this, a
// replayed pass would outlive a change to what these tests read there.
// That is the published pages, the presets, the recorded model runs and two files.
func TestTheTestCacheKeysOnTheTreeOutsideThisModule(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../docs/reference", "../../../frontend/src/lib/captureconcern.ts",
		"../../../config/presets", "../../../e2e/llm", "../../../scripts/e2e-llm.sh"); err != nil {
		t.Fatal(err)
	}
}
