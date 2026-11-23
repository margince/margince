// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"go/format"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

var updateWireRule = flag.Bool("update-wire-rule", false,
	"rewrite testdata/gemini_wire.golden after geminiWireRule was bumped for a Gemini wire change")

const geminiWireGolden = "testdata/gemini_wire.golden"

// geminiWireFiles are the Gemini adapters' translation of a request, read
// without comments so a reworded comment demands no new rule.
var geminiWireFiles = []string{"gemini.go", "geministrict.go", "geminithinking.go", "vertextransport.go"}

func geminiWireDigest(t *testing.T) string {
	t.Helper()
	var material bytes.Buffer
	for _, name := range geminiWireFiles {
		fset := token.NewFileSet()
		file, err := parser.ParseFile(fset, name, nil, parser.SkipObjectResolution)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		material.WriteString(name + "\n")
		if err := format.Node(&material, fset, file); err != nil {
			t.Fatalf("formatting %s: %v", name, err)
		}
	}
	sum := sha256.Sum256(material.Bytes())
	return hex.EncodeToString(sum[:])
}

// A Gemini record carries geminiWireRule, so the translation it was measured
// through may not change under the same version.
func TestTheGeminiWireRuleMovesWithItsCode(t *testing.T) {
	got := geminiWireDigest(t)
	if *updateWireRule {
		if err := os.WriteFile(geminiWireGolden, []byte(geminiWireRule+" "+got+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return
	}
	raw, err := os.ReadFile(geminiWireGolden)
	if err != nil {
		t.Fatalf("reading %s: %v; run this test with -update-wire-rule", geminiWireGolden, err)
	}
	rule, digest, _ := strings.Cut(strings.TrimSpace(string(raw)), " ")
	switch {
	case rule == geminiWireRule && digest != got:
		t.Fatalf("the Gemini wire code changed without a new geminiWireRule: bump it in wirerule.go, then run this test with -update-wire-rule")
	case rule != geminiWireRule:
		t.Fatalf("geminiWireRule is %q and %s records %q; run this test with -update-wire-rule", geminiWireRule, geminiWireGolden, rule)
	}
}
