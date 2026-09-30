// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deployconfig

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/runtimeenv"
)

// The file decodes strictly, so deleting a key would refuse to boot every
// operator file still carrying it. The retired key must load, and say once that
// it acts on nothing.
func TestARetiredModelPricingKeyStillLoadsAndWarns(t *testing.T) {
	cfg, err := layered(t, runtimeenv.Test,
		"version: 1\nrates:\n  model_pricing:\n    gemini: https://example.test/pricing\n", "")
	if err != nil {
		t.Fatalf("an operator file carrying rates.model_pricing must still boot: %v", err)
	}
	warnings := cfg.Rates.Warnings()
	if len(warnings) != 1 {
		t.Fatalf("Warnings() = %v, want exactly one", warnings)
	}
	if !strings.Contains(warnings[0], "rates.model_pricing") || !strings.Contains(warnings[0], "margince.yaml") {
		t.Errorf("warning %q names neither the retired key nor the file to edit", warnings[0])
	}
}

func TestARatesBlockWithoutTheRetiredKeyWarnsOfNothing(t *testing.T) {
	cfg, err := layered(t, runtimeenv.Test, "version: 1\nrates:\n  fx_currencies: [USD]\n", "")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if w := cfg.Rates.Warnings(); len(w) != 0 {
		t.Errorf("Warnings() = %v for a block that says nothing stale, want none", w)
	}
}
