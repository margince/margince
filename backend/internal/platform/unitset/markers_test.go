// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package unitset

import (
	"strings"
	"testing"
)

func TestMisplacedDevelopmentMarkersRefuseProductionComposition(t *testing.T) {
	for name, source := range map[string]string{
		"extra text":    "require example.test/demo v1.0.0 // margince:development-only demo",
		"punctuation":   "require example.test/demo v1.0.0 //margince:development-only; x",
		"case":          "require example.test/demo v1.0.0 // Margince:Development-only",
		"above":         "// margince:development-only\nrequire example.test/demo v1.0.0",
		"replace":       "require example.test/demo v1.0.0\nreplace example.test/demo => ../demo // margince:development-only",
		"block":         "require ( // margince:development-only\nexample.test/demo v1.0.0\n)",
		"closing block": "require (\nexample.test/demo v1.0.0\n) // margince:development-only",
		"unknown":       "require example.test/demo v1.0.0 // margince:development",
	} {
		t.Run(name, func(t *testing.T) {
			root := writeInstance(t, "module example.test/instance\n\n"+source+"\n")
			if _, err := Selected(root, false); err == nil || !strings.Contains(err.Error(), DevelopmentOnlyMarker) {
				t.Fatalf("marker admission = %v, want explicit placement error", err)
			}
		})
	}
}
