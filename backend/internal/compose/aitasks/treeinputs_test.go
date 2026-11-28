// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package aitasks_test

import (
	"os"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Go's test cache never rechecks a file outside this module. Without this, a
// replayed pass would outlive a change to the prompts pages these tests read.
func TestTheTestCacheKeysOnThePromptsPages(t *testing.T) {
	if err := gatekit.DeclareInputs(os.Getenv, "../../../../docs/reference/ai-prompts.md", "../../../../docs/reference/ai-prompts.json"); err != nil {
		t.Fatal(err)
	}
}
